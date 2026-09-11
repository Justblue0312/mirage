package schema

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// Function represents a PostgreSQL CREATE FUNCTION definition.
type Function struct {
	SearchPath  string
	Name        string
	Description string
	Language    string
	Arguments   []FunctionArgument
	ReturnType  string
	Volatility  string
	Security    string
	Body        string
}

// SQLName returns the fully-qualified function name.
func (f *Function) SQLName() string {
	if f.SearchPath == "" || f.SearchPath == GetDefaultSearchPath() {
		return f.Name
	}
	return f.SearchPath + "." + f.Name
}

// HashBody returns a SHA-256 hash of the function definition for change detection.
func (f *Function) HashBody() string {
	const sep = "\x00"
	h := sha256.New()
	h.Write([]byte(f.Body))
	h.Write([]byte(sep))
	h.Write([]byte(f.Language))
	h.Write([]byte(sep))
	h.Write([]byte(f.ReturnType))
	h.Write([]byte(sep))
	h.Write([]byte(f.Volatility))
	h.Write([]byte(sep))
	h.Write([]byte(f.Security))
	h.Write([]byte(sep))
	for _, arg := range f.Arguments {
		h.Write([]byte(arg.Name))
		h.Write([]byte(sep))
		h.Write([]byte(arg.Type))
		h.Write([]byte(sep))
		h.Write([]byte(arg.Mode))
		h.Write([]byte(sep))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// FunctionArgument represents a single argument to a database function.
type FunctionArgument struct {
	Name string
	Type string
	Mode string // IN, OUT, INOUT, VARIADIC (default: IN)
}

// View represents a PostgreSQL CREATE VIEW definition.
type View struct {
	SearchPath  string
	Name        string
	Description string
	Query       string
}

// SQLName returns the fully-qualified view name.
func (v *View) SQLName() string {
	if v.SearchPath == "" || v.SearchPath == GetDefaultSearchPath() {
		return v.Name
	}
	return v.SearchPath + "." + v.Name
}

// MaterializedView represents a PostgreSQL CREATE MATERIALIZED VIEW definition.
type MaterializedView struct {
	SearchPath  string
	Name        string
	Description string
	Query       string
}

// SQLName returns the fully-qualified materialized view name.
func (mv *MaterializedView) SQLName() string {
	if mv.SearchPath == "" || mv.SearchPath == GetDefaultSearchPath() {
		return mv.Name
	}
	return mv.SearchPath + "." + mv.Name
}

// Trigger represents a PostgreSQL CREATE TRIGGER definition.
type Trigger struct {
	SearchPath  string
	Name        string
	Description string
	Table       string
	Timing      string   // BEFORE, AFTER, INSTEAD OF
	Events      []string // INSERT, UPDATE, DELETE, TRUNCATE
	Function    string
	Constraint  string // optional constraint expression
}

// SQLName returns the fully-qualified trigger name.
func (t *Trigger) SQLName() string {
	if t.SearchPath == "" || t.SearchPath == GetDefaultSearchPath() {
		return t.Name
	}
	return t.SearchPath + "." + t.Name
}

// HashBody returns a hash of the trigger definition for change detection.
func (t *Trigger) HashBody() string {
	const sep = "\x00"
	h := sha256.New()
	h.Write([]byte(t.Table))
	h.Write([]byte(sep))
	h.Write([]byte(t.Timing))
	h.Write([]byte(sep))
	h.Write([]byte(t.Function))
	h.Write([]byte(sep))
	h.Write([]byte(t.Constraint))
	h.Write([]byte(sep))
	events := make([]string, len(t.Events))
	copy(events, t.Events)
	sort.Strings(events)
	h.Write([]byte(strings.Join(events, ",")))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Procedure represents a PostgreSQL CREATE PROCEDURE definition.
type Procedure struct {
	SearchPath  string
	Name        string
	Description string
	Language    string
	Arguments   []ProcedureArgument
	Body        string
}

// SQLName returns the fully-qualified procedure name.
func (p *Procedure) SQLName() string {
	if p.SearchPath == "" || p.SearchPath == GetDefaultSearchPath() {
		return p.Name
	}
	return p.SearchPath + "." + p.Name
}

// HashBody returns a SHA-256 hash of the procedure definition for change detection.
func (p *Procedure) HashBody() string {
	const sep = "\x00"
	h := sha256.New()
	h.Write([]byte(p.Body))
	h.Write([]byte(sep))
	h.Write([]byte(p.Language))
	h.Write([]byte(sep))
	for _, arg := range p.Arguments {
		h.Write([]byte(arg.Name))
		h.Write([]byte(sep))
		h.Write([]byte(arg.Type))
		h.Write([]byte(sep))
		h.Write([]byte(arg.Mode))
		h.Write([]byte(sep))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ProcedureArgument represents a single argument to a database procedure.
type ProcedureArgument struct {
	Name string
	Type string
	Mode string // IN, OUT, INOUT
}

// Grant represents a GRANT privilege statement.
type Grant struct {
	SearchPath string
	ObjectType string // table, view, sequence, function, procedure, schema
	ObjectName string
	Privileges []string // SELECT, INSERT, UPDATE, DELETE, etc.
	Roles      []string
}

// SortKey returns a deterministic identity key for the grant. It matches the
// grant identity used when diffing (object type + object name + sorted roles)
// so that scanner output ordering is stable and diff-safe.
func (g Grant) SortKey() string {
	roles := make([]string, len(g.Roles))
	copy(roles, g.Roles)
	sort.Strings(roles)
	// Use a NUL separator so that names containing ':' cannot collide.
	return g.ObjectType + "\x00" + g.ObjectName + "\x00" + strings.Join(roles, ",")
}

// Policy represents a row-level security policy.
type Policy struct {
	SearchPath string
	Name       string
	Table      string
	Command    string // ALL, SELECT, INSERT, UPDATE, DELETE
	Roles      []string
	Using      string // USING expression
	Check      string // WITH CHECK expression (defaults to Using if empty)
	Permissive string // PERMISSIVE or RESTRICTIVE (default: PERMISSIVE)
}

// SortKey returns a deterministic identity key for the policy (table + name),
// matching the policy identity used when diffing.
func (p Policy) SortKey() string {
	return p.Table + "\x00" + p.Name
}

// Extension represents a PostgreSQL CREATE EXTENSION definition.
type Extension struct {
	SearchPath  string
	Name        string
	Schema      string // optional: SCHEMA clause
	Version     string // optional: VERSION clause
	IfNotExists bool
	Cascade     bool // for DROP EXTENSION ... CASCADE
}

// SQLName returns the fully-qualified extension name (schema.name or just name).
func (e *Extension) SQLName() string {
	if e.SearchPath == "" || e.SearchPath == GetDefaultSearchPath() {
		return e.Name
	}
	return e.SearchPath + "." + e.Name
}

// HashBody returns a hash of the extension definition for change detection.
func (e *Extension) HashBody() string {
	const sep = "\x00"
	h := sha256.New()
	h.Write([]byte(e.Name))
	h.Write([]byte(sep))
	h.Write([]byte(e.Schema))
	h.Write([]byte(sep))
	h.Write([]byte(e.Version))
	return fmt.Sprintf("%x", h.Sum(nil))
}
