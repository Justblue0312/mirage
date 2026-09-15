package mirage

import (
	"errors"
	"strings"
	"testing"
)

// TestProcessNotification_AcceptErrorDoesNotPanic guards against the crash
// found during review: the original inline implementation in ListenTable's
// goroutine dereferenced notification.Payload unconditionally after handling
// a non-fatal Accept() error, but conn.Accept always returns a nil
// *Notification alongside a non-nil error. Any transient error (context
// deadline, transient network hiccup, etc.) other than io.ErrUnexpectedEOF/
// net.ErrClosed, for which the callback chose to keep listening by
// returning nil, crashed the listener goroutine with a nil-pointer
// dereference. This test calls processNotification directly with a nil
// notification and a non-nil error, exactly as conn.Accept produces it.
func TestProcessNotification_AcceptErrorDoesNotPanic(t *testing.T) {
	boom := errors.New("transient accept error")

	var gotErr error
	var callbackCalls int
	callback := func(_ TableNotificationJSON, err error) error {
		callbackCalls++
		gotErr = err
		return nil // caller chooses to keep listening
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("processNotification panicked with a nil notification: %v", r)
		}
	}()

	stop := processNotification(nil /* notification */, boom, callback)

	if stop {
		t.Fatal("expected stop=false when callback returns nil (keep listening)")
	}
	if callbackCalls != 1 {
		t.Fatalf("expected callback to be invoked exactly once, got %d", callbackCalls)
	}
	if !errors.Is(gotErr, boom) {
		t.Fatalf("expected callback to receive the accept error, got %v", gotErr)
	}
}

// TestProcessNotification_AcceptErrorCallbackStops verifies the loop stops
// when the callback decides to give up on a non-fatal accept error.
func TestProcessNotification_AcceptErrorCallbackStops(t *testing.T) {
	boom := errors.New("fatal to caller")
	callback := func(_ TableNotificationJSON, err error) error { return err }

	stop := processNotification(nil, boom, callback)
	if !stop {
		t.Fatal("expected stop=true when callback returns a non-nil error")
	}
}

// TestProcessNotification_DecodesPayload verifies the success path still
// decodes JSON payloads into the notification event and surfaces the raw
// payload via GetPayload().
func TestProcessNotification_DecodesPayload(t *testing.T) {
	n := &Notification{Payload: `{"table":"users","change":"INSERT","new":{"id":1}}`}

	var got TableNotificationJSON
	var gotErr error
	callback := func(evt TableNotificationJSON, err error) error {
		got = evt
		gotErr = err
		return nil
	}

	if stop := processNotification(n, nil, callback); stop {
		t.Fatal("expected stop=false on successful decode")
	}
	if gotErr != nil {
		t.Fatalf("expected no error, got %v", gotErr)
	}
	if got.Table != "users" || got.Change != TableChangeTypeInsert {
		t.Fatalf("unexpected decoded event: %+v", got)
	}
	if got.GetPayload() != n.Payload {
		t.Fatalf("expected GetPayload() to return the raw payload, got %q", got.GetPayload())
	}
}

// TestProcessNotification_InvalidJSONReportsError verifies malformed
// payloads are reported to the callback (with the raw payload still
// attached for debugging) instead of panicking or being silently dropped.
func TestProcessNotification_InvalidJSONReportsError(t *testing.T) {
	n := &Notification{Payload: `not json`}

	var gotErr error
	var gotPayload string
	callback := func(evt TableNotificationJSON, err error) error {
		gotErr = err
		gotPayload = evt.GetPayload()
		return nil
	}

	processNotification(n, nil, callback)
	if gotErr == nil {
		t.Fatal("expected a JSON decode error to be reported")
	}
	if gotPayload != "not json" {
		t.Fatalf("expected raw payload to be preserved for debugging, got %q", gotPayload)
	}
}

// TestChangesToString_RejectsArbitraryValues guards against the SQL
// injection found during review: TableChangeType is a plain string type,
// not a closed Go enum, so nothing at the type level stops a caller from
// constructing an arbitrary TableChangeType("..."). Because the result of
// changesToString is spliced directly into a CREATE TRIGGER statement's
// AFTER clause (see prepareListenTable), accepting arbitrary values would
// let a malicious/buggy caller inject SQL through the "changes" option.
func TestChangesToString_RejectsArbitraryValues(t *testing.T) {
	_, err := changesToString([]TableChangeType{TableChangeTypeInsert, TableChangeType("TRUNCATE); DROP TABLE users; --")})
	if err == nil {
		t.Fatal("expected an error for an unsupported/malicious TableChangeType value")
	}
}

