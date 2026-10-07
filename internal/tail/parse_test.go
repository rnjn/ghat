package tail

import (
	"testing"
	"time"

	"ghtui/internal/gh"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

var testSteps = []gh.Step{
	{Number: 1, Name: "Set up job", StartedAt: at("2026-10-07T04:28:23Z")},
	{Number: 2, Name: "Checkout", StartedAt: at("2026-10-07T04:28:25Z")},
	{Number: 3, Name: "Run make test", StartedAt: at("2026-10-07T04:28:27Z")},
	{Number: 4, Name: "Complete job", StartedAt: at("2026-10-07T04:28:40Z")},
}

func TestParseLinesStripsTimestampAndBOM(t *testing.T) {
	raw := "\uFEFF2026-10-07T04:28:23.9358429Z Current runner version: '2.329.0'\n"
	lines := ParseLines([]byte(raw), nil)
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	l := lines[0]
	if l.Text != "Current runner version: '2.329.0'" || l.Kind != Plain {
		t.Fatalf("line = %+v", l)
	}
	want := time.Date(2026, 10, 7, 4, 28, 23, 935842900, time.UTC)
	if !l.Timestamp.Equal(want) {
		t.Fatalf("Timestamp = %v, want %v", l.Timestamp, want)
	}
}

func TestParseLinesClassifiesMarkers(t *testing.T) {
	raw := "2026-10-07T04:28:24.0Z ##[group]Inputs\n" +
		"2026-10-07T04:28:24.1Z ##[endgroup]\n" +
		"2026-10-07T04:28:24.2Z ##[error]boom\n" +
		"2026-10-07T04:28:24.3Z ##[warning]careful\n" +
		"2026-10-07T04:28:24.4Z ##[command]/usr/bin/git version\n" +
		"2026-10-07T04:28:24.5Z plain text\n"
	want := []struct {
		kind Kind
		text string
	}{
		{Group, "Inputs"}, {EndGroup, ""}, {Error, "boom"}, {Warning, "careful"}, {Command, "/usr/bin/git version"}, {Plain, "plain text"},
	}
	lines := ParseLines([]byte(raw), nil)
	if len(lines) != len(want) {
		t.Fatalf("len = %d, want %d", len(lines), len(want))
	}
	for i, w := range want {
		if lines[i].Kind != w.kind || lines[i].Text != w.text {
			t.Errorf("line %d = %+v, want kind %v text %q", i, lines[i], w.kind, w.text)
		}
	}
}

func TestParseLinesAttributesSteps(t *testing.T) {
	raw := "2026-10-07T04:28:23.5Z Current runner version\n" + // step 1: before any header
		"2026-10-07T04:28:24.6Z ##[group] Inputs\n" + // nested group in step 1
		"2026-10-07T04:28:25.1Z ##[group]Run actions/checkout@v4\n" + // step 2: "Run" header, step 2 has started
		"2026-10-07T04:28:25.2Z Syncing repository\n" +
		"2026-10-07T04:28:27.1Z ##[group]Run make test\n" + // step 3: exact name match
		"2026-10-07T04:28:27.2Z ##[group]Run inner composite step\n" + // step 4 has not started: stays in 3
		"2026-10-07T04:28:30.0Z ok\n" +
		"2026-10-07T04:28:40.1Z Cleaning up orphan processes\n" // no header: stays in 3 (no evidence)
	want := []int{1, 1, 2, 2, 3, 3, 3, 3}
	lines := ParseLines([]byte(raw), testSteps)
	if len(lines) != len(want) {
		t.Fatalf("len = %d", len(lines))
	}
	for i, w := range want {
		if lines[i].StepNumber != w {
			t.Errorf("line %d %q: step %d, want %d", i, lines[i].Text, lines[i].StepNumber, w)
		}
	}
}

func TestParseLinesWithoutTimestamp(t *testing.T) {
	raw := "no timestamp here\n##[error]bare annotation\n\n2026-bad-time text\n"
	lines := ParseLines([]byte(raw), testSteps)
	if len(lines) != 4 {
		t.Fatalf("len = %d", len(lines))
	}
	if !lines[0].Timestamp.IsZero() || lines[0].Text != "no timestamp here" {
		t.Errorf("line 0 = %+v", lines[0])
	}
	if lines[1].Kind != Error || lines[1].Text != "bare annotation" {
		t.Errorf("line 1 = %+v", lines[1])
	}
	if lines[2].Text != "" || lines[3].Text != "2026-bad-time text" {
		t.Errorf("lines 2,3 = %+v %+v", lines[2], lines[3])
	}
}

func TestParseLinesEmptyAndTrailingNewline(t *testing.T) {
	if got := ParseLines(nil, nil); len(got) != 0 {
		t.Fatalf("nil input gave %d lines", len(got))
	}
	if got := ParseLines([]byte("a\r\nb\n"), nil); len(got) != 2 || got[0].Text != "a" || got[1].Text != "b" {
		t.Fatalf("got %+v", got)
	}
	if got := ParseLines([]byte("a\nb"), nil); len(got) != 2 {
		t.Fatalf("no trailing newline: got %d lines", len(got))
	}
}
