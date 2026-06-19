package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrIdentityNotFound  = errors.New("identity not found")
	ErrIdentityAmbiguous = errors.New("identity ambiguous")
)

type Resolver interface {
	ResolveGitHubLogin(ctx context.Context, email string) (string, error)
}

type StaticResolver struct {
	byEmail map[string]string
}

func NewStaticResolver(path string) (*StaticResolver, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read static identity map: %w", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse static identity map: %w", err)
	}
	normalized := make(map[string]string, len(raw))
	for email, login := range raw {
		normalized[strings.ToLower(strings.TrimSpace(email))] = strings.TrimSpace(login)
	}
	return &StaticResolver{byEmail: normalized}, nil
}

func (r *StaticResolver) ResolveGitHubLogin(_ context.Context, email string) (string, error) {
	login, ok := r.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok || login == "" {
		return "", fmt.Errorf("%w: no GitHub login for email %q", ErrIdentityNotFound, email)
	}
	return login, nil
}
