package tui

import (
	"strings"
	"testing"
	"time"
)

func TestAxisBarOffsetsWithoutCreatedTime(t *testing.T) {
	ax := axis{t0: testNow, total: 10 * time.Second, w: 10}
	if got := plain(ax.bar(time.Time{}, testNow.Add(4*time.Second), testNow.Add(6*time.Second), styleDim)); got != "    ██" {
		t.Fatalf("no created: %q", got)
	}
	if got := plain(ax.bar(testNow.Add(2*time.Second), testNow.Add(4*time.Second), testNow.Add(6*time.Second), styleDim)); got != "  ░░██" {
		t.Fatalf("with created: %q", got)
	}
	if got := plain(ax.bar(time.Time{}, testNow.Add(9*time.Second), testNow.Add(9*time.Second), styleDim)); got != "         █" {
		t.Fatalf("instant job: %q", got)
	}
	if got := ax.bar(time.Time{}, time.Time{}, time.Time{}, styleDim); got != "" {
		t.Fatalf("unstarted: %q", got)
	}
}

func TestTickLabels(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "0", 30 * time.Second: "30s", 5 * time.Minute: "5m", time.Hour: "1h", 90 * time.Minute: "1h30m"} {
		if got := tickLabel(d); got != want {
			t.Errorf("%v: %q want %q", d, got, want)
		}
	}
	ax := axis{t0: testNow, total: 100 * time.Second, w: 50}
	ticks, _ := ax.labels()
	if got := plain(ticks); got[:1] != "0" || len(got) != 50 || !contains(got, "30s") || contains(got, "10s") {
		t.Fatalf("ticks %q", got)
	}
}

func TestTickLabelsDropOnesThatDoNotFit(t *testing.T) {
	ax := axis{t0: testNow, total: 61 * time.Second, w: 14}
	ticks, _ := ax.labels()
	if got := plain(ticks); strings.Contains(got, "1") {
		t.Fatalf("ticks %q should drop the cut label", got)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
