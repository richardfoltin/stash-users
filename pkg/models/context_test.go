package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestViewerRole(t *testing.T) {
	withUser := func(roles ...RoleEnum) context.Context {
		return SetCurrentUser(context.Background(), User{Username: "user", Roles: roles})
	}

	tests := []struct {
		name           string
		ctx            context.Context
		wantRole       RoleEnum
		wantRestricted bool
	}{
		{"no user", context.Background(), "", false},
		{"admin", withUser(RoleEnumAdmin), "", false},
		{"editor", withUser(RoleEnumModify), RoleEnumModify, true},
		{"viewer", withUser(RoleEnumRead), RoleEnumRead, true},
		{"locked user", withUser(), RoleEnumRead, true},
		{"viewer role without user", SetViewerRole(context.Background(), RoleEnumRead), RoleEnumRead, true},
		{"viewer role over admin", SetViewerRole(withUser(RoleEnumAdmin), RoleEnumRead), RoleEnumRead, true},
		{"admin viewer role over viewer", SetViewerRole(withUser(RoleEnumRead), RoleEnumAdmin), RoleEnumAdmin, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, restricted := ViewerRole(tt.ctx)
			assert.Equal(t, tt.wantRestricted, restricted, "restricted")
			if tt.wantRestricted {
				assert.Equal(t, tt.wantRole, role, "role")
			}
		})
	}
}
