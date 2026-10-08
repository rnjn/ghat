package tail

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rnjn/ghat/internal/gh"
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
	// While running the log is fetched on the 1st poll only (then every 6th);
	// the completed poll fetches again.
	src := &fakeSource{
		jobs: []gh.Job{running(), running(), running(), done("success")},
		logs: []logResp{logOf("a"), logOf("a", "b", "c", "d")},
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

var finalSteps = []gh.Step{
	{Number: 1, Name: "Run make test", StartedAt: at("2026-10-07T04:28:23Z")},
	{Number: 2, Name: "Complete job", StartedAt: at("2026-10-07T04:28:30Z")},
}

func doneWithSteps() gh.Job {
	j := done("success")
	j.Steps = finalSteps
	return j
}

const (
	earlyLine = "2026-10-07T04:28:23.5Z early\n"
	lateLine  = "2026-10-07T04:28:30.2Z Cleaning up orphan processes\n"
)

func TestTailerRetriesFinalFetchUntilLogComplete(t *testing.T) {
	src := &fakeSource{
		jobs: []gh.Job{doneWithSteps()},
		logs: []logResp{{body: earlyLine}, {body: earlyLine}, {body: earlyLine + lateLine}},
	}
	h := &harness{}
	if _, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit); err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.got, ",") != "early,Cleaning up orphan processes" {
		t.Fatalf("emitted %v", h.got)
	}
	if src.logCalls != 3 || h.sleeps != 2 {
		t.Fatalf("log calls %d, sleeps %d; want 3, 2", src.logCalls, h.sleeps)
	}
}

func TestTailerFinalFetchGivesUpAfterThreeRetries(t *testing.T) {
	src := &fakeSource{jobs: []gh.Job{doneWithSteps()}, logs: []logResp{{err: gh.ErrLogNotReady}}}
	h := &harness{}
	job, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit)
	if !errors.Is(err, ErrLogIncomplete) || job.Status != "completed" {
		t.Fatalf("job=%+v err=%v, want completed job and ErrLogIncomplete", job, err)
	}
	if src.logCalls != 4 || h.sleeps != 3 {
		t.Fatalf("log calls %d, sleeps %d; want 4, 3", src.logCalls, h.sleeps)
	}
}

func TestTailerNoRetryWhenLogAlreadyComplete(t *testing.T) {
	src := &fakeSource{jobs: []gh.Job{doneWithSteps()}, logs: []logResp{{body: earlyLine + lateLine}}}
	h := &harness{}
	if _, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit); err != nil {
		t.Fatal(err)
	}
	if src.logCalls != 1 || h.sleeps != 0 {
		t.Fatalf("log calls %d, sleeps %d; want 1, 0", src.logCalls, h.sleeps)
	}
}

func TestTailerRetriesTransientErrors(t *testing.T) {
	src := &fakeSource{
		jobs: []gh.Job{running(), done("success")},
		logs: []logResp{
			{err: &gh.APIError{Status: 502}},
			{err: errors.New("dial tcp: connection reset")},
			logOf("a"),
		},
	}
	h := &harness{}
	job, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit)
	if err != nil || job.Conclusion != "success" || strings.Join(h.got, ",") != "a" {
		t.Fatalf("job=%+v err=%v emitted=%v", job, err, h.got)
	}
}

func TestTailerGivesUpAfterRepeatedTransientErrors(t *testing.T) {
	src := &fakeSource{jobs: []gh.Job{running()}, logs: []logResp{{err: &gh.APIError{Status: 503}}}}
	h := &harness{}
	_, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit)
	var ae *gh.APIError
	if !errors.As(err, &ae) || ae.Status != 503 {
		t.Fatalf("err = %v", err)
	}
	if src.logCalls != maxTransient+1 {
		t.Fatalf("log calls = %d, want %d", src.logCalls, maxTransient+1)
	}
}

func TestTailerFailsFastOnClientErrors(t *testing.T) {
	src := &fakeSource{jobs: []gh.Job{running()}, logs: []logResp{{err: &gh.APIError{Status: 403}}}}
	h := &harness{}
	if _, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit); err == nil || src.logCalls != 1 {
		t.Fatalf("err = %v, log calls %d", err, src.logCalls)
	}
}

func TestTailerFetchesLogEverySixthPollWhileRunning(t *testing.T) {
	jobs := make([]gh.Job, 13)
	for i := range jobs {
		jobs[i] = running()
	}
	src := &fakeSource{jobs: append(jobs, done("success")), logs: []logResp{logOf("a")}}
	h := &harness{}
	if _, err := NewTailer(src, "o", "r", 7, h.opts()).Run(context.Background(), h.emit); err != nil {
		t.Fatal(err)
	}
	// polls 0, 6 and 12 while running, plus the completed poll
	if src.logCalls != 4 {
		t.Fatalf("log calls = %d, want 4", src.logCalls)
	}
}
