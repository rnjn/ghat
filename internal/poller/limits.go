package poller

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ghtui/internal/gh"
)

const (
	lowQuota   = 500
	maxBackoff = 2 * time.Minute
)

// paused reports whether no request may be made at now: after a 401, or
// while the rate limit is exhausted.
func (p *Poller) paused(now time.Time) bool {
	if p.authFailed || now.Before(p.pauseUntil) {
		return true
	}
	rem, reset := p.api.RateLimit()
	return rem == 0 && now.Before(reset)
}

// scale doubles an interval while the remaining quota is low.
func (p *Poller) scale(d time.Duration) time.Duration {
	if rem, _ := p.api.RateLimit(); rem >= 0 && rem < lowQuota {
		return 2 * d
	}
	return d
}

// reportRate sends a RateLimit message when the quota changed.
func (p *Poller) reportRate() {
	rem, reset := p.api.RateLimit()
	if rem < 0 {
		return
	}
	msg := RateLimit{Remaining: rem, Reset: reset}
	if msg != p.lastRate {
		p.lastRate = msg
		p.send(msg)
	}
}

// succeeded clears a resource's backoff.
func (p *Poller) succeeded(key string) { delete(p.failures, key) }

// failed handles a poll error: a 401 stops all polling, a rate-limit error
// pauses until reset, 5xx and network errors back off exponentially per
// resource (capped at 2 min), anything else retries at the normal interval.
func (p *Poller) failed(key string, now time.Time, interval time.Duration, err error) {
	var ae *gh.APIError
	if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
		if !p.authFailed {
			p.authFailed = true
			p.send(AuthFailed{Err: err})
		}
		return
	}
	var rl *gh.RateLimitError
	if errors.As(err, &rl) {
		p.pauseUntil = rl.Reset
		if p.pauseUntil.IsZero() {
			p.pauseUntil = now.Add(time.Minute) // no reset header: wait a minute
		}
		p.sched.set(key, p.pauseUntil)
		p.send(PollerError{Resource: key, Err: err})
		return
	}
	delay := p.scale(interval)
	if gh.IsTransient(err) {
		p.failures[key]++
		delay = interval << p.failures[key]
		if delay > maxBackoff || delay <= 0 {
			delay = maxBackoff
		}
	}
	p.sched.set(key, now.Add(delay))
	if repoKey, ok := strings.CutPrefix(key, "runs:"); ok && gh.IsTransient(err) {
		p.send(RunsFailed{RepoKey: repoKey, Err: err})
		return
	}
	p.send(PollerError{Resource: key, Err: err})
}
