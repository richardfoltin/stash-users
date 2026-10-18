package models

import "context"

// PersonalSavedFilterIDOffset is the id above which saved filters are
// personal. The personal filters are numbered from it, so that the id of a
// saved filter is unique among the shared and the personal filters.
const PersonalSavedFilterIDOffset = 1000000000

// IsPersonalSavedFilterID returns true if the id is that of a personal saved
// filter.
func IsPersonalSavedFilterID(id int) bool {
	return id > PersonalSavedFilterIDOffset
}

// PersonalSavedFilterReader reads the personal saved filters of a user, which
// only that user sees. user is the name of the user.
type PersonalSavedFilterReader interface {
	// Find returns the filter with the id, or nil if the user has no such
	// filter.
	Find(ctx context.Context, user string, id int) (*SavedFilter, error)
	FindByMode(ctx context.Context, user string, mode FilterMode) ([]*SavedFilter, error)
	All(ctx context.Context, user string) ([]*SavedFilter, error)
}

// PersonalSavedFilterWriter writes the personal saved filters of a user.
type PersonalSavedFilterWriter interface {
	Create(ctx context.Context, user string, obj *SavedFilter) error
	Update(ctx context.Context, user string, obj *SavedFilter) error
	Destroy(ctx context.Context, user string, id int) error
}

type PersonalSavedFilterReaderWriter interface {
	PersonalSavedFilterReader
	PersonalSavedFilterWriter
}
