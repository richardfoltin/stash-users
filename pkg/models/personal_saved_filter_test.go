package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPersonalSavedFilterID(t *testing.T) {
	assert.False(t, IsPersonalSavedFilterID(1), "shared")
	assert.False(t, IsPersonalSavedFilterID(PersonalSavedFilterIDOffset), "offset")
	assert.True(t, IsPersonalSavedFilterID(PersonalSavedFilterIDOffset+1), "first personal")
}
