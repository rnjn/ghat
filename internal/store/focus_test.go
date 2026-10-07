package store

import "testing"

func TestFocusRun(t *testing.T) {
	s := New()
	if s.FocusRun() != 0 {
		t.Fatal("focus set initially")
	}
	s.SetFocusRun(7)
	if s.FocusRun() != 7 {
		t.Fatal("focus not set")
	}
	s.SetFocusRun(0)
	if s.FocusRun() != 0 {
		t.Fatal("focus not cleared")
	}
}

func TestTailJob(t *testing.T) {
	s := New()
	s.SetTailJob("o", "r", 5)
	if o, r, id := s.TailJob(); o != "o" || r != "r" || id != 5 {
		t.Fatalf("got %s %s %d", o, r, id)
	}
	s.SetTailJob("", "", 0)
	if _, _, id := s.TailJob(); id != 0 {
		t.Fatal("tail job not cleared")
	}
}
