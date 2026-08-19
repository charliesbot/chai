package githubskill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/skill"
)

const (
	repositoryDirName = "repository"
	stateFileName     = "state.json"
)

type cacheState struct {
	URL    string                 `json:"url"`
	Commit string                 `json:"commit"`
	Skills map[string]string      `json:"skills"`
	Files  map[string][]cacheFile `json:"files"`
}

type cacheFile struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	Executable bool   `json:"executable,omitempty"`
}

// AcquisitionPhase identifies a user-visible step in source acquisition.
type AcquisitionPhase int

const (
	AcquisitionInspecting AcquisitionPhase = iota
	AcquisitionFetching
)

// AcquisitionDecision selects discovered skills and optionally coordinates a
// manifest write with cache promotion.
type AcquisitionDecision struct {
	Names        []string
	BeforeCommit func() error
}

// AcquisitionDecisionFunc decides what to cache after repository discovery.
type AcquisitionDecisionFunc func(Discovery) (AcquisitionDecision, error)

// AcquisitionProgressFunc runs one acquisition phase with caller-owned progress presentation.
type AcquisitionProgressFunc func(AcquisitionPhase, int, func() error) error

// AcquireFunc is the complete source-acquisition seam used by command modules.
type AcquireFunc func(
	context.Context,
	string,
	Identity,
	AcquisitionDecisionFunc,
	AcquisitionProgressFunc,
) (Discovery, error)

// Acquire discovers a source, materializes the selected skills, and owns cache
// promotion, manifest coordination, rollback, and cleanup.
func Acquire(
	ctx context.Context,
	home string,
	id Identity,
	decide AcquisitionDecisionFunc,
	progress AcquisitionProgressFunc,
) (Discovery, error) {
	if _, err := CheckGit(ctx); err != nil {
		return Discovery{}, err
	}
	return acquireFromURL(ctx, home, id, id.URL(), decide, progress)
}

func acquireFromURL(
	ctx context.Context,
	home string,
	id Identity,
	cloneURL string,
	decide AcquisitionDecisionFunc,
	progress AcquisitionProgressFunc,
) (Discovery, error) {
	staging, err := os.MkdirTemp("", "chai-github-source-")
	if err != nil {
		return Discovery{}, err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	repository := RepositoryDir(staging)
	var discovery Discovery
	if err := runAcquisitionPhase(progress, AcquisitionInspecting, 0, func() error {
		var discoverErr error
		discovery, discoverErr = Discover(ctx, cloneURL, repository)
		return discoverErr
	}); err != nil {
		return Discovery{}, err
	}
	decision, err := decide(discovery)
	if err != nil || len(decision.Names) == 0 {
		return discovery, err
	}

	var selected map[string]string
	if err := runAcquisitionPhase(progress, AcquisitionFetching, len(decision.Names), func() error {
		var materializeErr error
		selected, materializeErr = Materialize(ctx, repository, discovery, decision.Names)
		return materializeErr
	}); err != nil {
		return discovery, err
	}
	commitBytes, err := gitOutput(ctx, repository, "rev-parse", "HEAD")
	if err != nil {
		return discovery, err
	}
	if err := CompleteStaging(staging, id, selected, strings.TrimSpace(string(commitBytes))); err != nil {
		return discovery, err
	}
	prepared, err := stagePrepared(home, id, staging)
	if err != nil {
		return discovery, err
	}
	staging = prepared
	promotion, err := BeginPromotion(staging, CacheDir(home, id))
	if err != nil {
		return discovery, err
	}
	if decision.BeforeCommit != nil {
		if err := decision.BeforeCommit(); err != nil {
			return discovery, errors.Join(err, promotion.Rollback())
		}
	}
	if err := promotion.Commit(); err != nil {
		return discovery, &CleanupError{Err: err}
	}
	return discovery, nil
}

func runAcquisitionPhase(progress AcquisitionProgressFunc, phase AcquisitionPhase, count int, operation func() error) error {
	if progress == nil {
		return operation()
	}
	return progress(phase, count, operation)
}

func Refresh(ctx context.Context, home string, id Identity, names []string) (Discovery, error) {
	if _, err := CheckGit(ctx); err != nil {
		return Discovery{}, err
	}
	return refreshFromURL(ctx, home, id, id.URL(), names)
}

func refreshFromURL(ctx context.Context, home string, id Identity, cloneURL string, names []string) (Discovery, error) {
	return acquireFromURL(ctx, home, id, cloneURL, func(Discovery) (AcquisitionDecision, error) {
		return AcquisitionDecision{Names: names}, nil
	}, nil)
}

type CleanupError struct {
	Err error
}

func (err *CleanupError) Error() string {
	return err.Err.Error()
}

func (err *CleanupError) Unwrap() error {
	return err.Err
}

func NewStaging(home string, id Identity) (string, error) {
	final := CacheDir(home, id)
	parent := filepath.Dir(final)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", fmt.Errorf("creating GitHub source cache parent: %w", err)
	}
	staging, err := os.MkdirTemp(parent, "."+id.repository+".staging-")
	if err != nil {
		return "", fmt.Errorf("creating GitHub source staging directory: %w", err)
	}
	return staging, nil
}

