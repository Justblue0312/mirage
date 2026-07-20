package content

import "time"

type PostStatus string

const (
	PostStatusDraft     PostStatus = "draft"
	PostStatusPublished PostStatus = "published"
	PostStatusArchived  PostStatus = "archived"
)

type Post struct {
	_ struct{} `db:"name=posts,comment=Blog posts with content and metadata"`

	ID          int64      `db:"pk,identity,type=bigserial"`
	UserID      int64      `db:"name=user_id,type=bigint,notnull,ref=users.id ON DELETE CASCADE,comment=Post author"`
	Title       string     `db:"name=title,type=varchar(500),notnull,comment=Post title"`
	Slug        string     `db:"name=slug,type=varchar(500),notnull,unique,comment=URL-safe identifier"`
	Excerpt     string     `db:"name=excerpt,type=text,null,comment=Short preview"`
	Body        string     `db:"name=body,type=text,notnull,comment=Full post content in markdown"`
	Status      PostStatus `db:"name=status,type=post_status,notnull,default='draft'"`
	PublishedAt *time.Time `db:"name=published_at,type=timestamptz,null,comment=When post was published"`

	ViewCount int `db:"name=view_count,type=int,default=0,comment=Total page views"`

	IsPinned bool `db:"name=is_pinned,type=bool,default=false,comment=Pinned to top"`
	IsLocked bool `db:"name=is_locked,type=bool,default=false,comment=Comments locked"`

	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt time.Time `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
}

type Comment struct {
	_ struct{} `db:"name=comments,comment=Threaded comments on posts"`

	ID       int64  `db:"pk,identity,type=bigserial"`
	PostID   int64  `db:"name=post_id,type=bigint,notnull,ref=posts.id ON DELETE CASCADE"`
	UserID   int64  `db:"name=user_id,type=bigint,notnull,ref=users.id ON DELETE CASCADE"`
	ParentID *int64 `db:"name=parent_id,type=bigint,null,ref=comments.id ON DELETE CASCADE,comment=Parent comment for threading"`

	Body string `db:"name=body,type=text,notnull,comment=Comment text"`

	IsApproved bool `db:"name=is_approved,type=bool,default=false"`
	IsSpam     bool `db:"name=is_spam,type=bool,default=false"`

	CreatedAt time.Time  `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
	UpdatedAt time.Time  `db:"name=updated_at,type=timestamptz,notnull,default=NOW()"`
	DeletedAt *time.Time `db:"name=deleted_at,type=timestamptz,null,comment=Soft delete timestamp"`
}

type Tag struct {
	_ struct{} `db:"name=tags,comment=Content tags for categorization"`

	ID   int64  `db:"pk,identity,type=bigserial"`
	Name string `db:"name=name,type=varchar(100),notnull,unique,comment=Tag display name"`
	Slug string `db:"name=slug,type=varchar(100),notnull,unique,comment=URL-safe tag identifier"`

	Color string `db:"name=color,type=varchar(7),null,comment=Hex color code e.g. #FF0000"`

	PostCount int `db:"name=post_count,type=int,default=0,comment=Denormalized count of posts with this tag"`

	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
}

type PostTag struct {
	_ struct{} `db:"name=post_tags,comment=Many-to-many relationship between posts and tags"`

	PostID int64 `db:"name=post_id,type=bigint,notnull,ref=posts.id ON DELETE CASCADE"`
	TagID  int64 `db:"name=tag_id,type=bigint,notnull,ref=tags.id ON DELETE CASCADE"`

	SortOrder int `db:"name=sort_order,type=int,default=0,comment=Display order within post"`

	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
}
