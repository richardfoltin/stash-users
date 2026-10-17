package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testUI is the index page of a built UI
var testUI = fstest.MapFS{
	"index.html": {Data: []byte(`<head>
    <link rel="shortcut icon" href="favicon.ico" />
    <script type="module" crossorigin src="./assets/index-a1.js"></script>
    <link rel="stylesheet" crossorigin href="./assets/index-b2.css">
  </head>`)},
}

func TestSameOriginURL(t *testing.T) {
	for _, u := range []string{"/", "/scenes?q=1", "/prefix/scenes/1#top", "/%2F%2Fexample.com"} {
		assert.Equal(t, u, sameOriginURL(u), u)
	}

	for _, u := range []string{
		"",
		"scenes",
		"//example.com",
		"///example.com",
		`/\example.com`,
		"/\t/example.com",
		"https://example.com/",
		"javascript:alert(1)",
		"http:/example.com",
	} {
		assert.Empty(t, sameOriginURL(u), u)
	}
}

// loginAutoTest serves the auto login switch endpoint behind the
// authentication middleware.
func loginAutoTest(t *testing.T) (*autoLoginTest, func(remoteAddr string, cookie *http.Cookie) *httptest.ResponseRecorder) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, true)

	getStore := func() *session.Store { return a.store }
	handler := RequestIPMiddleware(authenticateHandler(a.users, getStore)(handleLoginAuto(a.users, getStore)))

	post := func(remoteAddr string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, loginAutoEndpoint, nil)
		r.RemoteAddr = remoteAddr
		if cookie != nil {
			r.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec
	}

	return a, post
}

// assertSessionOf asserts that the session after the response, refreshed or
// not, is still the session of the user.
func assertSessionOf(t *testing.T, a *autoLoginTest, rec *httptest.ResponseRecorder, old *http.Cookie, username string) {
	t.Helper()

	c := newSessionCookie(rec)
	if c == nil {
		c = old
	}
	got := a.serve(http.MethodGet, "/", publicAddr, withCookie(c))
	assert.Equal(t, username, got.Header().Get("X-Test-User"), "the session must not be replaced")
}

func TestHandleLoginAuto_Allowed(t *testing.T) {
	a, post := loginAutoTest(t)
	admin := a.loginCookie("admin", "adminpw")

	// a signed in admin on the local network switches to the auto login user
	rec := post(localAddr, admin)
	require.Equal(t, http.StatusOK, rec.Code)
	c := newSessionCookie(rec)
	require.NotNil(t, c, "a new session cookie must be set")

	got := a.serve(http.MethodGet, "/", publicAddr, withCookie(c))
	assert.Equal(t, "editor", got.Header().Get("X-Test-User"))

	// without a session too
	rec = post(localAddr, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotNil(t, newSessionCookie(rec))
}

func TestHandleLoginAuto_RefusedFromPublicAddress(t *testing.T) {
	a, post := loginAutoTest(t)
	admin := a.loginCookie("admin", "adminpw")

	rec := post(publicAddr, admin)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assertSessionOf(t, a, rec, admin, "admin")
}

func TestHandleLoginAuto_NoAutoLoginUser(t *testing.T) {
	a, post := loginAutoTest(t)
	admin := a.loginCookie("admin", "adminpw")
	require.NoError(t, a.users.UpdateUser(context.Background(), "editor", models.User{Username: "editor", Role: models.RoleIDEditor}))

	rec := post(localAddr, admin)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assertSessionOf(t, a, rec, admin, "admin")

	// nor is a request without a session logged in
	rec = post(localAddr, nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Nil(t, newSessionCookie(rec))
}

func TestHandleLogin_SwitchMode(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, true)

	handler := RequestIPMiddleware(authenticateHandler(a.users, func() *session.Store {
		return a.store
	})(handleLogin(a.users, a.cfg, testUI, plugin.NewCache(a.cfg))))

	loginPage := func(target string, remoteAddr string, cookie *http.Cookie) string {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.RemoteAddr = remoteAddr
		if cookie != nil {
			r.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		require.Equal(t, http.StatusOK, rec.Code)
		return rec.Body.String()
	}

	switchURL := loginEndpoint + "?" + url.Values{switchParam: {"1"}, returnURLParam: {"/scenes"}}.Encode()

	// a signed in admin is offered the auto login user, by a POST
	body := loginPage(switchURL, localAddr, a.loginCookie("admin", "adminpw"))
	assert.Contains(t, body, `data-switch="true"`)
	assert.Contains(t, body, `data-current-user="admin"`)
	assert.Contains(t, body, `id="auto-login-switch"`)
	assert.Contains(t, body, "Continue as editor")
	assert.NotContains(t, body, `id="auto-login-link"`)
	assert.Contains(t, body, `id="login-cancel" href="/scenes"`)

	// not from a public address
	body = loginPage(switchURL, publicAddr, a.loginCookie("admin", "adminpw"))
	assert.Contains(t, body, `data-switch="true"`)
	assert.NotContains(t, body, `id="auto-login-switch"`)

	// not to the user that is signed in
	body = loginPage(switchURL, localAddr, a.loginCookie("editor", "editorpw"))
	assert.Contains(t, body, `data-current-user="editor"`)
	assert.NotContains(t, body, `id="auto-login-switch"`)

	// without a session there is nothing to switch from
	body = loginPage(switchURL, localAddr, nil)
	assert.Contains(t, body, `data-switch="false"`)
	assert.Contains(t, body, `id="auto-login-link"`)
}

func TestHandleLogin_ReturnURLSameOrigin(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.RoleIDAdmin, false)
	a.createUser("editor", models.RoleIDEditor, true)

	handler := RequestIPMiddleware(authenticateHandler(a.users, func() *session.Store {
		return a.store
	})(handleLogin(a.users, a.cfg, testUI, plugin.NewCache(a.cfg))))

	r := httptest.NewRequest(http.MethodGet, loginEndpoint+"?"+url.Values{returnURLParam: {"https://example.com/"}}.Encode(), nil)
	r.RemoteAddr = localAddr
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "example.com")
	assert.Contains(t, body, `id="returnURL" name="returnURL" value=""`)
	assert.Contains(t, body, `id="auto-login-link" href="/"`)
}

