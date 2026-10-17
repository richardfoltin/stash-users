package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	localAddr  = "192.168.1.10:50000"
	publicAddr = "203.0.113.7:50000"
)

type autoLoginTest struct {
	t       *testing.T
	cfg     *config.Config
	users   *user.Service
	store   *session.Store
	handler http.Handler
}

// newAutoLoginTest returns the authentication middleware backed by an empty
// config file. The wrapped handler serves every request it receives.
func newAutoLoginTest(t *testing.T) *autoLoginTest {
	t.Helper()

	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	cfg.SetString(config.SessionStoreKey, "test-session-store-key")

	users := &user.Service{Store: cfg.UserStore, APIKeyUserID: manager.GetUserIDFromAPIKey}
	store := session.NewStore(cfg, users)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := models.GetCurrentUser(r.Context()); u != nil {
			w.Header().Set("X-Test-User", u.Username)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestIPMiddleware(authenticateHandler(users, func() *session.Store {
		return store
	})(next))

	return &autoLoginTest{t: t, cfg: cfg, users: users, store: store, handler: handler}
}

// loginCookie returns the session cookie of a password login of the user.
func (a *autoLoginTest) loginCookie(username string, password string) *http.Cookie {
	a.t.Helper()

	token, err := a.users.Login(context.Background(), username, password)
	require.NoError(a.t, err)

	rec := httptest.NewRecorder()
	require.NoError(a.t, a.store.CreateSession(rec, httptest.NewRequest(http.MethodPost, "/login", nil), username, token))
	c := newSessionCookie(rec)
	require.NotNil(a.t, c)
	return c
}

func (a *autoLoginTest) createUser(name string, roles models.Roles, autoLogin bool) {
	a.t.Helper()
	require.NoError(a.t, a.users.CreateUser(context.Background(), models.User{Username: name, Roles: roles, AutoLogin: autoLogin}, name+"pw"))
}

func (a *autoLoginTest) serve(method string, target string, remoteAddr string, modify func(r *http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	r.RemoteAddr = remoteAddr
	if modify != nil {
		modify(r)
	}

	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, r)
	return rec
}

// newSessionCookie returns the session cookie set on the response, or nil.
func newSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" && c.MaxAge >= 0 {
			return c
		}
	}
	return nil
}

func withCookie(c *http.Cookie) func(r *http.Request) {
	return func(r *http.Request) {
		r.AddCookie(c)
	}
}

// assertAutoLogin asserts that the request was served as the user, with a new
// session cookie, and returns the cookie.
func assertAutoLogin(t *testing.T, rec *httptest.ResponseRecorder, username string) *http.Cookie {
	t.Helper()

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, username, rec.Header().Get("X-Test-User"))
	c := newSessionCookie(rec)
	assert.NotNil(t, c, "session cookie must be set")
	return c
}

// assertNoAutoLogin asserts that the request was not logged in, and that no
// session cookie was set.
func assertNoAutoLogin(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()

	assert.Equal(t, wantStatus, rec.Code)
	assert.Empty(t, rec.Header().Get("X-Test-User"))
	assert.Nil(t, newSessionCookie(rec), "session cookie must not be set")
}

func TestAuthenticateHandler_AutoLogin(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.Roles{models.RoleEnumAdmin}, false)
	a.createUser("editor", models.Roles{models.RoleEnumModify}, true)

	for _, addr := range []string{"127.0.0.1:50000", "[::1]:50000", localAddr, "10.1.2.3:50000", "100.64.1.2:50000"} {
		t.Run(addr, func(t *testing.T) {
			assertAutoLogin(t, a.serve(http.MethodGet, "/", addr, nil), "editor")
			assertAutoLogin(t, a.serve(http.MethodPost, "/graphql", addr, nil), "editor")
		})
	}

	// the auto login session is a normal session of the user
	c := assertAutoLogin(t, a.serve(http.MethodGet, "/scenes", localAddr, nil), "editor")
	rec := a.serve(http.MethodGet, "/scenes", publicAddr, withCookie(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "editor", rec.Header().Get("X-Test-User"))

	// a valid session of another user is kept
	rec = a.serve(http.MethodGet, "/scenes", localAddr, withCookie(a.loginCookie("admin", "adminpw")))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "admin", rec.Header().Get("X-Test-User"))
}