func stagePrepared(home string, id Identity, prepared string) (string, error) {
	staging, err := NewStaging(home, id)
	if err != nil {
		return "", err
	}
	if err := os.Remove(staging); err != nil {
		_ = os.RemoveAll(staging)
		return "", fmt.Errorf("preparing GitHub source staging path: %w", err)
	}
	if err := os.Rename(prepared, staging); err == nil {
		return staging, nil
	}
	if err := copyPreparedTree(prepared, staging); err != nil {
		_ = os.RemoveAll(staging)
		return "", fmt.Errorf("copying prepared GitHub source cache: %w", err)
	}
	if err := os.RemoveAll(prepared); err != nil {
		_ = os.RemoveAll(staging)
		return "", fmt.Errorf("removing temporary GitHub source cache: %w", err)
	}
	return staging, nil
}

func copyPreparedTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("prepared cache contains non-regular file %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tmp := target + ".tmp"
		if err := os.WriteFile(tmp, data, info.Mode().Perm()); err != nil {
			return err
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return nil
	})
}

func RepositoryDir(cacheRoot string) string {
	return filepath.Join(cacheRoot, repositoryDirName)
}

func CompleteStaging(root string, id Identity, selected map[string]string, commit string) error {
	files := make(map[string][]cacheFile, len(selected))
	for name, directory := range selected {
		manifest, err := cacheFiles(filepath.Join(RepositoryDir(root), filepath.FromSlash(directory)))
		if err != nil {
			return fmt.Errorf("recording cached skill %q: %w", name, err)
		}
		files[name] = manifest
	}
	state := cacheState{URL: id.URL(), Commit: commit, Skills: selected, Files: files}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding GitHub source cache state: %w", err)
	}
	data = append(data, '\n')
	tmp := filepath.Join(root, stateFileName+".tmp")
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing GitHub source cache state: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(root, stateFileName)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("committing GitHub source cache state: %w", err)
	}
	return nil
}

type Promotion struct {
	final    string
	backup   string
	hadFinal bool
}

func BeginPromotion(staging, final string) (Promotion, error) {
	return beginPromotion(staging, final, os.Rename)
}

func beginPromotion(staging, final string, rename func(string, string) error) (Promotion, error) {
	promotion := Promotion{final: final, backup: final + ".backup"}
	if filepath.Dir(staging) != filepath.Dir(final) {
		return Promotion{}, fmt.Errorf("GitHub source staging directory must be beside its cache")
	}
	_, statErr := os.Stat(final)
	promotion.hadFinal = statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return Promotion{}, fmt.Errorf("checking existing GitHub source cache: %w", statErr)
	}
	if promotion.hadFinal {
		if err := os.RemoveAll(promotion.backup); err != nil {
			return Promotion{}, fmt.Errorf("removing old GitHub source cache backup: %w", err)
		}
	} else if _, backupErr := os.Stat(promotion.backup); backupErr == nil {
		if err := rename(promotion.backup, final); err != nil {
			return Promotion{}, fmt.Errorf("restoring orphaned GitHub source cache backup: %w", err)
		}
		promotion.hadFinal = true
	} else if !os.IsNotExist(backupErr) {
		return Promotion{}, fmt.Errorf("checking GitHub source cache backup: %w", backupErr)
	}
	if promotion.hadFinal {
		if err := rename(final, promotion.backup); err != nil {
			return Promotion{}, fmt.Errorf("staging existing GitHub source cache: %w", err)
		}
	}
	if err := rename(staging, final); err != nil {
		promotionErr := fmt.Errorf("promoting GitHub source cache: %w", err)
		if promotion.hadFinal {
			restoreErr := rename(promotion.backup, final)
			if restoreErr != nil {
				// A transient filesystem error should not strand a valid backup.
				restoreErr = rename(promotion.backup, final)
			}
			if restoreErr != nil {
				return Promotion{}, errors.Join(promotionErr, fmt.Errorf("restoring previous GitHub source cache from %s: %w", promotion.backup, restoreErr))
			}
		}
		return Promotion{}, promotionErr
	}
	return promotion, nil
}

