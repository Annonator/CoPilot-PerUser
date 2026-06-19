package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	Exp   int64  `json:"exp"`
}

type Manager struct {
	Secret []byte
	Now    func() time.Time
}

func (m Manager) Sign(claims Claims, ttl time.Duration) (string, error) {
	now := m.now()
	claims.Exp = now.Add(ttl).Unix()

	header, err := encodeJSON(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := encodeJSON(claims)
	if err != nil {
		return "", err
	}
	signingInput := header + "." + payload
	return signingInput + "." + m.signature(signingInput), nil
}

func (m Manager) Validate(authorization string) (Claims, error) {
	if !strings.HasPrefix(authorization, "Bearer ") {
		return Claims{}, errors.New("missing bearer token")
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New("invalid token format")
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	headerData, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, fmt.Errorf("decode header: %w", err)
	}
	if err := json.Unmarshal(headerData, &header); err != nil {
		return Claims{}, fmt.Errorf("unmarshal header: %w", err)
	}
	if header.Alg != "HS256" || header.Typ != "JWT" {
		return Claims{}, errors.New("invalid token header")
	}
	signingInput := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(m.signature(signingInput))) {
		return Claims{}, errors.New("invalid token signature")
	}
	var claims Claims
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("decode payload: %w", err)
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, fmt.Errorf("unmarshal payload: %w", err)
	}
	if claims.Email == "" {
		return Claims{}, errors.New("email claim is required")
	}
	if claims.Exp <= m.now().Unix() {
		return Claims{}, errors.New("token is expired")
	}
	return claims, nil
}

func AllowedDomain(email string, domains []string) bool {
	if strings.Count(email, "@") != 1 {
		return false
	}
	at := strings.Index(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, allowed := range domains {
		if domain == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func (m Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m Manager) signature(signingInput string) string {
	mac := hmac.New(sha256.New, m.Secret)
	_, _ = mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func encodeJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
