package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/signedurl"
	"github.com/stashapp/stash/pkg/user"
)

func allowUnauthenticated(r *http.Request) bool {
	// #2715 - allow access to UI files
	return strings.HasPrefix(r.URL.Path, loginEndpoint) || r.URL.Path == logoutEndpoint || r.URL.Path == "/css" || strings.HasPrefix(r.URL.Path, "/assets")
}

type UserAuthenticator interface {
	LoginRequired(ctx context.Context) bool
	AuthenticateByAPIKey(ctx context.Context, apiKey string) (*models.User, error)
	AuthenticateSession(ctx context.Context, username string, sessionToken string) (*models.User, error)
	AutoLoginUser(ctx context.Context) (*models.User, string, error)
}

// authenticateSignedRequest checks if the request is a valid signed media request.
// Returns the matched user and true if valid, or nil and false otherwise.
func authenticateSignedRequest(r *http.Request, loginRequired bool) (*models.User, bool) {
	// Only apply to scene stream paths (used by AirPlay/Chromecast devices that can't pass cookies)
	if !strings.HasPrefix(r.URL.Path, "/scene/") {
		return nil, false
	}

	c := config.GetInstance()

	// Signed URLs are only relevant when credentials are configured
	if !loginRequired {
		return nil, false
	}

	// Check for signed URL parameters
	q := r.URL.Query()
	if q.Get(signedurl.CIDParam) == "" || q.Get(signedurl.ExpiresParam) == "" || q.Get(signedurl.SigParam) == "" {
		return nil, false
	}

	// Extract the credential ID and look up the user's signing key.
	// We need the key before we can verify the signature, since in a
	// multi-user setup each user could have their own signing key.
	cid := q.Get(signedurl.CIDParam)
	u, secret, found := resolveCredentialID(r.Context(), c, cid)
	if !found {
		logger.Warnf("signed URL credential ID mismatch")
		return nil, false
	}

	// Verify the signature using the user's signing key
	if _, err := signedurl.VerifyURL(r.URL.Path, q, secret); err != nil {
		logger.Warnf("signed URL verification failed: %v", err)
		return nil, false
	}

	return u, true
}

