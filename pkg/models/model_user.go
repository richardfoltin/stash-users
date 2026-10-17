package models

type User struct {
	Username string
	Roles    Roles
}

// IsAdmin returns true if the user has the ADMIN role.
func (u User) IsAdmin() bool {
	return u.Roles.HasRole(RoleEnumAdmin)
}
