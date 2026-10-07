package store

import (
	"fmt"
	"testing"

	"github.com/rnjn/ghtui/internal/tail"
)

func lines(texts ...string) []tail.LogLine {
	out := make([]tail.LogLine, len(texts))
	for i, t := range texts {
		out[i] = tail.LogLine{Text: t}
	}
	return out
}

func TestSetLogAppendsAndReturnsRange(t *testing.T) {
	s := New()
	if from, to := s.SetLog(1, lines("a", "b"), false); from != 0 || to != 2 {
		t.Fatalf("range %d..%d", from, to)
	}
	before := s.Log(1)
	if from, to := s.SetLog(1, lines("x", "y", "c"), false); from != 2 || to != 3 {
		t.Fatalf("range %d..%d", from, to)
	}
	got := s.Log(1)
	if len(got.Lines) != 3 || got.Lines[0].Text != "a" || got.Lines[2].Text != "c" {
		t.Fatalf("lines = %+v (existing prefix must be kept)", got.Lines)
	}
	if before.Lines[0].Text != "a" || len(before.Lines) != 2 {
		t.Fatal("earlier snapshot changed")
	}
}

func TestSetLogShorterOrEqualIsNoop(t *testing.T) {
	s := New()
	s.SetLog(1, lines("a", "b"), false)
	for _, in := range [][]tail.LogLine{lines("a", "b"), lines("a")} {
		if from, to := s.SetLog(1, in, false); from != to {
			t.Fatalf("non-empty range %d..%d", from, to)
		}
	}
	if len(s.Log(1).Lines) != 2 {
		t.Fatal("lines changed")
	}
}

func TestSetLogCompleteSticks(t *testing.T) {
	s := New()
	s.SetLog(1, lines("a"), true)
	s.SetLog(1, lines("a"), false)
	if !s.Log(1).Complete {
		t.Fatal("Complete reset")
	}
}

func TestLogUnknownJob(t *testing.T) {
	if b := New().Log(9); b.JobID != 9 || len(b.Lines) != 0 || b.Complete {
		t.Fatalf("got %+v", b)
	}
}

func TestDropLog(t *testing.T) {
	s := New()
	s.SetLog(1, lines("a"), true)
	s.DropLog(1)
	if len(s.Log(1).Lines) != 0 {
		t.Fatal("not dropped")
	}
}

func TestLogBuffersBoundedToFiveMostRecent(t *testing.T) {
	s := New()
	for id := int64(1); id <= 6; id++ {
		s.SetLog(id, lines(fmt.Sprint(id)), true)
	}
	if len(s.Log(1).Lines) != 0 {
		t.Fatal("oldest buffer kept")
	}
	for id := int64(2); id <= 6; id++ {
		if len(s.Log(id).Lines) != 1 {
			t.Fatalf("buffer %d dropped", id)
		}
	}
	// touching 2 makes it recent; adding 7 evicts 3
	s.SetLog(2, lines("2", "more"), true)
	s.SetLog(7, lines("7"), true)
	if len(s.Log(2).Lines) == 0 || len(s.Log(3).Lines) != 0 {
		t.Fatal("LRU order not respected")
	}
}
