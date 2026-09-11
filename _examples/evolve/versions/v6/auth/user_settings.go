package auth

type UserRole string

const (
	UserRoleGuest  UserRole = "guest"
	UserRoleMember UserRole = "member"
	UserRoleMod    UserRole = "moderator"
	UserRoleAdmin  UserRole = "admin"
)

type UserSettings struct {
	Theme         string `json:"theme"`
	Language      string `json:"language"`
	Notifications bool   `json:"notifications"`
}

// UserPreferences demonstrates slice-of-struct stored as jsonb.
type UserPreferences struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
