package githubskill

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	minimumGitMajor = 2
	minimumGitMinor = 37
)

var gitVersionPattern = regexp.MustCompile(`^git version ([0-9]+)\.([0-9]+)(?:\.([0-9]+))?(?:[. ].*)?$`)

type Version struct {
	Major int
	Minor int
	Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func CheckGit(ctx context.Context) (Version, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return Version{}, fmt.Errorf("Git 2.37 or newer is required, but git was not found in PATH; %s", gitInstallHint())
	}
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return Version{}, fmt.Errorf("checking Git version: %w", err)
	}
	version, err := ParseGitVersion(strings.TrimSpace(string(output)))
	if err != nil {
		return Version{}, err
	}
	if err := requireGitVersion(version); err != nil {
		return Version{}, err
	}
	return version, nil
}

func ParseGitVersion(output string) (Version, error) {
	matches := gitVersionPattern.FindStringSubmatch(strings.TrimSpace(output))
	if matches == nil {
		return Version{}, fmt.Errorf("cannot parse Git version output %q; Git 2.37 or newer is required", output)
	}
	values := [3]int{}
	for i := range values {
		if matches[i+1] == "" {
			continue
		}
		value, err := strconv.Atoi(matches[i+1])
		if err != nil {
			return Version{}, fmt.Errorf("cannot parse Git version output %q: %w", output, err)
		}
		values[i] = value
	}
	return Version{Major: values[0], Minor: values[1], Patch: values[2]}, nil
}

func requireGitVersion(version Version) error {
	if version.Major > minimumGitMajor || version.Major == minimumGitMajor && version.Minor >= minimumGitMinor {
		return nil
	}
	return fmt.Errorf("Git 2.37 or newer is required, detected %s; %s", version, gitInstallHint())
}

func gitInstallHint() string {
	if runtime.GOOS == "darwin" {
		return "install it with 'brew install git'"
	}
	return "install it from https://git-scm.com/downloads"
}
