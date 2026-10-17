package models

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSceneVisibility(t *testing.T) {
	tests := []struct {
		in     string
		want   SceneVisibility
		wantOk bool
	}{
		{"EVERYONE", SceneVisibilityEveryone, true},
		{"RESTRICTED", SceneVisibilityRestricted, true},
		{"PRIVATE", SceneVisibilityPrivate, true},
		// the values stored before the levels were named
		{"READ", SceneVisibilityEveryone, true},
		{"MODIFY", SceneVisibilityRestricted, true},
		{"ADMIN", SceneVisibilityPrivate, true},
		{"x", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := ParseSceneVisibility(tt.in)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSceneVisibilityGQL(t *testing.T) {
	var v SceneVisibility
	assert.NoError(t, v.UnmarshalGQL("MODIFY"))
	assert.Equal(t, SceneVisibilityRestricted, v)
	assert.Error(t, v.UnmarshalGQL("x"))
	assert.Error(t, v.UnmarshalGQL(1))

	var buf bytes.Buffer
	SceneVisibilityPrivate.MarshalGQL(&buf)
	assert.Equal(t, `"PRIVATE"`, buf.String())
}
