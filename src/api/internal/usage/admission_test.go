package usage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func requireLimited(t *testing.T, err error) *RateLimitError {
	t.Helper()
	var limited *RateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("error = %v, want rate limit with retry delay", err)
	}
	return limited
}

func TestAdmissionAlternatingPeriodsAndCachedRequestsCannotBypassIdentityQuota(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	resolver := &fakeResolver{login: "self"}
	billing := &fakeBilling{}
	service := NewService(ServiceConfig{Resolver: resolver, Billing: billing, CacheTTL: time.Minute, Now: func() time.Time { return now }})
	for i := 0; i < principalRequestsPerWindow; i++ {
		email := "self@example.com"
		if i%2 == 1 {
			email = " SELF@EXAMPLE.COM "
		}
		month := 5 + i%2
		result, err := service.GetMonthlyUsage(context.Background(), email, 2026, month)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if i >= 2 && !result.SourceMetadata.Cached {
			t.Fatal("ordinary repeat lost positive cache")
		}
		if result.User.Email != email || result.User.GitHubLogin != "self" {
			t.Fatalf("incorrect self binding: %#v", result.User)
		}
	}
	_, err := service.GetMonthlyUsage(context.Background(), "self@example.com", 2026, 4)
	requireLimited(t, err)
	if len(resolver.emails) != principalRequestsPerWindow {
		t.Fatalf("identity calls = %d", len(resolver.emails))
	}
	wantCalls := expectedBillingRequestCount(now, 2026, 5) + expectedBillingRequestCount(now, 2026, 6)
	if billing.requestCount() != wantCalls {
		t.Fatalf("billing calls = %d, want %d", billing.requestCount(), wantCalls)
	}
	now = now.Add(time.Minute)
	if _, err := service.GetMonthlyUsage(context.Background(), "self@example.com", 2026, 4); err != nil {
		t.Fatalf("quota did not refill: %v", err)
	}
}

func TestAdmissionFailuresBackOffAcrossPeriodsBeforeIdentity(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	upstreamErr := errors.New("identity unavailable")
	resolver := &fakeResolver{err: upstreamErr}
	service := NewService(ServiceConfig{Resolver: resolver, Billing: &fakeBilling{}, Now: func() time.Time { return now }})
	for attempt := 0; attempt < 8; attempt++ {
		_, err := service.GetMonthlyUsage(context.Background(), "self@example.com", 2026, 6-attempt%3)
		if !errors.Is(err, upstreamErr) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		_, err = service.GetMonthlyUsage(context.Background(), "SELF@example.com", 2026, 3)
		limited := requireLimited(t, err)
		want := min(time.Second<<min(attempt, 5), maxFailureBackoff)
		if limited.RetryAfter != want {
			t.Fatalf("retry = %v, want %v", limited.RetryAfter, want)
		}
		now = now.Add(limited.RetryAfter)
	}
	if len(resolver.emails) != 8 {
		t.Fatalf("identity calls = %d, want 8", len(resolver.emails))
	}
}

func TestAdmissionGlobalRateBoundsDifferentPrincipals(t *testing.T) {
	now := time.Now()
	gate := newAdmission(func() time.Time { return now })
	for i := 0; i < globalRequestsPerWindow; i++ {
		release, err := gate.beginRequest(fmt.Sprintf("user%d@example.com", i))
		if err != nil {
			t.Fatal(err)
		}
		release(nil)
	}
	_, err := gate.beginRequest("another@example.com")
	requireLimited(t, err)
	now = now.Add(time.Minute)
	release, err := gate.beginRequest("another@example.com")
	if err != nil {
		t.Fatal(err)
	}
	release(nil)
}

