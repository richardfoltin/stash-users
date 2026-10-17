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

func TestHandleLogin_ReturnURLSameOrigin(t *testing.T) {
	a := newAutoLoginTest(t)
	a.createUser("admin", models.Roles{models.RoleEnumAdmin}, false)
	a.createUser("editor", models.Roles{models.RoleEnumModify}, true)

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
	a.createUser("admin", models.Roles{models.RoleEnumAdmin}, false)

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
