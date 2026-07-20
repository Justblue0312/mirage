package metrics

import "time"

type Event struct {
	_ struct{} `db:"name=events,comment=User activity events for analytics,partitioned(RANGE,created_at),pk(id,created_at)"`

	ID int64 `db:"identity,type=bigserial"`

	UserID    *int64 `db:"name=user_id,type=bigint,null,ref=users.id ON DELETE SET NULL,comment=nil for anonymous events"`
	SessionID string `db:"name=session_id,type=varchar(255),notnull,comment=Browser session identifier"`
	EventType string `db:"name=event_type,type=varchar(100),notnull,comment=e.g. page_view, click, scroll, purchase"`

	ResourceType string `db:"name=resource_type,type=varchar(50),null,comment=e.g. post, product, comment"`
	ResourceID   *int64 `db:"name=resource_id,type=bigint,null"`

	Path      string `db:"name=path,type=varchar(1024),notnull,comment=URL path"`
	Referrer  string `db:"name=referrer,type=varchar(1024),null"`
	UserAgent string `db:"name=user_agent,type=text,null,comment=Full user agent string"`
	IPAddress string `db:"name=ip_address,type=inet,null,comment=Client IP for geolocation"`

	MetadataJSON string `db:"name=metadata_json,type=jsonb,null,comment=Flexible event-specific data"`

	DurationMs *int `db:"name=duration_ms,type=int,null,comment=Event duration in milliseconds"`

	CreatedAt time.Time `db:"name=created_at,type=timestamptz,notnull,default=NOW()"`
}