func (promotion Promotion) Commit() error {
	if !promotion.hadFinal {
		return nil
	}
	if err := os.RemoveAll(promotion.backup); err != nil {
		return fmt.Errorf("removing previous GitHub source cache: %w", err)
	}
	return nil
}

func (promotion Promotion) Rollback() error {
	if err := os.RemoveAll(promotion.final); err != nil {
		return fmt.Errorf("removing uncommitted GitHub source cache: %w", err)
	}
	if promotion.hadFinal {
		if err := os.Rename(promotion.backup, promotion.final); err != nil {
			return fmt.Errorf("restoring previous GitHub source cache: %w", err)
		}
	}
	return nil
}

func ResolveCached(home string, id Identity, names []string) ([]skill.Source, error) {
	root := CacheDir(home, id)
	data, err := os.ReadFile(filepath.Join(root, stateFileName))
	if err != nil {
		return nil, incompleteCacheError(id, fmt.Sprintf("cache state is missing for skills %s", strings.Join(names, ", ")))
	}
	var state cacheState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, incompleteCacheError(id, fmt.Sprintf("cache state is invalid for skills %s", strings.Join(names, ", ")))
	}
	if state.URL != id.URL() || state.Commit == "" {
		return nil, incompleteCacheError(id, fmt.Sprintf("cache identity or commit is invalid for skills %s", strings.Join(names, ", ")))
	}
	repository := RepositoryDir(root)
	sources := make([]skill.Source, 0, len(names))
	seen := make(map[string]bool, len(names))
	var problems []string
	for _, name := range names {
		if seen[name] {
			return nil, fmt.Errorf("remote skill %q is selected more than once", name)
		}
		seen[name] = true
		directory, ok := state.Skills[name]
		if !ok || !safeGitPath(directory) {
			problems = append(problems, fmt.Sprintf("skill %q is missing", name))
			continue
		}
		if err := validateMaterializedTree(repository, directory); err != nil {
			problems = append(problems, fmt.Sprintf("skill %q is incomplete", name))
			continue
		}
		path := filepath.Join(repository, filepath.FromSlash(directory))
		files, err := cacheFiles(path)
		if err != nil || !reflect.DeepEqual(files, state.Files[name]) {
			problems = append(problems, fmt.Sprintf("skill %q content does not match its cache state", name))
			continue
		}
		data, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
		if err != nil {
			problems = append(problems, fmt.Sprintf("skill %q metadata is missing", name))
			continue
		}
		metadata, err := skill.ParseMetadata(data)
		if err != nil || metadata.Name != name {
			problems = append(problems, fmt.Sprintf("skill %q metadata is invalid", name))
			continue
		}
		sources = append(sources, skill.Source{Name: name, Path: path})
	}
	if len(problems) > 0 {
		return nil, incompleteCacheError(id, strings.Join(problems, "; "))
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	return sources, nil
}

func cacheFiles(root string) ([]cacheFile, error) {
	var files []cacheFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link %s is not allowed", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular file %s is not allowed", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, cacheFile{
			Path: filepath.ToSlash(relative), Hash: hash.Sum(data), Executable: info.Mode()&0111 != 0,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func incompleteCacheError(id Identity, reason string) error {
	return fmt.Errorf("GitHub skill source %s is incomplete: %s; run 'chai update'", id.URL(), strings.TrimSpace(reason))
}
