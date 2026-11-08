package usage

import (
	"context"
	"errors"
	"testing"
	"time"
)

// These cases also run against the original implementation to demonstrate the
// cancellation bug without depending on the new shared-work configuration.
func TestServiceCanceledWaitersReturnBeforeBillingCompletes(t *testing.T) {
	for _, cancelOwner := range []bool{false, true} {
		name := "follower"
		if cancelOwner {
			name = "owner"
		}
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			defer close(release)
			billing := &fakeBilling{wait: release}
			resolver := &fakeResolver{login: "alice"}
			service := NewService(ServiceConfig{
				Resolver: resolver, Billing: billing, CacheTTL: time.Minute,
				Now: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ownerCtx := context.Background()
			if cancelOwner {
				ownerCtx = ctx
			}
			owner := make(chan error, 1)
			go func() {
				_, err := service.GetMonthlyUsage(ownerCtx, "alice@example.test", 2026, 6)
				owner <- err
			}()
			waitForBillingRequest(t, billing)
			waiting := owner
			if !cancelOwner {
				waiting = make(chan error, 1)
				go func() {
					_, err := service.GetMonthlyUsage(ctx, "alias@example.test", 2026, 6)
					waiting <- err
				}()
				deadline := time.After(time.Second)
				for {
					resolver.mu.Lock()
					joined := len(resolver.emails) == 2
					resolver.mu.Unlock()
					if joined {
						break
					}
					select {
					case <-deadline:
						t.Fatal("follower did not resolve its identity")
					default:
						time.Sleep(time.Millisecond)
					}
				}
			}
			cancel()
			select {
			case err := <-waiting:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled request error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("canceled request remained blocked on upstream billing")
			}
		})
	}
}
