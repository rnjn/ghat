package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/poller"
)

func TestStatusBarShowsAgeAndError(t *testing.T) {
	var sb statusBar
	sb.observe(poller.RateLimit{Remaining: 4321, Reset: testNow.Add(time.Hour)}, ago(0))
	sb.observe(poller.RunsUpdated{RepoKey: "acme/api"}, ago(3*time.Second))
	sb.observe(poller.PollerError{Resource: "runs:acme/web", Err: errors.New("GitHub API: 502")}, ago(time.Second))
	got := plain(sb.view(120, testNow))
	if strings.Contains(got, "Board") || strings.Contains(got, "4321") {
		t.Errorf("title and quota belong in the header now: %q", got)
	}
	if q := sb.quota(testNow); q != "quota 4321" {
		t.Errorf("quota = %q", q)
	}
	for _, want := range []string{"3s ago", "runs:acme/web", "502"} {
		if !strings.Contains(got, want) {
			t.Errorf("status bar %q lacks %q", got, want)
		}
	}
}

func TestStatusBarRateLimited(t *testing.T) {
	var sb statusBar
	reset := testNow.Add(34 * time.Minute)
	sb.observe(poller.RateLimit{Remaining: 0, Reset: reset}, testNow)
	got := sb.quota(testNow)
	if !strings.Contains(got, "rate limited until "+reset.Local().Format("15:04")) {
		t.Fatalf("status bar = %q", got)
	}
}

func TestStatusBarTruncates(t *testing.T) {
	var sb statusBar
	sb.observe(poller.PollerError{Resource: "runs:acme/web", Err: errors.New(strings.Repeat("x", 300))}, testNow)
	assertFits(t, sb.view(40, testNow), 40)
	assertFits(t, sb.view(0, testNow), 0)
}

func TestStatusBarPolledAdvancesAge(t *testing.T) {
	var sb statusBar
	sb.observe(poller.Polled{}, ago(2*time.Second))
	if got := plain(sb.view(120, testNow)); !strings.Contains(got, "polled 2s ago") {
		t.Fatalf("status bar = %q", got)
	}
}

func TestStatusBarKeepsPermissionGuidanceVisible(t *testing.T) {
	var sb statusBar
	sb.observe(poller.RateLimit{Remaining: 4321}, testNow)
	sb.observe(poller.RunsUpdated{}, testNow)
	err := &gh.PermissionError{Status: 403, Message: "Resource not accessible by personal access token"}
	sb.observe(poller.PollerError{Resource: "action", Err: err}, testNow)
	got := plain(sb.view(100, testNow))
	if !strings.Contains(got, "needs write access") {
		t.Fatalf("guidance cut off: %q", got)
	}
}

func TestStatusBarOffline(t *testing.T) {
	var sb statusBar
	sb.observe(poller.Polled{}, ago(40*time.Second))
	sb.observe(poller.RunsFailed{RepoKey: "a/x", Err: errAny}, ago(time.Second))
	if got := plain(sb.view(120, testNow)); !strings.Contains(got, "offline, retrying (data 40s old)") {
		t.Fatalf("status bar = %q", got)
	}
	sb.observe(poller.Polled{}, testNow)
	if got := plain(sb.view(120, testNow)); strings.Contains(got, "offline") {
		t.Fatalf("still offline after success: %q", got)
	}
}

func TestStatusBarLowQuota(t *testing.T) {
	var sb statusBar
	sb.observe(poller.RateLimit{Remaining: 420, Reset: testNow.Add(time.Hour)}, testNow)
	if got := sb.quota(testNow); !strings.Contains(got, "quota 420 low, polling slowed") {
		t.Fatalf("status bar = %q", got)
	}
}
