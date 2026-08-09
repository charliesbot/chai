package add

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/skill"
	chaisync "github.com/charliesbot/chai/internal/sync"
)

type Request struct {
	Source string
	Skills []string
	List   bool
	Yes    bool
	Global bool
}

type Options struct {
	Confirm         func(string) (bool, error)
	CheckGit        func(context.Context) error
	Discover        func(context.Context, githubskill.Identity, string) (githubskill.Discovery, error)
	Materialize     func(context.Context, string, githubskill.Discovery, []string) (map[string]string, error)
	CommitPromotion func(githubskill.Promotion) error
	Sync            func(context.Context, *config.Config, string, chaisync.Options) error
	SyncOptions     chaisync.Options
	Output          io.Writer
}

func ParseArgs(args []string) (Request, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return Request{}, fmt.Errorf("add requires a source as the first argument")
	}
	request := Request{Source: args[0]}
	seenSkill := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--list":
			request.List = true
		case "--yes", "-y":
			request.Yes = true
		case "--global", "-g":
			request.Global = true
		case "--skill":
			if seenSkill {
				return Request{}, fmt.Errorf("--skill may be specified only once")
			}
			seenSkill = true
			start := i + 1
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				request.Skills = append(request.Skills, args[i])
			}
			if i+1 == start {
				return Request{}, fmt.Errorf("--skill requires at least one skill name")
			}
		default:
			return Request{}, fmt.Errorf("unknown add option or unexpected argument %q", args[i])
		}
	}
	for _, name := range request.Skills {
		if !skill.ValidName(name) {
			return Request{}, fmt.Errorf("invalid skill name %q", name)
		}
	}
	return request, nil
}

func Run(ctx context.Context, cfg *config.Config, args []string, opts Options) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return RunWithHome(ctx, cfg, filepath.Join(home, "chai.toml"), home, args, opts)
}

func RunWithHome(ctx context.Context, cfg *config.Config, configPath, home string, args []string, opts Options) error {
	request, err := ParseArgs(args)
	if err != nil {
		return err
	}
	if request.List && len(request.Skills) > 0 {
		return fmt.Errorf("--list cannot be combined with --skill")
	}
	if isLocalInput(request.Source) {
		return addLocal(ctx, cfg, configPath, home, request, opts)
	}
	return addRemote(ctx, cfg, configPath, home, request, opts)
}

func isLocalInput(source string) bool {
	return source == "~" || strings.HasPrefix(source, "~/") || filepath.IsAbs(source) ||
		strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../")
}

func NormalizeLocalPath(raw, manifestDir, home string) (string, error) {
	if !isLocalInput(raw) {
		return "", fmt.Errorf("local source must begin with /, ~/, ./, or ../")
	}
	switch {
	case raw == "~":
		return "~", nil
	case strings.HasPrefix(raw, "~/"):
		clean := filepath.Clean(raw[2:])
		if clean == "." {
			return "~", nil
		}
		return "~/" + filepath.ToSlash(clean), nil
	case filepath.IsAbs(raw):
		clean := filepath.Clean(raw)
		relative, err := filepath.Rel(home, clean)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			if relative == "." {
				return "~", nil
			}
			return "~/" + filepath.ToSlash(relative), nil
		}
		return clean, nil
	case strings.HasPrefix(raw, "./"):
		clean := filepath.Clean(raw)
		if clean == "." {
			return "./.", nil
		}
		return "./" + filepath.ToSlash(strings.TrimPrefix(clean, "./")), nil
	default:
		clean := filepath.Clean(raw)
		if clean == ".." {
			return "../.", nil
		}
		return filepath.ToSlash(clean), nil
	}
}