func TestChangesToString_KnownValues(t *testing.T) {
	got, err := changesToString([]TableChangeType{TableChangeTypeInsert, TableChangeTypeUpdate, TableChangeTypeDelete})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "INSERT OR UPDATE OR DELETE"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestChangesToString_Empty(t *testing.T) {
	got, err := changesToString(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string for no changes, got %q", got)
	}
}

// TestBuildNotifyFunctionSQL_QuotesFunctionIdentifier guards against the SQL
// injection found during review: function/channel were previously
// interpolated into the CREATE FUNCTION statement with a bare fmt.Sprintf
// and no quoting at all. A function name containing a double quote could
// break out of the identifier position and inject arbitrary SQL that would
// run with the privileges of the connection preparing the listener.
func TestBuildNotifyFunctionSQL_QuotesFunctionIdentifier(t *testing.T) {
	malicious := `notify_fn"; DROP TABLE users; --`
	query := buildNotifyFunctionSQL(malicious, "chan")

	if strings.Contains(query, `CREATE OR REPLACE FUNCTION `+malicious+`(`) {
		t.Fatalf("function identifier was not quoted, injection possible:\n%s", query)
	}
	// pgx's Identifier.Sanitize doubles embedded double quotes and wraps in
	// quotes, so the malicious quote must appear escaped, not bare.
	if !strings.Contains(query, `"notify_fn""; DROP TABLE users; --"`) {
		t.Fatalf("expected the identifier to be safely quoted/escaped, got:\n%s", query)
	}
}

// TestBuildNotifyFunctionSQL_EscapesChannelLiteral guards against the
// second injection point in the same statement: channel is embedded inside
// a single-quoted PL/pgSQL string literal, so a channel name containing a
// single quote could previously close the literal early and inject SQL
// into the trigger function body.
func TestBuildNotifyFunctionSQL_EscapesChannelLiteral(t *testing.T) {
	malicious := `chan'; PERFORM pg_sleep(100); --`
	query := buildNotifyFunctionSQL("notify_fn", malicious)

	if strings.Contains(query, `channel text := '`+malicious+`';`) {
		t.Fatalf("channel string literal was not escaped, injection possible:\n%s", query)
	}
	if !strings.Contains(query, `channel text := 'chan''; PERFORM pg_sleep(100); --';`) {
		t.Fatalf("expected embedded single quote to be doubled, got:\n%s", query)
	}
}

// TestBuildNotifyTriggerSQL_QuotesIdentifiers guards against the same class
// of injection in the CREATE TRIGGER statement: table and function are now
// quoted, and the combined trigger name is quoted as a single identifier
// rather than built by string-concatenating two separately-quoted pieces
// (which would leave the "_" join point unprotected).
func TestBuildNotifyTriggerSQL_QuotesIdentifiers(t *testing.T) {
	maliciousTable := `orders"; DROP TABLE secrets; --`
	name, query := buildNotifyTriggerSQL(maliciousTable, "notify_fn", "INSERT OR UPDATE OR DELETE")

	// The unquoted, executable form must never appear in the query.
	if strings.Contains(query, `ON `+maliciousTable+"\n") || strings.Contains(query, "ON "+maliciousTable+" ") {
		t.Fatalf("table identifier was not quoted, injection possible:\n%s", query)
	}
	if !strings.HasPrefix(name, `"`) || !strings.HasSuffix(name, `"`) {
		t.Fatalf("expected trigger name to be a single quoted identifier, got %q", name)
	}
	// pgx's Identifier.Sanitize doubles embedded double quotes and wraps the
	// whole (table + "_" + function) string in one pair of quotes, so the
	// malicious text must only ever appear escaped inside that single
	// identifier -- never as a second, bare SQL statement.
	wantTriggerName := `"orders""; DROP TABLE secrets; --_notify_fn"`
	if name != wantTriggerName {
		t.Fatalf("got trigger name %q, want %q", name, wantTriggerName)
	}
	if !strings.Contains(query, "CREATE OR REPLACE TRIGGER "+wantTriggerName) {
		t.Fatalf("expected query to use the fully-quoted trigger name, got:\n%s", query)
	}
}

// TestPrepareListenTable_ValidatesChangesBeforeBuildingSQL is a focused unit
// test (no DB required) confirming that the same validation
// changesToString performs runs before any query is built, ensuring
// PrepareListenTable/ListenTable never construct a CREATE TRIGGER
// statement containing an attacker-controlled AFTER clause.
func TestPrepareListenTable_ValidatesChangesBeforeBuildingSQL(t *testing.T) {
	_, err := changesToString([]TableChangeType{"INSERT'); --"})
	if err == nil {
		t.Fatal("expected malicious change type to be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported table change type") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
