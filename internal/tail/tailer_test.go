package tail

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ghtui/internal/gh"
)

type logResp struct {
	body string
	err  error
}

// fakeSource replays scripted responses; the last one repeats.
type fakeSource struct {
	jobs     []gh.Job
	logs     []logResp
	jobCalls int
	logCalls int
}

func (f *fakeSource) GetJob(_ context.Context, owner, repo string, id int64) (gh.Job, error) {
	j := f.jobs[min(f.jobCalls, len(f.jobs)-1)]
	f.jobCalls++
	return j, nil
}

func (f *fakeSource) JobLog(_ context.Context, owner, repo string, id int64) ([]byte, error) {
	l := f.logs[min(f.logCalls, len(f.logs)-1)]
	f.logCalls++
	return []byte(l.body), l.err
}

func running() gh.Job { return gh.Job{ID: 7, Status: "in_progress"} }
func done(conclusion string) gh.Job {
	return gh.Job{ID: 7, Status: "completed", Conclusion: conclusion}
}

func logOf(texts ...string) logResp {
	var b strings.Builder
	for _, t := range texts {
		b.WriteString("2026-10-07T04:28:23.0Z " + t + "\n")
	}
	return logResp{body: b.String()}
}

type harness struct {
	sleeps int
	got    []string
}

func (h *harness) opts() Options {
	return Options{Interval: 5 * time.Second, Sleep: func(ctx context.Context, d time.Duration) error {
		h.sleeps++
		return ctx.Err()
	}}
}

func (h *harness) emit(l LogLine) { h.got = append(h.got, l.Text) }

func TestTailerEmitsOnlyNewLines(t *testing.T) {
	src := &fakeSource{
		jobs: []gh.Job{running(), running(), running(), done("success")},
		logs: []logResp{logOf("a"), logOf("a"), logOf("a", "b", "c"), logOf("a", "b", "c", "d")},
	}
	h := &harness{}
	job, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit)
	if err != nil {
		t.Fatal(err)
	}
	if job.Conclusion != "success" {
		t.Fatalf("job = %+v", job)
	}
	if strings.Join(h.got, ",") != "a,b,c,d" {
		t.Fatalf("emitted %v", h.got)
	}
	if h.sleeps != 3 {
		t.Fatalf("slept %d times, want 3", h.sleeps)
	}
}

func TestTailerSkipsLogNotReady(t *testing.T) {
	src := &fakeSource{
		jobs: []gh.Job{running(), running(), done("failure")},
		logs: []logResp{{err: gh.ErrLogNotReady}, {err: gh.ErrLogNotReady}, logOf("x")},
	}
	h := &harness{}
	job, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit)
	if err != nil {
		t.Fatalf("ErrLogNotReady surfaced: %v", err)
	}
	if job.Conclusion != "failure" || strings.Join(h.got, ",") != "x" {
		t.Fatalf("job=%+v emitted=%v", job, h.got)
	}
}

func TestTailerReturnsOtherErrors(t *testing.T) {
	boom := errors.New("boom")
	src := &fakeSource{jobs: []gh.Job{running()}, logs: []logResp{{err: boom}}}
	h := &harness{}
	_, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestTailerContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	src := &fakeSource{jobs: []gh.Job{running()}, logs: []logResp{logOf("a")}}
	h := &harness{}
	opts := Options{Interval: time.Second, Sleep: func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}}
	_, err := NewTailer(src, "o", "r", 7, opts).Run(ctx, h.emit)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestTailerDefaultSleepHonoursContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	src := &fakeSource{jobs: []gh.Job{running()}, logs: []logResp{logOf("a")}}
	_, err := NewTailer(src, "o", "r", 7, Options{Interval: time.Hour}).Run(ctx, func(LogLine) {})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}
