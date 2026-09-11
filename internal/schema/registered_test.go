package schema

import "testing"

func TestFunctionHashBody_DelimiterPreventsCollision(t *testing.T) {
	// Two functions whose field values, when concatenated WITHOUT a delimiter,
	// would collide ("abc"+"def" == "abcd"+"ef"). With a delimiter they must
	// produce different hashes.
	a := Function{Body: "abc", Language: "def"}
	b := Function{Body: "abcd", Language: "ef"}
	if a.HashBody() == b.HashBody() {
		t.Fatalf("HashBody collided for %q+%q vs %q+%q", a.Body, a.Language, b.Body, b.Language)
	}
}

func TestFunctionHashBody_ContentSensitive(t *testing.T) {
	base := Function{Body: "x", Language: "sql", ReturnType: "int", Volatility: "VOLATILE", Security: "DEFINER"}
	changed := base
	changed.Body = "y"
	if base.HashBody() == changed.HashBody() {
		t.Fatalf("HashBody should differ when Body changes")
	}
}

func TestTriggerHashBody_SortsEvents(t *testing.T) {
	a := Trigger{Table: "t", Timing: "BEFORE", Events: []string{"INSERT", "UPDATE"}, Function: "fn"}
	b := Trigger{Table: "t", Timing: "BEFORE", Events: []string{"UPDATE", "INSERT"}, Function: "fn"}
	if a.HashBody() != b.HashBody() {
		t.Fatalf("trigger event ordering should not affect hash")
	}
}

func TestProcedureHashBody_DelimiterPreventsCollision(t *testing.T) {
	a := Procedure{Body: "abc", Language: "def"}
	b := Procedure{Body: "abcd", Language: "ef"}
	if a.HashBody() == b.HashBody() {
		t.Fatalf("Procedure.HashBody collided")
	}
}

func TestFunctionSQLName_SchemaPrefix(t *testing.T) {
	f := Function{Name: "do_thing", SearchPath: "app"}
	if got := f.SQLName(); got != "app.do_thing" {
		t.Fatalf("SQLName = %q, want app.do_thing", got)
	}
	f2 := Function{Name: "do_thing", SearchPath: "public"}
	if got := f2.SQLName(); got != "do_thing" {
		t.Fatalf("SQLName = %q, want do_thing (public omitted)", got)
	}
}