func TestHandleLogin_Stylesheets(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.RoleIDAdmin, false)

	handler := RequestIPMiddleware(authenticateHandler(a.users, func() *session.Store {
		return a.store
	})(handleLogin(a.users, a.cfg, testUI, plugin.NewCache(a.cfg))))

	r := httptest.NewRequest(http.MethodGet, loginEndpoint, nil)
	r.RemoteAddr = publicAddr
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	// the CSS bundle of the app, then the login page and the custom CSS
	app := strings.Index(body, `<link rel="stylesheet" crossorigin href="./assets/index-b2.css">`)
	login := strings.Index(body, `<link rel="stylesheet" href="login/login.css">`)
	custom := strings.Index(body, `<link rel="stylesheet" href="css">`)
	assert.True(t, app >= 0 && app < login && login < custom, "the stylesheets must be in the order of the app")
	assert.NotContains(t, body, "index-a1.js")
}

func TestPluginStylesheets(t *testing.T) {
	plugins := []*plugin.Plugin{
		{ID: "Tweaks", Enabled: true, UI: plugin.PluginUI{Requires: []string{"Theme"}, CSS: []string{"tweaks.css"}}},
		{ID: "Script", Enabled: true, UI: plugin.PluginUI{Javascript: []string{"script.js"}}},
		{ID: "Disabled", UI: plugin.PluginUI{CSS: []string{"disabled.css"}}},
		{ID: "Other", Enabled: true, UI: plugin.PluginUI{CSS: []string{"other.css"}}},
		{ID: "Theme", Enabled: true, UI: plugin.PluginUI{CSS: []string{"theme.css"}, ExternalCSS: []string{"https://example.com/font.css"}}},
	}

	// as in the UI: each plugin after the plugins it requires, and the
	// others in reverse order
	ctx := context.WithValue(context.Background(), BaseURLCtxKey, "http://stash.local/prefix")
	assert.Equal(t, []string{
		"http://stash.local/prefix/plugin/Other/css",
		"https://example.com/font.css",
		"http://stash.local/prefix/plugin/Theme/css",
		"http://stash.local/prefix/plugin/Tweaks/css",
	}, pluginStylesheets(ctx, plugins))
}
