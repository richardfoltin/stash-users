package models

import "context"

type contextKey int

const (
	currentUserKey contextKey = iota
)

func SetCurrentUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, currentUserKey, u)
}

// GetCurrentUser returns the user of the request, or nil if there is none.
func GetCurrentUser(ctx context.Context) *User {
	if u, ok := ctx.Value(currentUserKey).(User); ok {
		return &u
	}

	return nil
}
