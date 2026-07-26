package favorites

import (
	"time"

	mirage "github.com/justblue/mirage"
)

// Base provides common fields for all favorites.
// Note: Base.ID is a bigserial primary key.
type Base struct {
	ID        int64     `db:"pk,identity,type=bigserial"`
	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}

// UserFav demonstrates overriding an embedded field.
// Base.ID is bigserial, but UserFav redefines ID as a plain bigint
// with a different column name (fav_id) and adds a unique constraint.
// The outer field completely replaces the embedded one.
type UserFav struct {
	Base

	ID       int64  `db:"name=fav_id,type=bigint,notnull"`
	UserID   int64  `db:"name=user_id,type=bigint,notnull,ref=users.id ON DELETE CASCADE"`
	Position int    `db:"name=position,type=int,default=0"`
	Note     string `db:"name=note,type=text,null"`
}

// ProductFav shows a simpler override — same column name, different attributes.
// Base.ID is bigserial, ProductFav redefines it as bigint NOT NULL.
type ProductFav struct {
	Base

	ID        int64 `db:"name=id,type=bigint,notnull"`
	ProductID int64 `db:"name=product_id,type=bigint,notnull,ref=products.id ON DELETE CASCADE"`
}

// PlainFav shows override with composite unique constraint via Register.
type PlainFav struct {
	Base

	ID         int64  `db:"name=id,type=bigint,notnull"`
	TargetType string `db:"name=target_type,type=varchar(50),notnull"`
	TargetID   int64  `db:"name=target_id,type=bigint,notnull"`
}

func init() {
	mirage.Register(mirage.Table{
		StructName: "UserFav",
		Name:       "user_favorites",
		Description: "User favorite items with position ordering",
		Uniques: []mirage.UniqueConstraint{
			{Name: "uq_user_fav_user_position", Columns: []string{"user_id", "position"}},
		},
	})
	mirage.Register(mirage.Table{
		StructName:  "ProductFav",
		Name:        "product_favorites",
		Description: "Product favorites (simple override of Base.ID)",
	})
	mirage.Register(mirage.Table{
		StructName:  "PlainFav",
		Name:        "plain_favorites",
		Description: "Generic favorites with composite unique and index on target",
		Uniques: []mirage.UniqueConstraint{
			{Name: "uq_plain_fav_target", Columns: []string{"target_type", "target_id"}},
		},
		Indexes: []mirage.Index{
			{Name: "idx_plain_fav_target", Columns: []string{"target_type", "target_id"}},
		},
	})
}
