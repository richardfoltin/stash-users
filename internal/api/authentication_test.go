package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin"
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

func (a *autoLoginTest) createUser(name string, roleID string, autoLogin bool) {
	a.t.Helper()
	require.NoError(a.t, a.users.CreateUser(context.Background(), models.User{Username: name, Role: roleID, AutoLogin: autoLogin}, name+"pw"))
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
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, true)

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
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, true)

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
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, true)

	// the login page and its assets, and logout, are not logged in
	for _, path := range []string{"/login", "/login/login.css", "/login/locale", "/logout", "/css", "/assets/index.js", "/favicon.ico", "/plugin/example-theme/css"} {
		assertNoAutoLogin(t, a.serve(http.MethodGet, path, localAddr, nil), http.StatusOK)
	}
}

func TestAllowUnauthenticated(t *testing.T) {
	allowed := func(path string) bool {
		return allowUnauthenticated(httptest.NewRequest(http.MethodGet, path, nil))
	}

	// the login page and what it loads
	for _, path := range []string{"/login", "/login/login.css", "/logout", "/css", "/assets/index-b2.css", "/favicon.ico", "/plugin/example-theme/css"} {
		assert.True(t, allowed(path), path)
	}

	// but not the javascript or the assets of plugins
	for _, path := range []string{"/", "/javascript", "/plugin/example-theme/javascript", "/plugin/example-theme/assets/theme.css", "/plugin/example-theme/css/x", "/plugin/a/b/css", "/plugin/css"} {
		assert.False(t, allowed(path), path)
	}
}

func TestAuthenticateHandler_AutoLoginAPIKey(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, true)

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
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, false)

	// no user is marked
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusFound)

	require.NoError(t, a.users.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: models.RoleIDEditor, AutoLogin: true}))
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
	require.NoError(t, a.users.UpdateUser(ctx, "editor", models.User{Username: "editor", AutoLogin: true}))
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusFound)

	// nor is a user whose mark was cleared
	require.NoError(t, a.users.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: models.RoleIDEditor}))
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusFound)

	// nor is a removed user
	require.NoError(t, a.users.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: models.RoleIDEditor, AutoLogin: true}))
	require.NoError(t, a.users.DeleteUser(ctx, "editor"))
	assertNoAutoLogin(t, a.serve(http.MethodGet, "/", localAddr, nil), http.StatusFound)
}

func TestHandleLogin_ContinueAsAutoLoginUser(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, true)

	// the login page behind the authentication middleware
	handler := RequestIPMiddleware(authenticateHandler(a.users, func() *session.Store {
		return a.store
	})(handleLogin(a.users, a.cfg, testUI, plugin.NewCache(a.cfg))))

	loginPage := func(target string, remoteAddr string, cookie *http.Cookie) string {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		if cookie != nil {
			r.AddCookie(cookie)
			handler.ServeHTTP(rec, r)
		} else {
			handler.ServeHTTP(rec, r)
			assert.Nil(t, newSessionCookie(rec), "the login page is not logged in")
		}
		require.Equal(t, http.StatusOK, rec.Code)
		return rec.Body.String()
	}

	body := loginPage("/login", localAddr, nil)
	assert.Contains(t, body, `id="auto-login-link" href="/"`)
	assert.Contains(t, body, "Continue as editor")

	body = loginPage("/login?"+url.Values{returnURLParam: {"/scenes?q=1"}}.Encode(), localAddr, nil)
	assert.Contains(t, body, `id="auto-login-link" href="/scenes?q=1"`)

	body = loginPage("/login", publicAddr, nil)
	assert.NotContains(t, body, `id="auto-login-link"`)
	assert.Contains(t, body, `id="login-button"`)

	// a request with a valid session would not be logged in automatically
	body = loginPage("/login", localAddr, a.loginCookie("admin", "adminpw"))
	assert.NotContains(t, body, `id="auto-login-link"`)
}