func TestAuthenticateHandler_AutoLoginNonLocal(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.Roles{models.RoleEnumAdmin}, false)
	a.createUser("editor", models.Roles{models.RoleEnumModify}, true)

	// requests from public addresses are redirected to the login page, or
	// refused for graphql and non-html requests
	rec := a.serve(http.MethodGet, "/scenes?q=1", publicAddr, nil)
	assertNoAutoLogin(t, rec, http.StatusFound)
	assert.Equal(t, "/login?"+url.Values{returnURLParam: {"/scenes?q=1"}}.Encode(), rec.Header().Get("Location"))
	assertNoAutoLogin(t, a.serve(http.MethodPost, "/graphql", publicAddr, nil), http.StatusUnauthorized)
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/image/1/thumbnail.jpg", publicAddr, nil), http.StatusUnauthorized)

	// a local proxy forwarding a request from a public client
	forwardedFrom := func(client string) func(r *http.Request) {
		return func(r *http.Request) {
			r.Header.Set("X-Forwarded-For", client)
		}
	}
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", "127.0.0.1:50000", forwardedFrom("203.0.113.7")), http.StatusFound)
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, forwardedFrom("203.0.113.7, 192.168.1.1")), http.StatusFound)

	// a local proxy forwarding a request from a local client
	assertAutoLogin(t, a.serve(http.MethodGet, "/", "127.0.0.1:50000", forwardedFrom("192.168.1.20")), "editor")
}

func TestAuthenticateHandler_AutoLoginExcludedPaths(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.Roles{models.RoleEnumAdmin}, false)
	a.createUser("editor", models.Roles{models.RoleEnumModify}, true)

	// the login page and its assets, and logout, are not logged in
	for _, path := range []string{"/login", "/login/login.css", "/login/locale", "/logout", "/css", "/assets/index.js"} {
		assertNoAutoLogin(t, a.serve(http.MethodGet, path, localAddr, nil), http.StatusOK)
	}
}

func TestAuthenticateHandler_AutoLoginAPIKey(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.Roles{models.RoleEnumAdmin}, false)
	a.createUser("editor", models.Roles{models.RoleEnumModify}, true)

	key, err := manager.GenerateAPIKey("admin")
	require.NoError(t, err)
	a.cfg.SetString(config.ApiKey, key)

	withAPIKey := func(k string) func(r *http.Request) {
		return func(r *http.Request) {
			r.Header.Set(session.ApiKeyHeader, k)
		}
	}

	// the API key takes precedence over auto login
	rec := a.serve(http.MethodPost, "/graphql", localAddr, withAPIKey(key))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "admin", rec.Header().Get("X-Test-User"))
	assert.Nil(t, newSessionCookie(rec))

	// an invalid API key is refused
	assertNoAutoLogin(t, a.serve(http.MethodPost, "/graphql", localAddr, withAPIKey("wrong")), http.StatusUnauthorized)
}

func TestAuthenticateHandler_AutoLoginNoUsers(t *testing.T) {
	a := newAutoLoginTest(t)

	// without users, requests are served without a user
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusOK)
	assertNoAutoLogin(t, a.serve(http.MethodPost, "/graphql", "127.0.0.1:50000", nil), http.StatusOK)
}

func TestAuthenticateHandler_AutoLoginUserChanges(t *testing.T) {
	ctx := context.Background()
	a := newAutoLoginTest(t)
	a.createUser("admin", models.Roles{models.RoleEnumAdmin}, false)
	a.createUser("editor", models.Roles{models.RoleEnumModify}, false)

	// no user is marked
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusFound)

	require.NoError(t, a.users.UpdateUser(ctx, "editor", models.User{Username: "editor", Roles: models.Roles{models.RoleEnumModify}, AutoLogin: true}))
	c := assertAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), "editor")

	// a password change ends the session; the next request is logged in
	// again with a new session
	require.NoError(t, a.users.ChangeUserPassword(ctx, "editor", "new"))
	rec := a.serve(http.MethodGet, "/", publicAddr, withCookie(c))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Empty(t, rec.Header().Get("X-Test-User"))
	newCookie := assertAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, withCookie(c)), "editor")
	require.NotNil(t, newCookie)
	assert.NotEqual(t, c.Value, newCookie.Value)
	rec = a.serve(http.MethodGet, "/", publicAddr, withCookie(newCookie))
	assert.Equal(t, "editor", rec.Header().Get("X-Test-User"))

	// a locked user is not logged in automatically
	require.NoError(t, a.users.UpdateUser(ctx, "editor", models.User{Username: "editor", Roles: models.Roles{}, AutoLogin: true}))
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusFound)

	// nor is a user whose mark was cleared
	require.NoError(t, a.users.UpdateUser(ctx, "editor", models.User{Username: "editor", Roles: models.Roles{models.RoleEnumModify}}))
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusFound)

	// nor is a removed user
	require.NoError(t, a.users.UpdateUser(ctx, "editor", models.User{Username: "editor", Roles: models.Roles{models.RoleEnumModify}, AutoLogin: true}))
	require.NoError(t, a.users.DeleteUser(ctx, "editor"))
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusFound)
}
