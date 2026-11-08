package usage

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	gh "copilot-per-user/api/internal/github"
)

type billingFunc func(context.Context, gh.AICreditUsageRequest) (gh.AICreditUsageReport, error)

func (f billingFunc) GetAICreditUsage(ctx context.Context, req gh.AICreditUsageRequest) (gh.AICreditUsageReport, error) {
	return f(ctx, req)
}

func TestServiceOwnerCancellationDoesNotPoisonFollowers(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	service := NewService(ServiceConfig{
		Resolver: &fakeResolver{login: "alice"}, CacheTTL: time.Minute,
		Now: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
		Billing: billingFunc(func(ctx context.Context, _ gh.AICreditUsageRequest) (gh.AICreditUsageReport, error) {
			if calls.Add(1) == 1 {
				close(started)
				select {
				case <-ctx.Done():
					return gh.AICreditUsageReport{}, ctx.Err()
				case <-release:
				}
			}
			return gh.AICreditUsageReport{}, nil
		}),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner := requestUsage(service, ctx, "owner@example.test")
	<-started
	follower := requestUsage(service, context.Background(), "alias@example.test")
	waitForWaiterCount(t, service, 2)
	cancel()
	if result := receiveUsage(t, owner); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("owner error = %v", result.err)
	}
	close(release)
	result := receiveUsage(t, follower)
	if result.err != nil || result.usage.User.Email != "alias@example.test" {
		t.Fatalf("follower = %#v, %v", result.usage, result.err)
	}
	cached, err := service.GetMonthlyUsage(context.Background(), "third@example.test", 2026, 6)
	if err != nil || !cached.SourceMetadata.Cached || cached.User.Email != "third@example.test" {
		t.Fatalf("cached result = %#v, %v", cached, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("billing calls = %d, want one monthly plus one daily", calls.Load())
	}
}

func TestServiceSharedDeadlineCleansUpAndAllowsRetry(t *testing.T) {
	var calls atomic.Int32
	service := NewService(ServiceConfig{
		Resolver: &fakeResolver{login: "alice"}, CacheTTL: time.Minute,
		Now: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
		Billing: billingFunc(func(ctx context.Context, _ gh.AICreditUsageRequest) (gh.AICreditUsageReport, error) {
			if calls.Add(1) == 1 {
				<-ctx.Done()
				return gh.AICreditUsageReport{}, ctx.Err()
			}
			return gh.AICreditUsageReport{}, nil
		}),
	})
	service.fetchTimeout = 30 * time.Millisecond
	_, err := service.GetMonthlyUsage(context.Background(), "alice@example.test", 2026, 6)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shared deadline error = %v", err)
	}
	service.mu.Lock()
	inflight, cached := len(service.inflight), len(service.cache)
	service.mu.Unlock()
	if inflight != 0 || cached != 0 {
		t.Fatalf("after timeout: inflight=%d cached=%d", inflight, cached)
	}
	result, err := service.GetMonthlyUsage(context.Background(), "alice@example.test", 2026, 6)
	if err != nil || result.SourceMetadata.Cached || calls.Load() != 3 {
		t.Fatalf("retry = %#v, %v; calls = %d", result, err, calls.Load())
	}
}

func TestServiceAbandonedSharedWorkExpires(t *testing.T) {
	started := make(chan struct{})
	service := NewService(ServiceConfig{
		Resolver: &fakeResolver{login: "alice"}, CacheTTL: time.Minute,
		Now: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
		Billing: billingFunc(func(ctx context.Context, _ gh.AICreditUsageRequest) (gh.AICreditUsageReport, error) {
			close(started)
			<-ctx.Done()
			return gh.AICreditUsageReport{}, ctx.Err()
		}),
	})
	service.fetchTimeout = 30 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner := requestUsage(service, ctx, "alice@example.test")
	<-started
	service.mu.Lock()
	var done <-chan struct{}
	for _, call := range service.inflight {
		done = call.done
	}
	service.mu.Unlock()
	cancel()
	if result := receiveUsage(t, owner); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("owner error = %v", result.err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("abandoned shared operation did not expire")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.inflight) != 0 || len(service.cache) != 0 {
		t.Fatal("abandoned operation retained inflight or cache state")
	}
}

func TestServiceBoundsWaitersAndReleasesCanceledAdmissions(t *testing.T) {
	release := make(chan struct{})
	var released bool
	defer func() {
		if !released {
			close(release)
		}
	}()
	billing := &fakeBilling{wait: release}
	service := NewService(ServiceConfig{
		Resolver: &fakeResolver{login: "alice"}, Billing: billing,
		Now: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
	})
	owner := requestUsage(service, context.Background(), "alice@example.test")
	waitForBillingRequest(t, billing)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	followers := make([]<-chan usageResult, 0, maxInflightWaiters-1)
	for i := 1; i < maxInflightWaiters; i++ {
		followers = append(followers, requestUsage(service, ctx, "alias@example.test"))
	}
	waitForWaiterCount(t, service, maxInflightWaiters)
	_, err := service.GetMonthlyUsage(context.Background(), "overflow@example.test", 2026, 6)
	if !errors.Is(err, ErrTooManyWaiters) {
		t.Fatalf("overflow error = %v", err)
	}
	cancel()
	for _, follower := range followers {
		if result := receiveUsage(t, follower); !errors.Is(result.err, context.Canceled) {
			t.Fatalf("canceled follower error = %v", result.err)
		}
	}
	waitForWaiterCount(t, service, 1)
	replacementCtx, replacementCancel := context.WithCancel(context.Background())
	replacement := requestUsage(service, replacementCtx, "replacement@example.test")
	waitForWaiterCount(t, service, 2)
	replacementCancel()
	if result := receiveUsage(t, replacement); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("replacement error = %v", result.err)
	}
	close(release)
	released = true
	if result := receiveUsage(t, owner); result.err != nil {
		t.Fatalf("owner error = %v", result.err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.inflight) != 0 {
		t.Fatal("completed operation retained inflight state")
	}
}

type usageResult struct {
	usage MonthlyUsage
	err   error
}

func requestUsage(service *Service, ctx context.Context, email string) <-chan usageResult {
	result := make(chan usageResult, 1)
	go func() {
		usage, err := service.GetMonthlyUsage(ctx, email, 2026, 6)
		result <- usageResult{usage, err}
	}()
	return result
}

func receiveUsage(t *testing.T, results <-chan usageResult) usageResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(time.Second):
		t.Fatal("usage request did not finish promptly")
		return usageResult{}
	}
}

func waitForWaiterCount(t *testing.T, service *Service, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		service.mu.Lock()
		count := 0
		for _, call := range service.inflight {
			count += call.waiters
		}
		service.mu.Unlock()
		if count == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("waiters = %d, want %d", count, want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
