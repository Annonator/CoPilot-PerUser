package auth

import (
	"strings"
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

func TestManagerRejectsTamperedPayload(t *testing.T) {
	manager := Manager{Secret: []byte("test-secret"), Now: func() time.Time {
		return time.Unix(1_800_000_000, 0)
	}}
	token, err := manager.Sign(Claims{Email: "user@company.name"}, time.Hour)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	parts := strings.Split(token, ".")
	parts[1], err = encodeJSON(Claims{
		Email: "attacker@company.name",
		Exp:   time.Unix(1_800_000_000, 0).Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("encodeJSON() error = %v", err)
	}

	_, err = manager.Validate("Bearer " + strings.Join(parts, "."))
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid signature error")
	}
}

func TestManagerRejectsWrongSecret(t *testing.T) {
	signer := Manager{Secret: []byte("test-secret"), Now: func() time.Time {
		return time.Unix(1_800_000_000, 0)
	}}
	validator := Manager{Secret: []byte("other-secret"), Now: signer.Now}
	token, err := signer.Sign(Claims{Email: "user@company.name"}, time.Hour)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	_, err = validator.Validate("Bearer " + token)
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid signature error")
	}
}

func TestManagerRejectsMalformedBearerHeader(t *testing.T) {
	manager := Manager{Secret: []byte("test-secret"), Now: func() time.Time {
		return time.Unix(1_800_000_000, 0)
	}}
	token, err := manager.Sign(Claims{Email: "user@company.name"}, time.Hour)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	_, err = manager.Validate(token)
	if err == nil {
		t.Fatal("Validate() error = nil, want missing bearer token error")
	}
}

func TestManagerRejectsMissingEmailClaim(t *testing.T) {
	manager := Manager{Secret: []byte("test-secret"), Now: func() time.Time {
		return time.Unix(1_800_000_000, 0)
	}}
	token, err := manager.Sign(Claims{Name: "Test User"}, time.Hour)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	_, err = manager.Validate("Bearer " + token)
	if err == nil {
		t.Fatal("Validate() error = nil, want missing email error")
	}
}

func TestManagerRejectsWrongJWTHeaderAlgorithm(t *testing.T) {
	manager := Manager{Secret: []byte("test-secret"), Now: func() time.Time {
		return time.Unix(1_800_000_000, 0)
	}}
	header, err := encodeJSON(map[string]string{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("encodeJSON(header) error = %v", err)
	}
	payload, err := encodeJSON(Claims{
		Email: "user@company.name",
		Exp:   time.Unix(1_800_000_000, 0).Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("encodeJSON(payload) error = %v", err)
	}
	signingInput := header + "." + payload
	token := signingInput + "." + manager.signature(signingInput)

	_, err = manager.Validate("Bearer " + token)
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid header error")
	}
}

func TestAllowedDomain(t *testing.T) {
	if !AllowedDomain("User@Company.Name", []string{"company.name"}) {
		t.Fatal("AllowedDomain returned false")
	}
	if AllowedDomain("user@other.test", []string{"company.name"}) {
		t.Fatal("AllowedDomain returned true for other domain")
	}
	if AllowedDomain("user@other.test@company.name", []string{"company.name"}) {
		t.Fatal("AllowedDomain returned true for malformed email")
	}
}
