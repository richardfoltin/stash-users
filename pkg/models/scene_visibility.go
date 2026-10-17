package models

import (
	"fmt"
	"io"
	"strconv"
)

// SceneVisibility decides who can see a scene.
type SceneVisibility string

const (
	SceneVisibilityEveryone   SceneVisibility = "EVERYONE"
	SceneVisibilityRestricted SceneVisibility = "RESTRICTED"
	SceneVisibilityPrivate    SceneVisibility = "PRIVATE"
)

// ParseSceneVisibility returns the visibility with the name. It also accepts
// READ, MODIFY and ADMIN, the values that were stored before the
// visibilities were named.
func ParseSceneVisibility(s string) (SceneVisibility, bool) {
	switch s {
	case string(SceneVisibilityEveryone), "READ":
		return SceneVisibilityEveryone, true
	case string(SceneVisibilityRestricted), "MODIFY":
		return SceneVisibilityRestricted, true
	case string(SceneVisibilityPrivate), "ADMIN":
		return SceneVisibilityPrivate, true
	}

	return "", false
}

func (v SceneVisibility) IsValid() bool {
	switch v {
	case SceneVisibilityEveryone, SceneVisibilityRestricted, SceneVisibilityPrivate:
		return true
	}
	return false
}

func (v SceneVisibility) String() string {
	return string(v)
}

func (v *SceneVisibility) UnmarshalGQL(i interface{}) error {
	str, ok := i.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	parsed, ok := ParseSceneVisibility(str)
	if !ok {
		return fmt.Errorf("%s is not a valid SceneVisibility", str)
	}

	*v = parsed
	return nil
}

func (v SceneVisibility) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(v.String()))
}
