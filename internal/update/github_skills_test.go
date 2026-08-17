package update

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
)

func TestGitHubSkillsModelShowsEveryConfiguredSource(t *testing.T) {
	sources := []config.GitHubSkills{
		{URL: "https://github.com/example/one", Include: []string{"selected-one"}},
		{URL: "https://github.com/example/two", Include: []string{"selected-two"}},
	}

	m := newGitHubSkillsModel(context.Background(), sources, t.TempDir(), nil)
	view := m.View()

	for _, want := range []string{"github skills", "refreshing 1/2", "example/one", "example/two"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestGitHubSkillsModelReportsResultsForEverySource(t *testing.T) {
	sources := []config.GitHubSkills{
		{URL: "https://github.com/example/one", Include: []string{"selected-one"}},
		{URL: "https://github.com/example/two", Include: []string{"selected-two"}},
	}
	m := newGitHubSkillsModel(context.Background(), sources, t.TempDir(), nil)

	updated, _ := m.Update(githubSourceDoneMsg{
		index:    0,
		duration: 1200 * time.Millisecond,
		discovery: githubskill.Discovery{Candidates: []githubskill.Candidate{
			{Name: "selected-one"},
			{Name: "new-one"},
		}},
	})
	m = updated.(githubSkillsModel)
	updated, cmd := m.Update(githubSourceDoneMsg{
		index:    1,
		duration: 800 * time.Millisecond,
		discovery: githubskill.Discovery{Candidates: []githubskill.Candidate{
			{Name: "selected-two"},
		}},
	})
	m = updated.(githubSkillsModel)

	if cmd == nil {
		t.Fatal("final source did not return a quit command")
	}
	view := m.View()
	for _, want := range []string{
		"example/one",
		"1 new skill available",
		"new-one",
		"1.2s",
		"example/two",
		"no new skills",
		"800ms",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestGitHubSkillsModelStopsOnRefreshError(t *testing.T) {
	sources := []config.GitHubSkills{{URL: "https://github.com/example/one"}}
	m := newGitHubSkillsModel(context.Background(), sources, t.TempDir(), nil)

	updated, cmd := m.Update(githubSourceDoneMsg{index: 0, err: assertError("fetch failed")})
	m = updated.(githubSkillsModel)

	if cmd == nil {
		t.Fatal("refresh error did not return a quit command")
	}
	if m.err == nil || !strings.Contains(m.err.Error(), "fetch failed") {
		t.Fatalf("error = %v", m.err)
	}
	if view := m.View(); !strings.Contains(view, "error") || !strings.Contains(view, "example/one") {
		t.Fatalf("View() does not show source error:\n%s", view)
	}
}
