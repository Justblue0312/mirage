package mirage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

// TableChangeType is the type of the table change.
// Available values: INSERT, UPDATE, DELETE.
type TableChangeType string

const (
	// TableChangeTypeInsert is the INSERT table change type.
	TableChangeTypeInsert TableChangeType = "INSERT"
	// TableChangeTypeUpdate is the UPDATE table change type.
	TableChangeTypeUpdate TableChangeType = "UPDATE"
	// TableChangeTypeDelete is the DELETE table change type.
	TableChangeTypeDelete TableChangeType = "DELETE"
)

// changesToString joins changes into the "INSERT OR UPDATE OR DELETE"
// clause used by the generated trigger's AFTER clause. It rejects any value
// outside the three known TableChangeType constants: TableChangeType is a
// plain string type (not a closed Go enum), so nothing stops a caller from
// constructing an arbitrary TableChangeType("..."). Because this string is
// spliced directly into a CREATE TRIGGER statement (see prepareListenTable),
// accepting arbitrary values here would be a SQL injection point.
func changesToString(changes []TableChangeType) (string, error) {
	if len(changes) == 0 {
		return "", nil
	}

	var b strings.Builder
	for i, change := range changes {
		switch change {
		case TableChangeTypeInsert, TableChangeTypeUpdate, TableChangeTypeDelete:
		default:
			return "", fmt.Errorf("mirage: unsupported table change type %q; expected one of INSERT, UPDATE, DELETE", change)
		}
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString(string(change))
	}

	return b.String(), nil
}

type (
	// TableNotification is the notification message sent by the postgresql server
	// when a table change occurs.
	// The subscribed postgres channel is named 'table_change_notifications'.
	// The "old" and "new" fields are the old and new values of the row.
	// The "old" field is only available for UPDATE and DELETE table change types.
	// The "new" field is only available for INSERT and UPDATE table change types.
	// The "old" and "new" fields are raw json values, use the "json.Unmarshal" to decode them.
	// See "DB.ListenTable" method.
	TableNotification[T any] struct {
		Table  string          `json:"table"`
		Change TableChangeType `json:"change"` // INSERT, UPDATE, DELETE.

		New T `json:"new"`
		Old T `json:"old"`

		payload string `json:"-"` /* just in case */
	}

	// TableNotificationJSON is the generic version of the TableNotification.
	TableNotificationJSON = TableNotification[json.RawMessage]
)

// GetPayload returns the raw payload of the notification.
func (tn TableNotification[T]) GetPayload() string {
	return tn.payload
}

// ListenTableOptions is the options for the "DB.ListenTable" method.
type ListenTableOptions struct {
	// Tables map of table name and changes to listen for.
	//
	// Key is the table to listen on for changes.
	// Value is changes is the list of table changes to listen for.
	// Defaults to {"*": ["INSERT", "UPDATE", "DELETE"] }.
	Tables map[string][]TableChangeType

	// Channel is the name of the postgres channel to listen on.
	// Default: "table_change_notifications".
	Channel string

	// Function is the name of the postgres function
	// which is used to notify on table changes, the
	// trigger name is <table_name>_<Function>.
	// Defaults to "table_change_notify".
	Function string
}

var defaultChangesToWatch = []TableChangeType{TableChangeTypeInsert, TableChangeTypeUpdate, TableChangeTypeDelete}

func (opts *ListenTableOptions) setDefaults() {
	if opts.Channel == "" {
		opts.Channel = "table_change_notifications"
	}

	if opts.Function == "" {
		opts.Function = "table_change_notify"
	}

	if len(opts.Tables) == 0 {
		opts.Tables = map[string][]TableChangeType{wildcardTableStr: defaultChangesToWatch}
	}
}

const wildcardTableStr = "*"

// PrepareListenTable prepares the table for listening for live table updates.
// See "db.ListenTable" method for more.
func (db *DB) PrepareListenTable(ctx context.Context, opts *ListenTableOptions) error {
	opts.setDefaults()

	isWildcard := false
	for table := range opts.Tables {
		if table == wildcardTableStr {
			isWildcard = true
			break
		}
	}

	if isWildcard {
		return fmt.Errorf("wildcard table '*' is no longer supported; pass explicit table names")
	}

	if len(opts.Tables) == 0 {
		return nil
	}

	for table, changes := range opts.Tables {
		if err := db.prepareListenTable(ctx, opts.Channel, opts.Function, table, changes); err != nil {
			return err
		}
	}

	return nil
}

// PrepareListenTable prepares the table for listening for live table updates.
// See "db.ListenTable" method for more.
func (db *DB) prepareListenTable(ctx context.Context, channel, function, table string, changes []TableChangeType) error {
	if table == "" {
		return errors.New("empty table name")
	}

	if len(changes) == 0 {
		return nil
	}

	changesClause, err := changesToString(changes)
	if err != nil {
		return err
	}

	// table, function and channel are caller-supplied strings (from
	// ListenTableOptions), not values introspected from the database, so
	// they must be treated as untrusted input and quoted/escaped the same
	// way every other identifier in this package is (see QuoteIdentifier
	// throughout db.go/repository.go). Previously these were interpolated
	// with fmt.Sprintf with no quoting at all -- a table or channel name
	// containing e.g. a double quote or a single quote could break out of
	// the identifier/string-literal context and inject arbitrary SQL/DDL.
	db.tableChangeNotifyMu.Lock()
	if !db.tableChangeNotifyFunctionOnce {
		query := buildNotifyFunctionSQL(function, channel)

		if _, err := db.Exec(ctx, query); err != nil {
			db.tableChangeNotifyMu.Unlock()
			return fmt.Errorf("create or replace function table_change_notify: %w", err)
		}
		db.tableChangeNotifyFunctionOnce = true
	}
	db.tableChangeNotifyMu.Unlock()

	if _, loaded := db.tableChangeNotifyTriggerOnce.LoadOrStore(table, struct{}{}); !loaded {
		triggerName, query := buildNotifyTriggerSQL(table, function, changesClause)

		_, err := db.Exec(ctx, query)
		if err != nil {
			db.tableChangeNotifyTriggerOnce.Delete(table)
			return fmt.Errorf("create trigger %s: %w", triggerName, err)
		}
	}

	return nil
}

