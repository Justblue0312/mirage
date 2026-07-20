package auth

import "github.com/justblue/mirage"

// UUIDExtension provides the uuid_generate_v4() function used for primary keys.
var _ = mirage.Register(mirage.Extension{
	Name:        "uuid-ossp",
	Schema:      "public",
	IfNotExists: true,
})

// UpdateTimestamps is a trigger function that sets updated_at on row modification.
var _ = mirage.Register(mirage.Function{
	Name:        "update_timestamps",
	Description: "Sets updated_at to current timestamp on INSERT or UPDATE",
	Language:    "plpgsql",
	ReturnType:  "trigger",
	Volatility:  "IMMUTABLE",
	Security:    "DEFINER",
	Body: `BEGIN
	NEW.updated_at = now();
	RETURN NEW;
END;`,
})

// UsersUpdatedTrigger fires UpdateTimestamps before every update on users.
var _ = mirage.Register(mirage.Trigger{
	Name:        "trg_users_updated_at",
	Description: "Auto-update updated_at column on users",
	Table:       "users",
	Timing:      "BEFORE",
	Events:      []string{"INSERT", "UPDATE"},
	Function:    "update_timestamps",
})

// ActiveUsers is a view of non-deleted users.
var _ = mirage.Register(mirage.View{
	Name:        "active_users",
	Description: "Users that are not suspended or deleted",
	Query:       "SELECT id, username, email, role, created_at FROM users WHERE status = 'active'",
})

// GrantAppRole gives the application role read/write access to users.
var _ = mirage.Register(mirage.Grant{
	ObjectType: "table",
	ObjectName: "users",
	Privileges: []string{"SELECT", "INSERT", "UPDATE"},
	Roles:      []string{"app_role"},
})

// UserIsolationPolicy restricts rows to the authenticated user.
var _ = mirage.Register(mirage.Policy{
	Name:       "user_isolation",
	Table:      "users",
	Command:    "ALL",
	Roles:      []string{"app_role"},
	Using:      "auth.uid() = id",
	Permissive: "PERMISSIVE",
})
