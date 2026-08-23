package githubskill

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshFromURLPromotesCompleteCache(t *testing.T) {
	source := t.TempDir()
	runGit(t, source, "init", "-b", "main")
	runGit(t, source, "config", "uploadpack.allowFilter", "true")
	writeRepoFile(t, source, "skills/chosen/SKILL.md", "---\nname: chosen\n---\n")
	writeRepoFile(t, source, "skills/ignored/SKILL.md", "---\nname: ignored\n---\n")
	commitRepo(t, source, "skills")
	home := t.TempDir()
	id, _ := ParseCanonical("https://github.com/example/skills")
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()

	discovery, err := refreshFromURL(context.Background(), home, id, cloneURL, []string{"chosen"})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Candidates) != 2 {
		t.Fatalf("candidates = %#v", discovery.Candidates)
	}
	if _, err := ResolveCached(home, id, []string{"chosen"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(CacheDir(home, id), repositoryDirName, "skills", "ignored")); !os.IsNotExist(err) {
		t.Fatalf("unselected skill was cached: %v", err)
	}
}

func TestAcquireFromURLOwnsRollbackAndProgress(t *testing.T) {
	source := t.TempDir()
	runGit(t, source, "init", "-b", "main")
	runGit(t, source, "config", "uploadpack.allowFilter", "true")
	writeRepoFile(t, source, "skills/chosen/SKILL.md", "---\nname: chosen\n---\nnew cache\n")
	commitRepo(t, source, "skills")

	home := t.TempDir()
	id, _ := ParseCanonical("https://github.com/example/skills")
	cache := CacheDir(home, id)
	writeCacheSkill(t, RepositoryDir(cache), "chosen", "chosen")
	if err := os.WriteFile(filepath.Join(RepositoryDir(cache), "chosen", "SKILL.md"), []byte("---\nname: chosen\n---\nold cache\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CompleteStaging(cache, id, map[string]string{"chosen": "chosen"}, "old-commit"); err != nil {
		t.Fatal(err)
	}

	var phases []AcquisitionPhase
	callbackErr := assertError("manifest write failed")
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	_, err := acquireFromURL(
		context.Background(),
		home,
		id,
		cloneURL,
		func(Discovery) (AcquisitionDecision, error) {
			return AcquisitionDecision{
				Names:   []string{"chosen"},
				Install: func() error { return callbackErr },
			}, nil
		},
		func(phase AcquisitionPhase, _ int, operation func() error) error {
			phases = append(phases, phase)
			return operation()
		},
	)
	if !errors.Is(err, callbackErr) {
		t.Fatalf("error = %v, want %v", err, callbackErr)
	}
	if len(phases) != 2 || phases[0] != AcquisitionInspecting || phases[1] != AcquisitionFetching {
		t.Fatalf("phases = %v", phases)
	}
	sources, err := ResolveCached(home, id, []string{"chosen"})
	if err != nil {
		t.Fatalf("previous cache was not restored: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(sources[0].Path, "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "old cache") {
		t.Fatalf("restored content = %q, err=%v", data, err)
	}
}

func TestResolveCached(t *testing.T) {
	home := t.TempDir()
	id, err := ParseCanonical("https://github.com/example/skills")
	if err != nil {
		t.Fatal(err)
	}
	cache := CacheDir(home, id)
	repository := filepath.Join(cache, repositoryDirName)
	writeCacheSkill(t, repository, "nested/renamed", "chosen")
	if err := CompleteStaging(cache, id, map[string]string{"chosen": "nested/renamed"}, "abc123"); err != nil {
		t.Fatal(err)
	}

	sources, err := ResolveCached(home, id, []string{"chosen"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Name != "chosen" || sources[0].Path != filepath.Join(repository, "nested", "renamed") {
		t.Fatalf("sources = %#v", sources)
	}
}

func TestResolveCachedRejectsDeletedAsset(t *testing.T) {
	home := t.TempDir()
	id, _ := ParseCanonical("https://github.com/example/skills")
	cache := CacheDir(home, id)
	repository := RepositoryDir(cache)
	writeCacheSkill(t, repository, "chosen", "chosen")
	asset := filepath.Join(repository, "chosen", "asset.txt")
	if err := os.WriteFile(asset, []byte("asset"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CompleteStaging(cache, id, map[string]string{"chosen": "chosen"}, "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(asset); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCached(home, id, []string{"chosen"}); err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("deleted asset error = %v", err)
	}
}

func TestResolveCachedRejectsIncompleteCache(t *testing.T) {
	home := t.TempDir()
	id, _ := ParseCanonical("https://github.com/example/skills")

	if _, err := ResolveCached(home, id, []string{"chosen"}); err == nil || !strings.Contains(err.Error(), "run 'chai update'") {
		t.Fatalf("missing cache error = %v", err)
	}

	cache := CacheDir(home, id)
	writeCacheState(t, cache, cacheState{URL: id.URL(), Commit: "abc123", Skills: map[string]string{"chosen": "missing"}})
	if _, err := ResolveCached(home, id, []string{"chosen"}); err == nil || !strings.Contains(err.Error(), "chosen") {
		t.Fatalf("partial cache error = %v", err)
	}
}

func TestResolveCachedReportsAllIncompleteSkills(t *testing.T) {
	home := t.TempDir()
	id, _ := ParseCanonical("https://github.com/example/skills")
	cache := CacheDir(home, id)
	repository := RepositoryDir(cache)
	writeCacheSkill(t, repository, "deleted", "deleted")
	writeCacheSkill(t, repository, "corrupt", "corrupt")
	if err := CompleteStaging(cache, id, map[string]string{
		"deleted": "deleted",
		"corrupt": "corrupt",
	}, "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(repository, "deleted")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "corrupt", "SKILL.md"), []byte("corrupt"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ResolveCached(home, id, []string{"missing", "deleted", "corrupt"})
	if err == nil {
		t.Fatal("expected incomplete cache error")
	}
	for _, detail := range []string{
		id.URL(),
		`skill "missing" is missing`,
		`skill "deleted" is incomplete`,
		`skill "corrupt" content does not match its cache state`,
		"run 'chai update'",
	} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("error %q does not contain %q", err, detail)
		}
	}
}

func TestPromoteReplacesExistingCache(t *testing.T) {
	home := t.TempDir()
	id, _ := ParseCanonical("https://github.com/example/skills")
	final := CacheDir(home, id)
	if err := os.MkdirAll(final, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(final, "old"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	staging, err := NewStaging(home, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "new"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	promotion, err := BeginPromotion(staging, final)
	if err != nil {
		t.Fatal(err)
	}
	if err := promotion.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(final, "new")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(final, "old")); !os.IsNotExist(err) {
		t.Fatalf("old cache survived promotion: %v", err)
	}
}

func TestBeginPromotionRestoresExistingCacheAfterPromotionFailure(t *testing.T) {
	root := t.TempDir()
	final := filepath.Join(root, "cache")
	staging := filepath.Join(root, "staging")
	if err := os.Mkdir(final, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staging, 0755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	rename := func(old, new string) error {
		calls++
		if calls == 2 || calls == 3 {
			return assertError("injected rename failure")
		}
		return os.Rename(old, new)
	}
	if _, err := beginPromotion(staging, final, rename); err == nil || !strings.Contains(err.Error(), "promoting") {
		t.Fatalf("promotion error = %v", err)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("existing cache was not restored after retry: %v", err)
	}
}

func TestBeginPromotionPreservesOrphanedBackupAfterPromotionFailure(t *testing.T) {
	root := t.TempDir()
	final := filepath.Join(root, "cache")
	backup := final + ".backup"
	staging := filepath.Join(root, "staging")
	if err := os.Mkdir(backup, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "old"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staging, 0755); err != nil {
		t.Fatal(err)
	}

	calls := 0
	rename := func(old, new string) error {
		calls++
		if calls == 3 {
			return assertError("injected promotion failure")
		}
		return os.Rename(old, new)
	}
	if _, err := beginPromotion(staging, final, rename); err == nil || !strings.Contains(err.Error(), "promoting") {
		t.Fatalf("promotion error = %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(final, "old")); err != nil || string(data) != "old" {
		t.Fatalf("orphaned backup was not preserved: data=%q err=%v", data, err)
	}
}

func TestBeginPromotionReportsPersistentRestoreFailure(t *testing.T) {
	root := t.TempDir()
	final := filepath.Join(root, "cache")
	staging := filepath.Join(root, "staging")
	if err := os.Mkdir(final, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staging, 0755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	rename := func(old, new string) error {
		calls++
		if calls >= 2 {
			return assertError("persistent rename failure")
		}
		return os.Rename(old, new)
	}
	_, err := beginPromotion(staging, final, rename)
	if err == nil || !strings.Contains(err.Error(), "promoting") || !strings.Contains(err.Error(), "restoring") {
		t.Fatalf("joined recovery error = %v", err)
	}
	if _, err := os.Stat(final + ".backup"); err != nil {
		t.Fatalf("previous cache backup was not preserved: %v", err)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

func writeCacheSkill(t *testing.T, repository, relative, name string) {
	t.Helper()
	dir := filepath.Join(repository, filepath.FromSlash(relative))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeCacheState(t *testing.T, root string, state cacheState) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, stateFileName), data, 0644); err != nil {
		t.Fatal(err)
	}
}