func addLocal(ctx context.Context, cfg *config.Config, configPath, home string, request Request, opts Options) error {
	if request.List {
		return fmt.Errorf("--list is supported only for GitHub sources")
	}
	if len(request.Skills) > 0 {
		return fmt.Errorf("--skill is supported only for GitHub sources")
	}
	normalized, err := NormalizeLocalPath(request.Source, filepath.Dir(configPath), home)
	if err != nil {
		return err
	}
	tracked, err := skill.DiscoverLocal([]string{normalized}, filepath.Dir(configPath), home)
	if err != nil {
		return err
	}
	found := false
	for _, existing := range cfg.Skills.Local {
		canonical, err := NormalizeLocalPath(existing, filepath.Dir(configPath), home)
		if err == nil && canonical == normalized {
			found = true
			break
		}
	}
	roots := append([]string(nil), cfg.Skills.Local...)
	if !found {
		roots = append(roots, normalized)
	}
	discovered, err := skill.DiscoverLocal(roots, filepath.Dir(configPath), home)
	if err != nil {
		return err
	}
	if err := rejectNameConflicts(discovered, nil, cfg, ""); err != nil {
		return err
	}
	trackedNames := make([]string, len(tracked))
	for i, source := range tracked {
		trackedNames[i] = source.Name
	}
	manifestChange := "no changes"
	if !found {
		manifestChange = fmt.Sprintf("add local source %s to %s", normalized, configPath)
	}
	summary := fmt.Sprintf("Add local source %s; tracked skills: %s; manifest: %s; sync to %s", normalized, strings.Join(trackedNames, ", "), manifestChange, strings.Join(cfg.Platforms, ", "))
	confirmed, err := confirm(summary, request.Yes, opts)
	if err != nil || !confirmed {
		return err
	}
	if !found {
		cfg.Skills.Local = append(cfg.Skills.Local, normalized)
		sort.Strings(cfg.Skills.Local)
	}
	if err := chaisync.ValidateSources(cfg, home); err != nil {
		return err
	}
	if err := config.SaveAtomic(configPath, cfg); err != nil {
		return err
	}
	return runSync(ctx, cfg, home, opts)
}

func addRemote(ctx context.Context, cfg *config.Config, configPath, home string, request Request, opts Options) error {
	id, err := githubskill.ParseInput(request.Source)
	if err != nil {
		return err
	}
	checkGit := opts.CheckGit
	if checkGit == nil {
		checkGit = func(ctx context.Context) error {
			_, err := githubskill.CheckGit(ctx)
			return err
		}
	}
	if err := checkGit(ctx); err != nil {
		return err
	}

	staging, err := os.MkdirTemp("", "chai-skill-add-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(staging)
		}
	}()
	discover := opts.Discover
	if discover == nil {
		discover = func(ctx context.Context, id githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, id.URL(), repository)
		}
	}
	repository := githubskill.RepositoryDir(staging)
	discovery, err := discover(ctx, id, repository)
	if err != nil {
		return err
	}
	if request.List {
		printDiscovery(output(opts), discovery)
		return nil
	}

	selected := append([]string(nil), request.Skills...)
	if len(selected) == 0 {
		if len(discovery.Problems) > 0 {
			return fmt.Errorf("cannot add all skills because %s", discovery.Problems[0])
		}
		for _, candidate := range discovery.Candidates {
			selected = append(selected, candidate.Name)
		}
		if len(selected) == 0 {
			return fmt.Errorf("GitHub source contains no valid skills")
		}
	}
	existingIndex := -1
	existingNames := make(map[string]bool)
	for i, source := range cfg.Skills.GitHub {
		if source.URL == id.URL() {
			existingIndex = i
			for _, name := range source.Include {
				existingNames[name] = true
			}
			selected = append(selected, source.Include...)
			break
		}
	}
	selected = sortedUnique(selected)
	materialize := opts.Materialize
	if materialize == nil {
		materialize = githubskill.Materialize
	}
	mapping, err := materialize(ctx, repository, discovery, selected)
	if err != nil {
		return err
	}
	locals, err := skill.DiscoverLocal(cfg.Skills.Local, filepath.Dir(configPath), home)
	if err != nil {
		return err
	}
	selectedSources := make([]skill.Source, len(selected))
	for i, name := range selected {
		selectedSources[i] = skill.Source{Name: name, Path: id.URL()}
	}
	if err := rejectNameConflicts(selectedSources, locals, cfg, id.URL()); err != nil {
		return err
	}
	commit, err := gitCommit(ctx, repository)
	if err != nil {
		return err
	}
	if err := githubskill.CompleteStaging(staging, id, mapping, commit); err != nil {
		return err
	}
	manifestChange := fmt.Sprintf("add GitHub source to %s", configPath)
	if existingIndex >= 0 {
		var added []string
		for _, name := range selected {
			if !existingNames[name] {
				added = append(added, name)
			}
		}
		if len(added) == 0 {
			manifestChange = "no changes"
		} else {
			manifestChange = fmt.Sprintf("add skills %s to %s", strings.Join(added, ", "), configPath)
		}
	}
	summary := fmt.Sprintf("Add %s; selected skills: %s; manifest: %s; sync to %s", id.URL(), strings.Join(selected, ", "), manifestChange, strings.Join(cfg.Platforms, ", "))
	confirmed, err := confirm(summary, request.Yes, opts)
	if err != nil || !confirmed {
		return err
	}
	staging, err = githubskill.StagePrepared(home, id, staging)
	if err != nil {
		return err
	}
	promotion, err := githubskill.BeginPromotion(staging, githubskill.CacheDir(home, id))
	if err != nil {
		return err
	}
	keep = true
	if existingIndex >= 0 {
		cfg.Skills.GitHub[existingIndex].Include = selected
	} else {
		cfg.Skills.GitHub = append(cfg.Skills.GitHub, config.GitHubSkills{URL: id.URL(), Include: selected})
		sort.Slice(cfg.Skills.GitHub, func(i, j int) bool { return cfg.Skills.GitHub[i].URL < cfg.Skills.GitHub[j].URL })
	}
	if err := chaisync.ValidateSources(cfg, home); err != nil {
		return errors.Join(err, promotion.Rollback())
	}
	if err := config.SaveAtomic(configPath, cfg); err != nil {
		return errors.Join(err, promotion.Rollback())
	}
	commitPromotion := opts.CommitPromotion
	if commitPromotion == nil {
		commitPromotion = func(promotion githubskill.Promotion) error { return promotion.Commit() }
	}
	commitErr := commitPromotion(promotion)
	syncErr := runSync(ctx, cfg, home, opts)
	if commitErr != nil {
		cleanupErr := fmt.Errorf("previous cache cleanup is incomplete: %w", commitErr)
		if syncErr != nil {
			return errors.Join(syncErr, cleanupErr)
		}
		return fmt.Errorf("source was recorded and synced, but %w", cleanupErr)
	}
	return syncErr
}

