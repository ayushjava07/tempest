package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/tempest-io/tempest/internal/persistence"
	"github.com/tempest-io/tempest/pkg/errors"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

type Role string

const (
	RoleReader   Role = "reader"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type Action string

const (
	ActionRead            Action = "read"
	ActionSubmit          Action = "submit"
	ActionCancel          Action = "cancel"
	ActionPublish         Action = "publish"
	ActionDelete          Action = "delete"
	ActionManageWebhooks  Action = "manage_webhooks"
	ActionManageTokens    Action = "manage_tokens"
)

var allowed = map[Role]map[Action]bool{
	RoleReader: {
		ActionRead: true,
	},
	RoleOperator: {
		ActionRead: true, ActionSubmit: true, ActionCancel: true,
		ActionPublish: true,
	},
	RoleAdmin: {
		ActionRead: true, ActionSubmit: true, ActionCancel: true,
		ActionPublish: true, ActionDelete: true,
		ActionManageWebhooks: true, ActionManageTokens: true,
	},
}

func (r Role) Allow(a Action) bool {
	actions, ok := allowed[r]
	if !ok {
		return false
	}
	return actions[a]
}

func ParseRole(s string) (Role, error) {
	switch Role(strings.ToLower(strings.TrimSpace(s))) {
	case RoleReader, RoleOperator, RoleAdmin:
		return Role(strings.ToLower(strings.TrimSpace(s))), nil
	default:
		return "", fmt.Errorf("unknown role %q", s)
	}
}

type Principal struct {
	ID        string
	Namespace ttypes.Namespace
	Role      Role
}

type Resolver struct {
	store persistence.Store
}

func NewResolver(store persistence.Store) *Resolver {
	return &Resolver{store: store}
}

func (r *Resolver) Resolve(ctx context.Context, tokenHash string) (*Principal, error) {
	tok, err := r.store.LookupTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, errors.UnauthenticatedError(errors.ErrUnauthenticated)
	}
	return &Principal{
		ID: tok.ID, Namespace: tok.Namespace, Role: Role(tok.Role),
	}, nil
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	return p, ok
}

func HashToken(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return fmt.Sprintf("%x", h)
}

func BearerToken(hdr http.Header) (string, bool) {
	auth := hdr.Get("Authorization")
	if auth == "" {
		return "", false
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	return strings.TrimPrefix(auth, "Bearer "), true
}
