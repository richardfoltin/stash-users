package models

type User struct {
	Username string
	// Role is the ID of the role of the user, or empty if the user is locked.
	Role string
	// Permissions are the permissions of the role of the user.
	Permissions Permissions
	// AutoLogin is true if requests from the local network without a session
	// are logged in as this user. At most one user has it set.
	AutoLogin bool
	// HasPassword is false for a user without a password, who can only be
	// logged in automatically.
	HasPassword bool
}

// IsAdmin returns true if the user has the Admin role.
func (u User) IsAdmin() bool {
	return u.Role == RoleIDAdmin
}

// IsLocked returns true if the user has no role, and cannot log in.
func (u User) IsLocked() bool {
	return u.Role == ""
}

func (u User) HasPermission(p Permission) bool {
	return u.IsAdmin() || u.Permissions.Has(p)
}
