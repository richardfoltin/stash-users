package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestSceneVisibleClause(t *testing.T) {
	withUser := func(role models.RoleEnum) context.Context {
		return models.SetCurrentUser(testCtx, models.User{Username: "user", Roles: models.Roles{role}})
	}

	const hiddenFmt = "NOT EXISTS (SELECT 1 FROM scene_visibility AS hidden_scene WHERE hidden_scene.scene_id = scenes.id AND hidden_scene.min_role IN (%s))"

	tests := []struct {
		name   string
		ctx    context.Context
		hidden string
	}{
		{"no user", testCtx, ""},
		{"admin", withUser(models.RoleEnumAdmin), ""},
		{"editor", withUser(models.RoleEnumModify), "'ADMIN'"},
		{"viewer", withUser(models.RoleEnumRead), "'MODIFY', 'ADMIN'"},
		{"viewer role", models.SetViewerRole(testCtx, models.RoleEnumRead), "'MODIFY', 'ADMIN'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sceneVisibleClause(tt.ctx, "scenes.id")
			if tt.hidden == "" {
				assert.Empty(t, got)
				assert.Empty(t, andSceneVisible(tt.ctx, "scenes.id"))
				assert.Empty(t, fileVisibleClause(tt.ctx, "files.id"))
				assert.Equal(t, "SELECT 1 FROM s WHERE x", withSceneVisible(tt.ctx, "SELECT 1 FROM s WHERE x%s", "scenes.id"))
				return
			}

			want := fmt.Sprintf(hiddenFmt, tt.hidden)
			assert.Equal(t, want, got)
			assert.Equal(t, " AND "+want, andSceneVisible(tt.ctx, "scenes.id"))
			assert.Equal(t, " WHERE "+want, whereSceneVisibleSQL(tt.ctx, "scenes.id"))
			assert.Equal(t, want, visibleClause(tt.ctx, sceneTable))
			assert.Contains(t, visibleClause(tt.ctx, sceneMarkerTable), "hidden_scene.scene_id = scene_markers.scene_id")
			assert.Contains(t, visibleClause(tt.ctx, fileTable), "file_scene.file_id = files.id")
			assert.Empty(t, visibleClause(tt.ctx, performerTable))
		})
	}
}

func TestAddFilterKeepsOrInside(t *testing.T) {
	f := &filterBuilder{}
	f.addWhere("a = 1")
	f.or(&filterBuilder{whereClauses: []sqlClause{{sql: "b = 1"}}})

	query := sceneRepository.newQuery()
	query.addWhere("visible")
	assert.NoError(t, query.addFilter(f))

	// the OR must not escape the visibility condition
	assert.Equal(t, []string{"visible", "((a = 1) OR ((b = 1)))"}, query.whereClauses)
}
