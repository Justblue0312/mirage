package common

import "time"

type Timestamps struct {
	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}

type SoftDelete struct {
	DeletedAt *time.Time `db:"name=deleted_at,type=timestamptz,null"`
	DeletedBy string     `db:"name=deleted_by,type=varchar(255),null"`
}
