package auth

type UserRole string

const (
	UserRoleGuest  UserRole = "guest"
	UserRoleMember UserRole = "member"
	UserRoleMod    UserRole = "moderator"
	UserRoleAdmin  UserRole = "admin"
)

type UserSettings struct {
	Theme         string `db:"name=theme,type=varchar(50),default='light'"`
	Language      string `db:"name=language,type=varchar(10),default='en'"`
	Notifications bool   `db:"name=notifications,type=bool,default=true"`
}
