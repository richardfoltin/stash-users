package api

import (
	"context"
	"net/url"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/signedurl"
)

// userSigningKey returns the HMAC signing key for a given user.
func userSigningKey(c *config.Config, _ string) []byte {
	return c.GetJWTSignKey()
}

// userAPIKey returns the global API key, which authenticates with the ADMIN
// role, for administrators and when authentication is not configured.
func userAPIKey(ctx context.Context, c *config.Config) string {
	if u := models.GetCurrentUser(ctx); u != nil && !u.IsAdmin() {
		return ""
	}

	return c.GetAPIKey()
}

// signedParams generates signed URL query parameters for the given path prefix and user.
func signedParams(c *config.Config, userID string, prefix string) url.Values {
	secret := userSigningKey(c, userID)
	cid := signedurl.GenerateCredentialID(secret, userID)
	expires := time.Now().Add(c.GetSignedURLExpiry())
	return signedurl.SignPrefix(prefix, secret, cid, expires)
}

// resolveCredentialID maps a credential ID back to a user and their signing key.
func resolveCredentialID(ctx context.Context, c *config.Config, cid string) (*models.User, []byte, bool) {
	users, err := c.UserStore.AllUsers(ctx)
	if err != nil {
		logger.Errorf("error getting users: %v", err)
		return nil, nil, false
	}

	for _, u := range users {
		// locked users cannot authenticate
		if len(u.Roles) == 0 {
			continue
		}

		secret := userSigningKey(c, u.Username)
		if signedurl.GenerateCredentialID(secret, u.Username) == cid {
			return u, secret, true
		}
	}
	return nil, nil, false
}
