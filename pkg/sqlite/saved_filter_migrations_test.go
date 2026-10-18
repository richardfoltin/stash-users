package sqlite_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// savedFilterMigrations are the migrations that mention saved_filters.
// user_saved_filters has the same filter columns, but no migration changes
// it: when upstream adds a migration that changes saved_filters, make the same
// change to user_saved_filters (personal_saved_filter.go), then list the
// migration here.
var savedFilterMigrations = []string{
	"25_saved_filters.up.sql",
	"49_postmigrate.go",
	"49_saved_filter_refactor.up.sql",
	"60_postmigrate.go",
}

func TestSavedFilterMigrationsMirrored(t *testing.T) {
	entries, err := os.ReadDir("migrations")
	require.NoError(t, err)

	var found []string
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("migrations", e.Name()))
		require.NoError(t, err)

		if strings.Contains(string(data), "saved_filters") {
			found = append(found, e.Name())
		}
	}
	sort.Strings(found)

	assert.Equal(t, savedFilterMigrations, found, "a migration mentions saved_filters: make the same change to user_saved_filters, then list it in savedFilterMigrations")
}
