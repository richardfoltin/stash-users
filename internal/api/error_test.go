package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactArgsJSON(t *testing.T) {
	type userInput struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	type changePasswordInput struct {
		ExistingPassword string `json:"existingPassword"`
		NewPassword      string `json:"newPassword"`
	}

	type stashBox struct {
		Endpoint string `json:"endpoint"`
		APIKey   string `json:"api_key"`
	}

	args := map[string]interface{}{
		"input": userInput{Name: "viewer", Password: "secret-1"},
		"change": &changePasswordInput{
			ExistingPassword: "secret-2",
			NewPassword:      "secret-3",
		},
		"nested": map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"initialPassword": "secret-4"},
			},
		},
		"stashBoxes": []stashBox{{Endpoint: "https://stashdb.org", APIKey: "secret-5"}},
		"apiKey":     "secret-6",
		"interface":  map[string]interface{}{"handyKey": "secret-7"},
	}

	data, err := json.Marshal(args)
	require.NoError(t, err)

	redacted := string(redactArgsJSON(data))

	for _, secret := range []string{"secret-1", "secret-2", "secret-3", "secret-4", "secret-5", "secret-6", "secret-7"} {
		assert.NotContains(t, redacted, secret)
	}

	assert.Contains(t, redacted, `"name":"viewer"`)
	assert.Contains(t, redacted, `"endpoint":"https://stashdb.org"`)
	assert.Contains(t, redacted, redactedValue)
}
