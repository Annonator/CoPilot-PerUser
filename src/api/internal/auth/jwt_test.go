package auth

import (
	"testing"
	"time"
)

func TestManagerValidatesSignedToken(t *testing.T) {
	manager := Manager{Secret: []byte("test-secret"), Now: func() time.Time {
		return time.Unix(1_800_000_000, 0)
	}}
	token, err := manager.Sign(Claims{
		Email: "user@company.name",
		Name:  "Test User",
	}, time.Hour)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	claims, err := manager.Validate("Bearer " + token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if claims.Email != "user@company.name" {
		t.Fatalf("Email = %q", claims.Email)
	}
}

func TestManagerRejectsExpiredToken(t *testing.T) {
	manager := Manager{Secret: []byte("test-secret"), Now: func() time.Time {
		return time.Unix(1_800_000_000, 0)
	}}
	token, err := manager.Sign(Claims{Email: "user@company.name"}, -time.Minute)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	_, err = manager.Validate("Bearer " + token)
	if err == nil {
		t.Fatal("Validate() error = nil, want expired token error")
	}
}

func TestAllowedDomain(t *testing.T) {
	if !AllowedDomain("User@Company.Name", []string{"company.name"}) {
		t.Fatal("AllowedDomain returned false")
	}
	if AllowedDomain("user@other.test", []string{"company.name"}) {
		t.Fatal("AllowedDomain returned true for other domain")
	}
}
