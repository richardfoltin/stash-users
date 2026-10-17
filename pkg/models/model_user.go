package models

type User struct {
	Username string
	Roles    Roles
	// AutoLogin is true if requests from the local network without a session
	// are logged in as this user. At most one user has it set.
	AutoLogin bool
}

// IsAdmin returns true if the user has the ADMIN role.
func (u User) IsAdmin() bool {
	return u.Roles.HasRole(RoleEnumAdmin)
}
