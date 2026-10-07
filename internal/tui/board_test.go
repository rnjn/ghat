package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"ghtui/internal/gh"
	"ghtui/internal/poller"
	"ghtui/internal/store"
)

func TestBoardGolden(t *testing.T) {
	ctx, _ := testContext(seedStore())
	b := NewBoard()
	golden.RequireEqual(t, plain(b.View(ctx, 100, 8)))
}

func TestBoardEmpty(t *testing.T) {
	ctx, _ := testContext(seedStore())
	ctx.Store.SetRepos(nil)
	golden.RequireEqual(t, plain(NewBoard().View(ctx, 60, 4)))
}

func TestBoardCursorAndEnter(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := NewBoard()
	s, _ = s.Update(key("j"), ctx)
	s, _ = s.Update(key("down"), ctx)
	s, _ = s.Update(key("k"), ctx)
	_, cmd := s.Update(key("enter"), ctx)
	if got := pushed(t, cmd).Title(); got != "acme/web" {
		t.Fatalf("pushed %q, want acme/web (second row)", got)
	}
}

func TestBoardCursorClampsWhenReposShrink(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := NewBoard()
	for i := 0; i < 5; i++ {
		s, _ = s.Update(key("j"), ctx)
	}
	ctx.Store.SetRepos(nil)
	_ = s.View(ctx, 60, 5)
	if _, cmd := s.Update(key("enter"), ctx); cmd != nil {
		t.Fatal("enter on empty board pushed a screen")
	}
}

func TestBoardTinyAndZeroSizes(t *testing.T) {
	ctx, _ := testContext(seedStore())
	b := NewBoard()
	assertFits(t, b.View(ctx, 40, 10), 40)
	_ = b.View(ctx, 0, 0)
	_ = b.View(ctx, 3, 1)
}

func TestBoardKeepsSelectedRepoWhenOrderChanges(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := NewBoard()
	s, _ = s.Update(key("j"), ctx) // acme/web
	_ = s.View(ctx, 100, 8)
	// a run starts on me/dots... make acme/web idle-sorted below a newly active repo
	ctx.Store.SetRepos(append(ctx.Store.Repos(), store.RepoState{Repo: gh.Repo{Owner: "z", Name: "new", PushedAt: ago(5 * time.Hour)}}))
	ctx.Store.SetRuns("z/new", []gh.Run{{ID: 99, RepoKey: "z/new", Status: "in_progress", CreatedAt: ago(time.Second)}}, "")
	s, _ = s.Update(poller.RunsUpdated{RepoKey: "z/new"}, ctx)
	_ = s.View(ctx, 100, 8)
	_, cmd := s.Update(key("enter"), ctx)
	if got := pushed(t, cmd).Title(); got != "acme/web" {
		t.Fatalf("enter opened %q, want acme/web", got)
	}
}

func TestBoardShowsLoadingUntilPolled(t *testing.T) {
	ctx, _ := testContext(store.New())
	ctx.Store.SetRepos([]store.RepoState{{Repo: gh.Repo{Owner: "z", Name: "fresh"}}})
	v := plain(NewBoard().View(ctx, 80, 5))
	if !strings.Contains(v, "loading…") || strings.Contains(v, "no runs") {
		t.Fatalf("view:\n%s", v)
	}
	ctx.Store.SetRuns("z/fresh", nil, "")
	if v := plain(NewBoard().View(ctx, 80, 5)); !strings.Contains(v, "no runs") {
		t.Fatalf("after poll:\n%s", v)
	}
}
