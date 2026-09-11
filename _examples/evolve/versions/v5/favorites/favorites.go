package favorites

import (
	"time"

	mirage "github.com/Justblue0312/mirage"
)

// Base provides common fields for all favorites.
type Base struct {
	ID        int64     `db:"pk,identity,type=bigserial"`
	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}

// UserFav demonstrates overriding an embedded field.
type UserFav struct {
	Base

	ID       int64  `db:"name=fav_id,type=bigint,notnull"`
	UserID   int64  `db:"name=user_id,type=bigint,notnull,ref=users.id ON DELETE CASCADE"`
	Position int    `db:"name=position,type=int,default=0"`
	Note     string `db:"name=note,type=text,null"`
}

// ProductFav shows a simpler override.
type ProductFav struct {
	Base

	ID        int64 `db:"name=id,type=bigint,notnull"`
	ProductID int64 `db:"name=product_id,type=bigint,notnull,ref=products.id ON DELETE CASCADE"`
}

func init() {
	mirage.Register(mirage.Table{
		StructName:  "UserFav",
		Name:        "user_favorites",
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
}