// buildNotifyFunctionSQL builds the CREATE OR REPLACE FUNCTION statement
// for the shared table-change-notify trigger function.
//
// function and channel are caller-supplied strings (from
// ListenTableOptions), not values introspected from the database, so they
// must be treated as untrusted input: function is used as a SQL identifier
// and is quoted with QuoteIdentifier (the same helper used everywhere else
// in this package for table/column names); channel is embedded inside a
// single-quoted PL/pgSQL string literal (not an identifier position), so it
// is escaped as a SQL string literal (embedded single quotes doubled)
// rather than identifier-quoted. Previously both were interpolated with
// fmt.Sprintf with no quoting or escaping at all, so a function or channel
// name containing a double or single quote could break out of its context
// and inject arbitrary SQL/DDL.
func buildNotifyFunctionSQL(function, channel string) string {
	quotedFunction := QuoteIdentifier(function)
	escapedChannel := strings.ReplaceAll(channel, "'", "''")

	return fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS trigger AS $$
			DECLARE
			payload text;
			channel text := '%s';
			
			BEGIN
			SELECT json_build_object('table', TG_TABLE_NAME, 'change', TG_OP, 'old', OLD, 'new', NEW)::text
			INTO payload;
			PERFORM pg_notify(channel, payload);
			IF (TG_OP = 'DELETE') THEN
				RETURN OLD;
		  	ELSE
				RETURN NEW;
		  	END IF;
		END; 
		$$
		LANGUAGE plpgsql;`, quotedFunction, escapedChannel)
}

// buildNotifyTriggerSQL builds the CREATE OR REPLACE TRIGGER statement that
// attaches the shared notify function to table. It returns the quoted
// trigger name (for error messages) alongside the query.
//
// table and function are quoted the same way as in buildNotifyFunctionSQL.
// The trigger name itself (table_function) is quoted as a single identifier
// rather than by quoting table and function separately and concatenating
// with "_": quoting them separately would let a table named
// `a", x); DROP TABLE y; --` still inject via the unquoted "_" join point,
// since QuoteIdentifier only guarantees safety for what's inside its own
// quotes, not for raw string concatenation around it. changesClause must
// already be validated by changesToString (or callers of this function must
// only pass literal "INSERT"/"UPDATE"/"DELETE" combinations) since it is
// spliced in unquoted -- it is not a string literal or identifier but a SQL
// keyword clause.
func buildNotifyTriggerSQL(table, function, changesClause string) (triggerName, query string) {
	quotedFunction := QuoteIdentifier(function)
	quotedTable := QuoteIdentifier(table)
	triggerName = QuoteIdentifier(table + "_" + function)

	query = fmt.Sprintf(`CREATE OR REPLACE TRIGGER %s
        AFTER %s
        ON %s
        FOR EACH ROW
        EXECUTE FUNCTION %s();`, triggerName, changesClause, quotedTable, quotedFunction)

	return triggerName, query
}

// ListenTable registers a function which notifies on the given "table" changes (INSERT, UPDATE, DELETE),
// the subscribed postgres channel is named 'table_change_notifications'.
//
// The callback function can return any other error to stop the listener.
// The callback function can return nil to continue listening.
//
// TableNotification's New and Old fields are raw json values, use the "json.Unmarshal" to decode them
// to the actual type.
func (db *DB) ListenTable(ctx context.Context, opts *ListenTableOptions, callback func(TableNotificationJSON, error) error) (Closer, error) {
	if err := db.PrepareListenTable(ctx, opts); err != nil {
		return nil, err
	}

	conn, err := db.Listen(ctx, opts.Channel)
	if err != nil {
		return nil, err
	}

	go func() {
		defer func() { _ = conn.Close(ctx) }()

		for {
			notification, err := conn.Accept(ctx)
			if err != nil && (errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed)) {
				return // may be produced by close.
			}

			if processNotification(notification, err, callback) {
				return
			}
		}
	}()

	return conn, nil
}

// processNotification decodes a single raw notification and invokes
// callback, reporting whether the listener loop should stop.
//
// This is split out of ListenTable's goroutine for two reasons:
//
//  1. Correctness: the original inline version dereferenced notification
//     (notification.Payload) unconditionally after handling a non-fatal
//     Accept error, but conn.Accept returns a nil *Notification whenever it
//     returns a non-nil error. Any transient error other than
//     io.ErrUnexpectedEOF/net.ErrClosed for which the callback chose to keep
//     listening (returned nil) crashed the goroutine with a nil-pointer
//     dereference. This version never touches notification when acceptErr
//     is non-nil.
//  2. Testability: Listener wraps a concrete *pgxpool.Conn with no seam for
//     injecting a fake connection, so the notification-handling control flow
//     could not otherwise be unit tested without a live database.
func processNotification(notification *Notification, acceptErr error, callback func(TableNotificationJSON, error) error) (stop bool) {
	var evt TableNotificationJSON

	if acceptErr != nil {
		return callback(evt, acceptErr) != nil
	}

	// make payload available for debugging on errors.
	evt.payload = notification.Payload

	if err := json.Unmarshal([]byte(notification.Payload), &evt); err != nil {
		return callback(evt, err) != nil
	}

	return callback(evt, nil) != nil
}
