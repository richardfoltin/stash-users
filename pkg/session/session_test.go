package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropSetCookie(t *testing.T) {
	h := http.Header{}
	h.Add("Set-Cookie", "session=fresh; Path=/; Max-Age=3600")
	h.Add("Set-Cookie", "other=1; Path=/")
	h.Add("Set-Cookie", "sessionx=2; Path=/")

	dropSetCookie(h, "session")

	assert.Equal(t, []string{"other=1; Path=/", "sessionx=2; Path=/"}, h.Values("Set-Cookie"))
}

type testSessionConfig struct{}

func (testSessionConfig) GetUsername() string        { return "" }
func (testSessionConfig) GetAPIKey() string          { return "" }
func (testSessionConfig) GetSessionStoreKey() []byte { return []byte("test-session-store-key") }
func (testSessionConfig) GetMaxSessionAge() int      { return 3600 }

type testAuthenticator struct{}

func (testAuthenticator) LoginRequired(ctx context.Context) bool { return true }
func (testAuthenticator) Login(ctx context.Context, username string, password string) (string, error) {
	return "", errors.New("not implemented")
}

// sessionCookies returns the session cookies set on the response.
func sessionCookies(rec *httptest.ResponseRecorder) []*http.Cookie {
	var ret []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			ret = append(ret, c)
		}
	}
	return ret
}

func TestStore_CreateSession(t *testing.T) {
	s := NewStore(testSessionConfig{}, testAuthenticator{})

	// start from a plugin session, which must not carry over
	pluginCookie := s.MakePluginCookie(models.SetCurrentUser(context.Background(), models.User{Username: "other"}))
	require.NotNil(t, pluginCookie)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(pluginCookie)
	rec := httptest.NewRecorder()

	// the session is refreshed first, as the authentication middleware does
	su, err := s.GetSessionUser(rec, r)
	require.NoError(t, err)
	require.True(t, su.Plugin)

	require.NoError(t, s.CreateSession(rec, r, "editor", "token"))

	cookies := sessionCookies(rec)
	require.Len(t, cookies, 1, "only the new session cookie is sent")

	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookies[0])
	su, err = s.GetSessionUser(httptest.NewRecorder(), r)
	require.NoError(t, err)
	assert.Equal(t, SessionUser{UserID: "editor", Token: "token", Plugin: false}, su)
}