func TestAdmissionFetchConcurrencyAcrossUsers(t *testing.T) {
	releaseBilling := make(chan struct{})
	billing := &fakeBilling{wait: releaseBilling}
	resolver := &fakeResolver{loginsByEmail: map[string]string{}}
	for i := 0; i <= maxGlobalFetches; i++ {
		resolver.loginsByEmail[fmt.Sprintf("user%d@example.com", i)] = fmt.Sprintf("login%d", i)
	}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	service := NewService(ServiceConfig{Resolver: resolver, Billing: billing, Now: func() time.Time { return now }})
	var wg sync.WaitGroup
	errs := make(chan error, maxGlobalFetches)
	for i := 0; i < maxGlobalFetches; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := service.GetMonthlyUsage(context.Background(), fmt.Sprintf("user%d@example.com", i), 2026, 6)
			errs <- err
		}(i)
	}
	deadline := time.After(time.Second)
	for billing.requestCount() != maxGlobalFetches {
		select {
		case <-deadline:
			close(releaseBilling)
			wg.Wait()
			t.Fatal("fetch slots not occupied")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	_, err := service.GetMonthlyUsage(context.Background(), fmt.Sprintf("user%d@example.com", maxGlobalFetches), 2026, 6)
	requireLimited(t, err)
	if billing.requestCount() != maxGlobalFetches {
		t.Fatal("rejected fetch called billing")
	}
	close(releaseBilling)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Released operations allow another user without waiting for a cooldown.
	if _, err := service.GetMonthlyUsage(context.Background(), fmt.Sprintf("user%d@example.com", maxGlobalFetches), 2026, 6); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionFetchLeaseSurvivesRequestRelease(t *testing.T) {
	gate := newAdmission(time.Now)
	request, err := gate.beginRequest("self@example.com")
	if err != nil {
		t.Fatal(err)
	}
	first, err := gate.beginFetch("self@example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := gate.beginFetch("self@example.com")
	if err != nil {
		t.Fatal(err)
	}
	request(context.Canceled)
	_, err = gate.beginFetch("SELF@example.com")
	requireLimited(t, err)
	first()
	second()
	if gate.activeFetches != 0 {
		t.Fatal("fetch leases leaked")
	}
}

func TestAdmissionRequestConcurrencyBeforeIdentity(t *testing.T) {
	gate := newAdmission(time.Now)
	releases := []func(error){}
	for i := 0; i < maxGlobalRequests; i++ {
		release, err := gate.beginRequest(fmt.Sprintf("user%d@example.com", i))
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	_, err := gate.beginRequest("extra@example.com")
	requireLimited(t, err)
	for _, release := range releases {
		release(nil)
	}
	for i := 0; i < maxPrincipalRequests; i++ {
		release, err := gate.beginRequest("self@example.com")
		if err != nil {
			t.Fatal(err)
		}
		defer release(nil)
	}
	_, err = gate.beginRequest("SELF@example.com")
	requireLimited(t, err)
}

func TestAdmissionCapacityDoesNotForgetSpentQuota(t *testing.T) {
	now := time.Now()
	gate := newAdmission(func() time.Time { return now })
	for i := 0; i < maxAdmissionPrincipals; i++ {
		gate.principals[fmt.Sprintf("user%d@example.com", i)] = &principalAdmission{requests: []time.Time{now}}
	}
	_, err := gate.beginRequest("new@example.com")
	requireLimited(t, err)
	if len(gate.principals) != maxAdmissionPrincipals {
		t.Fatal("live rate history evicted")
	}
	now = now.Add(time.Minute)
	release, err := gate.beginRequest("new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	release(nil)
	if len(gate.principals) != 1 {
		t.Fatal("expired history not cleaned")
	}
}

func TestReportingWindowAlwaysCapped(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := ValidateReportingPeriod(2025, 6, now, 1000000); !errors.Is(err, ErrPeriodOutOfRange) {
		t.Fatalf("uncapped period accepted: %v", err)
	}
	if err := ValidateReportingPeriod(2025, 7, now, 1000000); err != nil {
		t.Fatalf("12 month boundary rejected: %v", err)
	}
}

type deadlineRecordingResolver struct{ deadline time.Time }

func (r *deadlineRecordingResolver) ResolveGitHubLogin(ctx context.Context, _ string) (string, error) {
	r.deadline, _ = ctx.Deadline()
	return "self", nil
}

func TestServiceRequestDeadlineAllowsIdentityThenSharedFetch(t *testing.T) {
	resolver := &deadlineRecordingResolver{}
	service := NewService(ServiceConfig{
		Resolver: resolver,
		Billing:  &fakeBilling{},
		Now:      func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
	})
	before := time.Now()
	if _, err := service.GetMonthlyUsage(context.Background(), "self@example.com", 2026, 6); err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	// The request owns enough time for the identity budget followed by the full
	// shared-operation budget; an outer 45s deadline would discard that result.
	minimum := before.Add(10*time.Second + 45*time.Second)
	maximum := after.Add(55 * time.Second)
	if resolver.deadline.Before(minimum) || resolver.deadline.After(maximum) {
		t.Fatalf("identity deadline = %v, want 55s request deadline between %v and %v", resolver.deadline, minimum, maximum)
	}
}
