package tui

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
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