// TestAuthenticateHandler_WebsocketReauthenticates checks that each operation
// on a websocket connection authenticates its user again, as the next request
// would, instead of keeping the user of the request that opened it.
func TestAuthenticateHandler_WebsocketReauthenticates(t *testing.T) {
	ctx := context.Background()
	a := newAutoLoginTest(t)
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, false)

	// connect returns the context of the request that opens a connection
	connect := func(remoteAddr string, modify func(r *http.Request)) context.Context {
		var connCtx context.Context
		h := RequestIPMiddleware(authenticateHandler(a.users, func() *session.Store {
			return a.store
		})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			connCtx = r.Context()
		})))

		r := httptest.NewRequest(http.MethodGet, "/graphql", nil)
		r.RemoteAddr = remoteAddr
		r.Header.Set("Connection", "Upgrade")
		r.Header.Set("Upgrade", "websocket")
		if modify != nil {
			modify(r)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		require.NotNil(t, connCtx)
		return connCtx
	}

	// operation runs an operation on a connection and returns the user it ran
	// as, or nil if it did not run
	operation := func(connCtx context.Context) (*models.User, *graphql.Response) {
		var u *models.User
		resp := reauthenticateOperation(connCtx, func(ctx context.Context) graphql.ResponseHandler {
			u = models.GetCurrentUser(ctx)
			return graphql.OneShot(&graphql.Response{})
		})(connCtx)
		return u, resp
	}

	conn := connect(publicAddr, withCookie(a.loginCookie("editor", "editorpw")))
	u, resp := operation(conn)
	require.Empty(t, resp.Errors)
	require.NotNil(t, u)
	assert.True(t, u.HasPermission(models.PermissionScenesEdit))

	// a permission removed from the role applies to the next operation
	editor, err := a.users.GetRole(ctx, models.RoleIDEditor)
	require.NoError(t, err)
	editor.Permissions = models.Permissions{models.PermissionRecordActivity}
	require.NoError(t, a.users.UpdateRole(ctx, *editor))
	u, resp = operation(conn)
	require.Empty(t, resp.Errors)
	require.NotNil(t, u)
	assert.False(t, u.HasPermission(models.PermissionScenesEdit))

	// a password change ends the session
	require.NoError(t, a.users.ChangeUserPassword(ctx, "editor", "new"))
	u, resp = operation(conn)
	assert.Nil(t, u)
	assert.NotEmpty(t, resp.Errors)

	// a session issued to a plugin keeps the Admin role
	pluginCookie := a.store.MakePluginCookie(models.SetCurrentUser(ctx, models.User{Username: "editor", Role: models.RoleIDEditor}))
	require.NotNil(t, pluginCookie)
	u, resp = operation(connect(publicAddr, withCookie(pluginCookie)))
	require.Empty(t, resp.Errors)
	require.NotNil(t, u)
	assert.True(t, u.IsAdmin())

	// a replaced API key ends the connections opened with it
	key, err := manager.GenerateAPIKey("admin")
	require.NoError(t, err)
	a.cfg.SetString(config.ApiKey, key)
	conn = connect(publicAddr, func(r *http.Request) {
		r.Header.Set(session.ApiKeyHeader, key)
	})
	u, resp = operation(conn)
	require.Empty(t, resp.Errors)
	require.NotNil(t, u)
	assert.Equal(t, "admin", u.Username)
	a.cfg.SetString(config.ApiKey, "")
	u, resp = operation(conn)
	assert.Nil(t, u)
	assert.NotEmpty(t, resp.Errors)

	// a connection logged in automatically ends when the mark is cleared
	require.NoError(t, a.users.UpdateUser(ctx, "admin", models.User{Username: "admin", Role: models.RoleIDAdmin, AutoLogin: true}))
	conn = connect(localAddr, nil)
	u, resp = operation(conn)
	require.Empty(t, resp.Errors)
	require.NotNil(t, u)
	assert.Equal(t, "admin", u.Username)
	require.NoError(t, a.users.UpdateUser(ctx, "admin", models.User{Username: "admin", Role: models.RoleIDAdmin}))
	u, resp = operation(conn)
	assert.Nil(t, u)
	assert.NotEmpty(t, resp.Errors)

	// requests that do not open a connection are not authenticated again
	httpCtx := models.SetCurrentUser(ctx, models.User{Username: "gone", Role: models.RoleIDAdmin})
	u, resp = operation(httpCtx)
	require.Empty(t, resp.Errors)
	require.NotNil(t, u)
	assert.Equal(t, "gone", u.Username)
}
