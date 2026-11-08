package usage

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	admissionWindow            = time.Minute
	principalRequestsPerWindow = 12
	globalRequestsPerWindow    = 60
	maxPrincipalRequests       = 8
	maxGlobalRequests          = 16
	maxPrincipalFetches        = 2
	maxGlobalFetches           = 8
	maxAdmissionPrincipals     = 4096
	maxFailureBackoff          = 30 * time.Second
)

// RateLimitError is safe to expose as HTTP 429 with a Retry-After delay.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string { return "usage request admission limited" }

type principalAdmission struct {
	requests       []time.Time
	activeRequests int
	activeFetches  int
	failures       int
	retryAt        time.Time
}

type admission struct {
	mu             sync.Mutex
	now            func() time.Time
	principals     map[string]*principalAdmission
	requests       []time.Time
	activeRequests int
	activeFetches  int
}

func newAdmission(now func() time.Time) *admission {
	return &admission{now: now, principals: make(map[string]*principalAdmission)}
}

func principalKey(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Every request reserves the worst-case upstream cost, even on positive cache
// hits: identity resolution precedes the usage cache. Rejections consume no calls.
func (a *admission) beginRequest(email string) (func(error), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	a.requests = recentRequests(a.requests, now)
	key := principalKey(email)
	state := a.principals[key]
	if state == nil {
		// Only forget principals whose entire rate history and cooldown expired.
		// Capacity pressure must never reset spent quota or active operation counts.
		for key, candidate := range a.principals {
			candidate.requests = recentRequests(candidate.requests, now)
			if len(candidate.requests) == 0 && candidate.activeRequests == 0 && candidate.activeFetches == 0 && !now.Before(candidate.retryAt) {
				delete(a.principals, key)
			}
		}
		if len(a.principals) >= maxAdmissionPrincipals {
			return nil, &RateLimitError{RetryAfter: admissionWindow}
		}
		state = &principalAdmission{}
		a.principals[key] = state
	}
	state.requests = recentRequests(state.requests, now)
	retryAfter := time.Duration(0)
	if now.Before(state.retryAt) {
		retryAfter = state.retryAt.Sub(now)
	}
	if len(state.requests) >= principalRequestsPerWindow {
		retryAfter = max(retryAfter, state.requests[0].Add(admissionWindow).Sub(now))
	}
	if len(a.requests) >= globalRequestsPerWindow {
		retryAfter = max(retryAfter, a.requests[0].Add(admissionWindow).Sub(now))
	}
	if state.activeRequests >= maxPrincipalRequests || a.activeRequests >= maxGlobalRequests {
		retryAfter = max(retryAfter, time.Second)
	}
	if retryAfter > 0 {
		return nil, &RateLimitError{RetryAfter: retryAfter}
	}
	state.requests = append(state.requests, now)
	a.requests = append(a.requests, now)
	state.activeRequests++
	a.activeRequests++
	return func(err error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		state.activeRequests--
		a.activeRequests--
		var limited *RateLimitError
		if errors.As(err, &limited) {
			return
		}
		if err == nil {
			state.failures = 0
			state.retryAt = time.Time{}
			return
		}
		state.failures = min(state.failures+1, 6)
		backoff := min(time.Second<<(state.failures-1), maxFailureBackoff)
		state.retryAt = a.now().Add(backoff)
	}, nil
}

// Fetch leases live inside the actual shared operation, not its requesting
// waiter, so canceled callers cannot release capacity while work continues.
func (a *admission) beginFetch(email string) (func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.principals[principalKey(email)]
	if state == nil || state.activeFetches >= maxPrincipalFetches || a.activeFetches >= maxGlobalFetches {
		return nil, &RateLimitError{RetryAfter: time.Second}
	}
	state.activeFetches++
	a.activeFetches++
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		state.activeFetches--
		a.activeFetches--
	}, nil
}

func recentRequests(requests []time.Time, now time.Time) []time.Time {
	first := 0
	for first < len(requests) && !now.Before(requests[first].Add(admissionWindow)) {
		first++
	}
	return requests[first:]
}
