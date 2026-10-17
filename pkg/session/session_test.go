package session

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDropSetCookie(t *testing.T) {
	h := http.Header{}
	h.Add("Set-Cookie", "session=fresh; Path=/; Max-Age=3600")
	h.Add("Set-Cookie", "other=1; Path=/")
	h.Add("Set-Cookie", "sessionx=2; Path=/")

	dropSetCookie(h, "session")

	assert.Equal(t, []string{"other=1; Path=/", "sessionx=2; Path=/"}, h.Values("Set-Cookie"))
}
