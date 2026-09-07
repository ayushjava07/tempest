package fencing

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrMissingContextToken = errors.New("fencing: no fencing token found in execution context")
)

type fencingKey struct{}

// WithFencingToken binds a fencing token into the context.
func WithFencingToken(ctx context.Context, token FencingToken) context.Context {
	return context.WithValue(ctx, fencingKey{}, token)
}

// FromContext extracts the fencing token from the context, if present.
func FromContext(ctx context.Context) (FencingToken, bool) {
	val := ctx.Value(fencingKey{})
	if val == nil {
		return FencingToken{}, false
	}
	tok, ok := val.(FencingToken)
	return tok, ok
}

// RequireFencingToken returns the fencing token or an error if missing or expired.
func RequireFencingToken(ctx context.Context) (FencingToken, error) {
	tok, ok := FromContext(ctx)
	if !ok {
		return FencingToken{}, ErrMissingContextToken
	}
	if tok.IsExpired() {
		return tok, fmt.Errorf("%w: token %d expired at %s", ErrTokenExpired, tok.Token, tok.ExpiresAt)
	}
	return tok, nil
}

// DeriveSubStepToken branches a parent token for a parallel sub-step with scoped owner.
func DeriveSubStepToken(parent FencingToken, subStepID string) FencingToken {
	return FencingToken{
		Resource:  parent.Resource,
		Token:     parent.Token,
		Owner:     fmt.Sprintf("%s/%s", parent.Owner, subStepID),
		Epoch:     parent.Epoch,
		IssuedAt:  parent.IssuedAt,
		ExpiresAt: parent.ExpiresAt,
	}
}

// WithDerivedSubStep binds a scoped sub-step token into a child context.
func WithDerivedSubStep(ctx context.Context, subStepID string) (context.Context, FencingToken, error) {
	parent, err := RequireFencingToken(ctx)
	if err != nil {
		return ctx, FencingToken{}, err
	}
	child := DeriveSubStepToken(parent, subStepID)
	return WithFencingToken(ctx, child), child, nil
}
