// Package session provides session authentication and management for the application.
package session

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/stashapp/stash/pkg/logger"
)

type key int

const (
	contextVisitedPlugins key = iota
	contextLocalRequest
)

const (
	userIDKey = "userID"
	// must match the current session token of the user
	sessionTokenKey       = "sessionToken"
	visitedPluginHooksKey = "visitedPluginsHooks"
	// marks a session issued to a plugin, which has the ADMIN role
	pluginSessionKey = "plugin"
)

const (
	ApiKeyHeader    = "ApiKey"
	ApiKeyParameter = "apikey"
)

const (
	cookieName      = "session"
	usernameFormKey = "username"
	passwordFormKey = "password"
)

type InvalidCredentialsError struct {
	Username string
}

func (e InvalidCredentialsError) Error() string {
	// don't leak the username
	return "invalid credentials"
}

var ErrUnauthorized = errors.New("unauthorized")

type Authenticator interface {
	LoginRequired(ctx context.Context) bool
	// Login validates the credentials and returns the session token of the user.
	Login(ctx context.Context, username string, password string) (string, error)
}

type Store struct {
	sessionStore  *sessions.CookieStore
	authenticator Authenticator
	config        SessionConfig
}

func NewStore(c SessionConfig, a Authenticator) *Store {
	ret := &Store{
		sessionStore:  sessions.NewCookieStore(c.GetSessionStoreKey()),
		config:        c,
		authenticator: a,
	}

	ret.sessionStore.MaxAge(c.GetMaxSessionAge())
	ret.sessionStore.Options.SameSite = http.SameSiteLaxMode

	return ret
}

func (s *Store) LoginRequired(ctx context.Context) bool {
	return s.authenticator.LoginRequired(ctx)
}

func (s *Store) Login(w http.ResponseWriter, r *http.Request) error {
	username := r.FormValue(usernameFormKey)
	password := r.FormValue(passwordFormKey)

	// authenticate the user
	token, err := s.authenticator.Login(r.Context(), username, password)
	if err != nil {
		return &InvalidCredentialsError{Username: username}
	}

	logger.Infof("User %s logged in", username)

	return s.CreateSession(w, r, username, token)
}

// CreateSession stores a login session of the user, with its current session
// token, in the session cookie.
func (s *Store) CreateSession(w http.ResponseWriter, r *http.Request, username string, token string) error {
	// ignore error - we want a new session regardless
	newSession, _ := s.sessionStore.Get(r, cookieName)

	newSession.Values[userIDKey] = username
	newSession.Values[sessionTokenKey] = token
	// a login session is never a plugin session
	delete(newSession.Values, pluginSessionKey)

	// the authentication middleware may already have refreshed the cookie on
	// this request; drop that header so only the new cookie is sent
	dropSetCookie(w.Header(), cookieName)

	return newSession.Save(r, w)
}

func (s *Store) Logout(w http.ResponseWriter, r *http.Request) error {
	// a session cookie that cannot be decoded is removed as well
	session, err := s.sessionStore.Get(r, cookieName)
	if err != nil {
		logger.Debugf("logout: invalid session cookie: %v", err)
	}
	if session == nil {
		session = sessions.NewSession(s.sessionStore, cookieName)
	}

	userID, _ := session.Values[userIDKey].(string)

	delete(session.Values, userIDKey)
	delete(session.Values, sessionTokenKey)
	delete(session.Values, pluginSessionKey)
	session.Options.MaxAge = -1

	// the authentication middleware may already have refreshed the cookie on
	// this request; drop that header so only the expired cookie is sent
	dropSetCookie(w.Header(), cookieName)

	if err := s.sessionStore.Save(r, w, session); err != nil {
		return err
	}

	logger.Infof("User %s logged out", userID)

	return nil
}

// dropSetCookie removes the Set-Cookie headers of the named cookie from h.
func dropSetCookie(h http.Header, name string) {
	prefix := name + "="
	var kept []string
	for _, c := range h.Values("Set-Cookie") {
		if !strings.HasPrefix(c, prefix) {
			kept = append(kept, c)
		}
	}
	h.Del("Set-Cookie")
	for _, c := range kept {
		h.Add("Set-Cookie", c)
	}
}

// SessionUser is the user of a session.
type SessionUser struct {
	// UserID is empty if there is no valid session.
	UserID string
	// Token is the session token of the user at the time of login.
	Token string
	// Plugin is true if the session was issued by the server to a plugin.
	Plugin bool
}

// GetSessionUser returns the user stored in the session cookie.
// Returns an empty user ID if there is no valid session.
func (s *Store) GetSessionUser(w http.ResponseWriter, r *http.Request) (SessionUser, error) {
	session, err := s.sessionStore.Get(r, cookieName)
	// ignore errors and treat as an empty user id, so that we handle expired
	// cookie
	if err != nil {
		return SessionUser{}, nil
	}

	if !session.IsNew {
		var ret SessionUser
		ret.UserID, _ = session.Values[userIDKey].(string)
		ret.Token, _ = session.Values[sessionTokenKey].(string)
		ret.Plugin, _ = session.Values[pluginSessionKey].(bool)

		// refresh the cookie
		err = session.Save(r, w)
		if err != nil {
			return SessionUser{}, err
		}

		return ret, nil
	}

	return SessionUser{}, nil
}

func GetRequestApiKey(r *http.Request) string {
	apiKey := r.Header.Get(ApiKeyHeader)

	// try getting the api key as a query parameter
	if apiKey == "" {
		apiKey = r.URL.Query().Get(ApiKeyParameter)
	}

	return apiKey
}
