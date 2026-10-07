package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rnjn/ghtui/internal/gh"
	"github.com/rnjn/ghtui/internal/store"
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func seeded() *store.Store {
	st := store.New()
	st.SetRepos([]store.RepoState{
		{Repo: gh.Repo{Owner: "acme", Name: "api", PushedAt: now.Add(-time.Hour), DefaultBranch: "trunk"}, Pinned: true},
		{Repo: gh.Repo{Owner: "acme", Name: "empty", PushedAt: now.Add(-2 * time.Hour)}},
		{Repo: gh.Repo{Owner: "acme", Name: "fresh", PushedAt: now.Add(-3 * time.Hour)}},
	})
	st.SetRuns("acme/api", []gh.Run{
		{ID: 2, RepoKey: "acme/api", Number: 9, WorkflowName: "CI", WorkflowID: 5, Branch: "main", Event: "push", Actor: "al",
			Status: "completed", Conclusion: "success", CreatedAt: now.Add(-time.Minute), UpdatedAt: now, HTMLURL: "https://x/2"},
		{ID: 1, RepoKey: "acme/api", Number: 8, Status: "completed", Conclusion: "failure", CreatedAt: now.Add(-time.Hour)},
	}, `"etag-api"`)
	st.SetRuns("acme/empty", nil, `"etag-empty"`)
	return st
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, FromStore(seeded(), now)); err != nil {
		t.Fatal(err)
	}
	snap, ok := Load(dir)
	if !ok || !snap.SavedAt.Equal(now) {
		t.Fatalf("ok=%v savedAt=%v", ok, snap.SavedAt)
	}
	st := store.New()
	snap.Apply(st)
	api, _ := st.Repo("acme/api")
	if !api.Pinned || api.RunsETag != `"etag-api"` || api.Repo.DefaultBranch != "trunk" || !api.Polled {
		t.Fatalf("api = %+v", api)
	}
	runs := st.Runs("acme/api")
	if len(runs) != 2 || runs[0].ID != 2 || runs[0].WorkflowName != "CI" || runs[0].Conclusion != "success" ||
		runs[0].HTMLURL != "https://x/2" || runs[0].WorkflowID != 5 || !runs[0].UpdatedAt.Equal(now) {
		t.Fatalf("runs = %+v", runs)
	}
	if e, _ := st.Repo("acme/empty"); !e.Polled || e.RunsETag != `"etag-empty"` {
		t.Fatalf("empty = %+v (polled with zero runs must stay polled)", e)
	}
	if f, _ := st.Repo("acme/fresh"); f.Polled {
		t.Fatal("never-polled repo restored as polled")
	}
}

func TestLoadMisses(t *testing.T) {
	cases := map[string]func(dir string){
		"missing":   func(string) {},
		"truncated": func(d string) { _ = os.WriteFile(filepath.Join(d, fileName), []byte(`{"version":1,"repos":[`), 0o600) },
		"version":   func(d string) { _ = os.WriteFile(filepath.Join(d, fileName), []byte(`{"version":999}`), 0o600) },
		"directory": func(d string) { _ = os.Mkdir(filepath.Join(d, fileName), 0o700) },
		"garbage":   func(d string) { _ = os.WriteFile(filepath.Join(d, fileName), []byte("\x00\x01"), 0o600) },
	}
	for name, setup := range cases {
		dir := t.TempDir()
		setup(dir)
		if _, ok := Load(dir); ok {
			t.Errorf("%s: Load reported a hit", name)
		}
	}
}

func TestSaveUnwritableKeepsOldFileAndNoTemp(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, FromStore(seeded(), now)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()
	if err := Save(dir, FromStore(store.New(), now.Add(time.Hour))); err == nil {
		t.Fatal("Save into a read-only dir succeeded")
	}
	_ = os.Chmod(dir, 0o700)
	snap, ok := Load(dir)
	if !ok || !snap.SavedAt.Equal(now) {
		t.Fatal("old cache damaged by a failed save")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestSaveCreatesPrivateDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "ghtui")
	if err := Save(dir, FromStore(seeded(), now)); err != nil {
		t.Fatal(err)
	}
	di, _ := os.Stat(dir)
	fi, _ := os.Stat(filepath.Join(dir, fileName))
	if di.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("dir %v file %v", di.Mode().Perm(), fi.Mode().Perm())
	}
}

func TestDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	if Dir() != "/tmp/xdg/ghtui" {
		t.Fatalf("Dir = %s", Dir())
	}
	t.Setenv("XDG_CACHE_HOME", "")
	home, _ := os.UserHomeDir()
	if Dir() != filepath.Join(home, ".cache", "ghtui") {
		t.Fatalf("Dir = %s", Dir())
	}
}