func httpError(w http.ResponseWriter, r *http.Request, text string, status int) {
	// if request accepts json, return json error response
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"error": "%s"}`, text)
	} else {
		http.Error(w, text, status)
	}
}

// authenticateRequest returns the user of the API key or the session cookie, or
// nil. Returns an error if the API key is invalid.
func authenticateRequest(w http.ResponseWriter, r *http.Request, g UserAuthenticator, store *session.Store, loginRequired bool) (*models.User, error) {
	ctx := r.Context()

	// try to authenticate using api key first
	if apiKey := session.GetRequestApiKey(r); apiKey != "" {
		return g.AuthenticateByAPIKey(ctx, apiKey)
	}

	su, err := store.GetSessionUser(w, r)
	if err != nil {
		return nil, fmt.Errorf("%w: getting session user: %v", user.ErrInternalError, err)
	}

	// sessions issued to plugins by the server have the ADMIN role
	if su.Plugin {
		return &models.User{
			Username: su.UserID,
			Roles:    models.Roles{models.RoleEnumAdmin},
		}, nil
	}

	// session user IDs are only meaningful if users are configured
	if su.UserID == "" || !loginRequired {
		return nil, nil
	}

	u, err := g.AuthenticateSession(ctx, su.UserID, su.Token)
	if err != nil {
		if errors.Is(err, user.ErrInternalError) {
			return nil, err
		}

		// user was deleted or locked, or the session token does not match -
		// treat the session as logged out
		return nil, nil
	}

	return u, nil
}

// autoLoginUser returns the user that a local request without a session is
// logged in as, if any, and the token to store in its session.
func autoLoginUser(ctx context.Context, g UserAuthenticator) (*models.User, string, error) {
	// fail closed - a request without a known IP is not local
	requestIP, err := getRequestIPFromCtx(ctx)
	if err != nil || !isLocalIP(requestIP) {
		return nil, "", nil
	}

	return g.AutoLoginUser(ctx)
}

// autoLogin logs a local request without a session in as the auto login user,
// if any, with a new session.
func autoLogin(w http.ResponseWriter, r *http.Request, g UserAuthenticator, store *session.Store) (*models.User, error) {
	ctx := r.Context()

	u, token, err := autoLoginUser(ctx, g)
	if err != nil || u == nil {
		return nil, err
	}

	if err := store.CreateSession(w, r, u.Username, token); err != nil {
		return nil, fmt.Errorf("%w: creating auto login session: %v", user.ErrInternalError, err)
	}

	// clients that do not keep cookies are logged in on every request
	requestIP, _ := getRequestIPFromCtx(ctx)
	logger.Debugf("User %s logged in automatically from %s", u.Username, requestIP)

	return u, nil
}

// authenticateHandler authenticates each request with the current session
// store.
func authenticateHandler(g UserAuthenticator, getStore func() *session.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := config.GetInstance()
			store := getStore()

			r = session.SetLocalRequest(r)
			ctx := r.Context()

			loginRequired := g.LoginRequired(ctx)

			// Check for signed media requests
			if u, ok := authenticateSignedRequest(r, loginRequired); ok {
				ctx = models.SetCurrentUser(ctx, *u)
				r = r.WithContext(ctx)
				next.ServeHTTP(w, r)
				return
			}

			u, err := authenticateRequest(w, r, g, store, loginRequired)
			if err != nil {
				if errors.Is(err, user.ErrInternalError) {
					logger.Errorf("error authenticating request: %v", err)
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}

				// unauthorized error
				w.Header().Add("WWW-Authenticate", "FormBased")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// reject connections from the public internet if authentication is not configured
			// don't apply for new systems
			if !c.IsNewSystem() && !loginRequired {
				requestIP, err := getRequestIPFromCtx(ctx)
				if err != nil {
					logger.Errorf("error getting request IP: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if err := checkAllowPublicWithoutAuth(c, requestIP); err != nil {
					httpError(w, r, "Access denied: Stash cannot be accessed from public IPs when authentication is not configured", http.StatusForbidden)
					return
				}
			}

			if loginRequired && u == nil && !allowUnauthenticated(r) {
				// log requests from the local network in as the user marked
				// for auto login, if any
				u, err = autoLogin(w, r, g, store)
				if err != nil {
					logger.Errorf("error logging in automatically: %v", err)
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
			}

			if loginRequired {
				// authentication is required
				if u == nil && !allowUnauthenticated(r) {
					// if graphql or a non-webpage was requested, we just return a forbidden error
					ext := path.Ext(r.URL.Path)
					if r.URL.Path == gqlEndpoint || (ext != "" && ext != ".html") {
						w.Header().Add("WWW-Authenticate", "FormBased")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}

					prefix := getProxyPrefix(r)

					// otherwise redirect to the login page
					returnURL := url.URL{
						Path:     prefix + r.URL.Path,
						RawQuery: r.URL.RawQuery,
					}
					q := make(url.Values)
					q.Set(returnURLParam, returnURL.String())
					u := url.URL{
						Path:     prefix + loginEndpoint,
						RawQuery: q.Encode(),
					}
					http.Redirect(w, r, u.String(), http.StatusFound)
					return
				}
			}

			if u != nil {
				// set the user object in the context
				ctx = models.SetCurrentUser(ctx, *u)
			}

			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

func checkAllowPublicWithoutAuth(c *config.Config, requestIP net.IP) error {
	// reject connections from the public internet if authentication is not configured
	// don't apply for new systems
	if c.IsNewSystem() {
		return nil
	}

	if !isLocalIP(requestIP) && !matchIPWhitelist(c, requestIP) {
		return fmt.Errorf("stash accessed from external IP %s", requestIP.String())
	}

	return nil
}

func matchIPWhitelist(c *config.Config, requestIP net.IP) bool {
	nets, addrs := c.GetPublicWhitelist()

	for _, addr := range addrs {
		if addr.Equal(requestIP) {
			return true
		}
	}
	for _, net := range nets {
		if net.Contains(requestIP) {
			return true
		}
	}
	return false
}
