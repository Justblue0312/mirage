package catalog

import "time"

type Category struct {
	_ struct{} `db:"name=categories,comment=Product category tree with self-referencing hierarchy"`

	ID          int64  `db:"pk,identity,type=bigserial"`
	ParentID    *int64 `db:"name=parent_id,type=bigint,null,ref=categories.id ON DELETE SET NULL,comment=Parent category for tree structure"`
	Name        string `db:"name=name,type=varchar(255),notnull,comment=Category display name"`
	Slug        string `db:"name=slug,type=varchar(255),notnull,unique,comment=URL-safe identifier"`
	Description string `db:"name=description,type=text,null"`
	SortOrder   int    `db:"name=sort_order,type=int,default=0,comment=Display order among siblings"`
	IsActive    bool   `db:"name=is_active,type=bool,default=true,comment=Visible in navigation"`

	Depth int `db:"name=depth,type=int,default=0,comment=Tree depth level (0=root)"`

	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}

type Product struct {
	_ struct{} `db:"name=products,comment=Product catalog with pricing and inventory"`

	ID int64 `db:"pk,identity,type=bigserial"`

	CategoryID int64  `db:"name=category_id,type=bigint,notnull,ref=categories.id ON DELETE RESTRICT"`
	SKU        string `db:"name=sku,type=varchar(100),notnull,unique,comment=Stock keeping unit"`
	Name       string `db:"name=name,type=varchar(500),notnull,comment=Product display name"`
	Slug       string `db:"name=slug,type=varchar(500),notnull,unique"`
	Summary    string `db:"name=summary,type=text,null,comment=Short description"`
	Body       string `db:"name=body,type=text,null,comment=Full product description"`

	PriceCents   int64  `db:"name=price_cents,type=bigint,notnull,check=price_cents >= 0,comment=Price in smallest currency unit"`
	CurrencyCode string `db:"name=currency_code,type=char(3),notnull,default='USD',comment=ISO 4217 currency code"`

	WeightGrams *int `db:"name=weight_grams,type=int,null,check=weight_grams > 0"`
	WidthMM     *int `db:"name=width_mm,type=int,null"`
	HeightMM    *int `db:"name=height_mm,type=int,null"`
	DepthMM     *int `db:"name=depth_mm,type=int,null"`

	StockQuantity int    `db:"name=stock_quantity,type=int,notnull,default=0,check=stock_quantity >= 0,comment=Current inventory level"`
	StockStatus   string `db:"name=stock_status,type=varchar(20),notnull,default='in_stock',check=stock_status IN ('in_stock','out_of_stock','preorder','discontinued')"`

	IsPublished bool `db:"name=is_published,type=bool,default=false,comment=Visible in catalog"`
	IsFeatured  bool `db:"name=is_featured,type=bool,default=false,comment=Featured on homepage"`

	MetaTitle       string `db:"name=meta_title,type=varchar(255),null,comment=SEO title override"`
	MetaDescription string `db:"name=meta_description,type=text,null,comment=SEO meta description"`

	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}

type ProductImage struct {
	_ struct{} `db:"name=product_images,comment=Product image gallery with ordering"`

	ID        int64  `db:"pk,identity,type=bigserial"`
	ProductID int64  `db:"name=product_id,type=bigint,notnull,ref=products.id ON DELETE CASCADE"`
	URL       string `db:"name=url,type=varchar(1024),notnull,comment=Image URL"`
	AltText   string `db:"name=alt_text,type=varchar(255),null,comment=Accessibility alt text"`
	SortOrder int    `db:"name=sort_order,type=int,default=0,comment=Display order"`
	IsPrimary bool   `db:"name=is_primary,type=bool,default=false,comment=Main product image"`

	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
}

type ProductVariant struct {
	_ struct{} `db:"name=product_variants,comment=Product variants for size/color/etc"`

	ID        int64  `db:"pk,identity,type=bigserial"`
	ProductID int64  `db:"name=product_id,type=bigint,notnull,ref=products.id ON DELETE CASCADE"`
	SKU       string `db:"name=sku,type=varchar(100),notnull,unique,comment=Variant-specific SKU"`
	Name      string `db:"name=name,type=varchar(255),notnull,comment=e.g. 'Large / Blue'"`

	PriceCents    int64 `db:"name=price_cents,type=bigint,notnull,check=price_cents >= 0"`
	StockQuantity int   `db:"name=stock_quantity,type=int,notnull,default=0,check=stock_quantity >= 0"`

	AttributesJSON string `db:"name=attributes_json,type=jsonb,null,comment=Flexible key-value attributes"`

	IsAvailable bool `db:"name=is_available,type=bool,default=true"`

	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}