func rejectNameConflicts(selected, locals []skill.Source, cfg *config.Config, skipURL string) error {
	sources := append([]skill.Source(nil), locals...)
	for _, remote := range cfg.Skills.GitHub {
		if remote.URL == skipURL {
			continue
		}
		for _, name := range remote.Include {
			sources = append(sources, skill.Source{Name: name, Path: remote.URL})
		}
	}
	sources = append(sources, selected...)
	return skill.ValidateUniqueNames(sources)
}

func confirm(summary string, yes bool, opts Options) (bool, error) {
	fmt.Fprintln(output(opts), summary)
	if yes {
		return true, nil
	}
	if opts.Confirm != nil {
		return opts.Confirm(summary)
	}
	fmt.Fprint(output(opts), "Continue? [y/N] ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	answer := strings.TrimSpace(line)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func printDiscovery(writer io.Writer, discovery githubskill.Discovery) {
	for _, candidate := range discovery.Candidates {
		if candidate.Description == "" {
			fmt.Fprintln(writer, candidate.Name)
		} else {
			fmt.Fprintf(writer, "%s\t%s\n", candidate.Name, candidate.Description)
		}
	}
	for _, problem := range discovery.Problems {
		fmt.Fprintf(writer, "invalid\t%s\n", problem)
	}
}

func sortedUnique(values []string) []string {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	values = values[:0]
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func gitCommit(ctx context.Context, repository string) (string, error) {
	command := exec.CommandContext(ctx, "git", "-C", repository, "rev-parse", "HEAD")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("reading GitHub source commit: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func runSync(ctx context.Context, cfg *config.Config, home string, opts Options) error {
	run := opts.Sync
	if run == nil {
		run = chaisync.RunWithHome
	}
	if err := run(ctx, cfg, home, opts.SyncOptions); err != nil {
		return fmt.Errorf("source was recorded but sync is incomplete: %w", err)
	}
	return nil
}

func output(opts Options) io.Writer {
	if opts.Output != nil {
		return opts.Output
	}
	return os.Stdout
}
