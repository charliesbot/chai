package update

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	chaisync "github.com/charliesbot/chai/internal/sync"
)

func TestGitHubSkillsUpdateRefreshesAtMostThreeSourcesConcurrently(t *testing.T) {
	var sources []config.GitHubSkills
	for _, name := range []string{"one", "two", "three", "four", "five"} {
		sources = append(sources, config.GitHubSkills{URL: "https://github.com/example/" + name})
	}

	started := make(chan string, len(sources))
	release := make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	var active, maximum int32
	refresh := func(_ context.Context, _ string, id githubskill.Identity, _ []string) (githubskill.Discovery, error) {
		current := atomic.AddInt32(&active, 1)
		for {
			previous := atomic.LoadInt32(&maximum)
			if current <= previous || atomic.CompareAndSwapInt32(&maximum, previous, current) {
				break
			}
		}
		started <- id.Repository()
		<-release
		atomic.AddInt32(&active, -1)
		return githubskill.Discovery{}, nil
	}

	done := make(chan error, 1)
	home := t.TempDir()
	go func() {
		_, err := runGitHubSkillsUpdate(context.Background(), sources, home, refresh)
		done <- err
	}()

	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("only %d refreshes started before the concurrency limit", i)
		}
	}
	select {
	case name := <-started:
		t.Fatalf("fourth refresh %q started before a slot was available", name)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	released = true
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&maximum); got != 3 {
		t.Fatalf("maximum concurrent refreshes = %d, want 3", got)
	}
}

func TestGitHubSkillsModelShowsEveryConfiguredSource(t *testing.T) {
	sources := []config.GitHubSkills{
		{URL: "https://github.com/example/one", Include: []string{"selected-one"}},
		{URL: "https://github.com/example/two", Include: []string{"selected-two"}},
	}

	m := newGitHubSkillsModel(context.Background(), sources, t.TempDir(), nil)
	view := m.View()

	for _, want := range []string{"github skills", "refreshing · 0/2 complete", "example/one", "example/two"} {
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
		index:    1,
		duration: 800 * time.Millisecond,
	})
	m = updated.(githubSkillsModel)
	updated, cmd := m.Update(githubSourceDoneMsg{
		index:    0,
		duration: 1200 * time.Millisecond,
	})
	m = updated.(githubSkillsModel)

	if cmd == nil {
		t.Fatal("final source did not return a quit command")
	}
	if view := m.View(); view != "" {
		t.Fatalf("completed live view should be cleared, got:\n%s", view)
	}
	report := m.report()
	for _, want := range []string{
		"example/one",
		"refreshed",
		"1.2s",
		"example/two",
		"800ms",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report() missing %q:\n%s", want, report)
		}
	}
	if strings.Index(report, "example/one") > strings.Index(report, "example/two") {
		t.Fatalf("report() does not preserve config order after out-of-order completion:\n%s", report)
	}
	for _, unwanted := range []string{"new skill", "new-one"} {
		if strings.Contains(report, unwanted) {
			t.Fatalf("report() contains unselected-skill detail %q:\n%s", unwanted, report)
		}
	}
}

func TestGitHubSkillsModelReportsOnlyRefreshStatus(t *testing.T) {
	sources := []config.GitHubSkills{{URL: "https://github.com/example/one", Include: []string{"selected-one"}}}
	refresh := func(context.Context, string, githubskill.Identity, []string) (githubskill.Discovery, error) {
		return githubskill.Discovery{Candidates: []githubskill.Candidate{
			{Name: "selected-one"},
			{Name: "new-one"},
		}}, nil
	}
	m := newGitHubSkillsModel(context.Background(), sources, t.TempDir(), refresh)

	message := m.startSource(0)().(githubSourceDoneMsg)
	updated, _ := m.Update(message)
	m = updated.(githubSkillsModel)

	report := m.report()
	if !strings.Contains(report, "refreshed") || strings.Contains(report, "new-one") {
		t.Fatalf("report should show only refresh status:\n%s", report)
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
	if view := m.View(); view != "" {
		t.Fatalf("failed live view should be cleared, got:\n%s", view)
	}
	if report := m.report(); !strings.Contains(report, "error") || !strings.Contains(report, "example/one") {
		t.Fatalf("report() does not show source error:\n%s", report)
	}
}

func TestGitHubSkillsModelContinuesAfterMissingConfiguredSkills(t *testing.T) {
	sources := []config.GitHubSkills{
		{URL: "https://github.com/android/skills", Include: []string{"perfetto-sql"}},
		{URL: "https://github.com/example/two"},
		{URL: "https://github.com/example/three"},
		{URL: "https://github.com/example/four"},
	}
	m := newGitHubSkillsModel(context.Background(), sources, t.TempDir(), nil)

	updated, cmd := m.Update(githubSourceDoneMsg{index: 0, err: &githubskill.MissingSkillsError{Names: []string{"perfetto-sql"}}})
	m = updated.(githubSkillsModel)
	if cmd == nil {
		t.Fatal("missing-skill failure did not start the waiting source")
	}
	if m.done {
		t.Fatal("missing-skill failure stopped remaining refreshes")
	}
	for _, index := range []int{1, 2, 3} {
		updated, _ = m.Update(githubSourceDoneMsg{index: index})
		m = updated.(githubSkillsModel)
	}
	if !m.done {
		t.Fatal("model did not finish after every source completed")
	}
	var missing *githubskill.MissingSkillsError
	if !errors.As(m.err, &missing) {
		t.Fatalf("final error = %v, want MissingSkillsError", m.err)
	}
	report := m.report()
	for _, want := range []string{"perfetto-sql", "Existing cache retained", "chai add android/skills", "example/four"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRunWithHomeSyncsAfterMissingConfiguredSkills(t *testing.T) {
	cfg := &config.Config{Skills: config.Skills{GitHub: []config.GitHubSkills{
		{URL: "https://github.com/android/skills", Include: []string{"perfetto-sql"}},
		{URL: "https://github.com/example/other", Include: []string{"other"}},
	}}}
	var refreshed atomic.Int32
	synced := false
	err := RunWithHome(context.Background(), cfg, t.TempDir(), Options{
		CheckGit: func(context.Context) error { return nil },
		Refresh: func(_ context.Context, _ string, id githubskill.Identity, _ []string) (githubskill.Discovery, error) {
			refreshed.Add(1)
			if id.Owner() == "android" {
				return githubskill.Discovery{}, &githubskill.MissingSkillsError{Names: []string{"perfetto-sql"}}
			}
			return githubskill.Discovery{}, nil
		},
		Sync: func(context.Context, *config.Config, string, chaisync.Options) error {
			synced = true
			return nil
		},
	})
	var missing *githubskill.MissingSkillsError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %v, want MissingSkillsError", err)
	}
	if refreshed.Load() != 2 {
		t.Fatalf("refresh count = %d, want 2", refreshed.Load())
	}
	if !synced {
		t.Fatal("sync did not run after recoverable refresh failure")
	}
}
