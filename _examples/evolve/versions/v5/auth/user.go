package auth

import (
	"time"

	mirage "github.com/justblue/mirage"
)

type User struct {
	ID int64 `db:"pk,identity,type=bigserial"`

	Username string `db:"name=username,type=varchar(100),notnull,unique_index=idx_users_username_lower,comment=Unique login handle"`
	Email    string `db:"name=email,type=varchar(255),notnull,unique,comment=Primary email address"`
	Password string `db:"name=password,type=varchar(255),notnull,password,comment=bcrypt hashed password"`

	Role     UserRole `db:"name=role,type=user_role,notnull,default='member'"`
	Status   string   `db:"name=status,type=varchar(20),notnull,default='active',check=status IN ('active','suspended','deleted')"`
	Settings UserSettings `db:"name=settings,type=jsonb,notnull,default='{}'"`
	Preferences []UserPreferences `db:"name=preferences,type=jsonb,null"`
	Extra       *UserSettings     `db:"name=extra,type=jsonb,null"`

	FirstName string `db:"name=first_name,type=varchar(100),null"`
	LastName  string `db:"name=last_name,type=varchar(200),null"`
	AvatarURL string `db:"name=avatar_uri,type=varchar(512),null"`

	LoginCount int `db:"name=login_count,type=int,default=1,comment=Total successful logins"`

	LastLoginAt *time.Time `db:"name=last_login_at,type=timestamptz,null"`
	CreatedAt   time.Time  `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt   time.Time  `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}

func init() {
	mirage.Register(mirage.Table{
		StructName:  "User",
		Name:        "users",
		Description: "User accounts with authentication and profile data",
	})
}
