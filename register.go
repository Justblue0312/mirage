package mirage

import (
	"fmt"
	"sync"

	schemapkg "github.com/justblue/mirage/internal/schema"
)

type Function = schemapkg.Function
type FunctionArgument = schemapkg.FunctionArgument
type View = schemapkg.View
type MaterializedView = schemapkg.MaterializedView
type Trigger = schemapkg.Trigger
type Procedure = schemapkg.Procedure
type ProcedureArgument = schemapkg.ProcedureArgument
type Grant = schemapkg.Grant
type Policy = schemapkg.Policy
type Extension = schemapkg.Extension

// Registry holds a set of database objects (functions, views, triggers, etc.)
// registered programmatically. Each Registry instance is independent, which
// makes it safe to use for multi-schema/multi-tenant setups and gives tests a
// way to obtain an isolated registry instead of stomping on process-global
// state.
type Registry struct {
	mu                sync.Mutex
	extensions        []Extension
	functions         []Function
	views             []View
	materializedViews []MaterializedView
	triggers          []Trigger
	procedures        []Procedure
	grants            []Grant
	policies          []Policy
}

// NewRegistry returns a fresh, empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// registry is the process-global default Registry. The package-level Register
// and Get* functions operate on it, preserving the original API for callers
// that don't need instance isolation.
var registry = NewRegistry()

// Register stores one or more database objects in the registry.
//
// Register returns nil so it can be used in var declarations:
//
//	var _ = reg.Register(mirage.Function{...})
func (r *Registry) Register(items ...any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, it := range items {
		switch v := it.(type) {
		case Function:
			// Deduplicate by Name. If you need stronger uniqueness, build key from Name+Language+ReturnType+Body.
			exists := false
			for _, f := range r.functions {
				if f.Name == v.Name {
					exists = true
					break
				}
			}
			if !exists {
				r.functions = append(r.functions, v)
			}
		case View:
			exists := false
			for _, vv := range r.views {
				if vv.Name == v.Name {
					exists = true
					break
				}
			}
			if !exists {
				r.views = append(r.views, v)
			}
		case MaterializedView:
			exists := false
			for _, m := range r.materializedViews {
				if m.Name == v.Name {
					exists = true
					break
				}
			}
			if !exists {
				r.materializedViews = append(r.materializedViews, v)
			}
		case Trigger:
			exists := false
			for _, t := range r.triggers {
				if t.Name == v.Name {
					exists = true
					break
				}
			}
			if !exists {
				r.triggers = append(r.triggers, v)
			}
		case Procedure:
			exists := false
			for _, p := range r.procedures {
				if p.Name == v.Name {
					exists = true
					break
				}
			}
			if !exists {
				r.procedures = append(r.procedures, v)
			}
		case Grant:
			// Same identity a Grant is deduplicated/matched by elsewhere
			// (internal/diff and the scanner's post-scan sort both key
			// Grants by SortKey()), so Register uses it too rather than
			// appending unconditionally.
			exists := false
			for _, g := range r.grants {
				if g.SortKey() == v.SortKey() {
					exists = true
					break
				}
			}
			if !exists {
				r.grants = append(r.grants, v)
			}
		case Policy:
			exists := false
			for _, p := range r.policies {
				if p.Name == v.Name {
					exists = true
					break
				}
			}
			if !exists {
				r.policies = append(r.policies, v)
			}
		case Extension:
			exists := false
			for _, e := range r.extensions {
				if e.Name == v.Name {
					exists = true
					break
				}
			}
			if !exists {
				r.extensions = append(r.extensions, v)
			}
		default:
			return fmt.Errorf("register: unsupported type %T; expected one of Function, View, MaterializedView, Trigger, Procedure, Grant, Policy, Extension", it)
		}
	}

	return nil
}

// Functions returns a copy of all registered functions.
func (r *Registry) Functions() []Function {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Function, len(r.functions))
	copy(result, r.functions)
	return result
}

// Views returns a copy of all registered views.
func (r *Registry) Views() []View {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]View, len(r.views))
	copy(result, r.views)
	return result
}

// MaterializedViews returns a copy of all registered materialized views.
func (r *Registry) MaterializedViews() []MaterializedView {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]MaterializedView, len(r.materializedViews))
	copy(result, r.materializedViews)
	return result
}

// Triggers returns a copy of all registered triggers.
func (r *Registry) Triggers() []Trigger {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Trigger, len(r.triggers))
	copy(result, r.triggers)
	return result
}

// Procedures returns a copy of all registered procedures.
func (r *Registry) Procedures() []Procedure {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Procedure, len(r.procedures))
	copy(result, r.procedures)
	return result
}

// Grants returns a copy of all registered grants.
func (r *Registry) Grants() []Grant {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Grant, len(r.grants))
	copy(result, r.grants)
	return result
}

// Policies returns a copy of all registered policies.
func (r *Registry) Policies() []Policy {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Policy, len(r.policies))
	copy(result, r.policies)
	return result
}

// Extensions returns a copy of all registered extensions.
func (r *Registry) Extensions() []Extension {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Extension, len(r.extensions))
	copy(result, r.extensions)
	return result
}

// Reset clears all registered objects.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extensions = nil
	r.functions = nil
	r.views = nil
	r.materializedViews = nil
	r.triggers = nil
	r.procedures = nil
	r.grants = nil
	r.policies = nil
}

// Register stores one or more database objects in the process-global registry.
// At scan time, the scanner detects these calls via AST and populates
// schema.Package. At runtime, Register populates the global registry for
// programmatic access.
//
// Register returns nil so it can be used in var declarations:
//
//	var _ = mirage.Register(mirage.Function{...})
func Register(objects ...any) error { return registry.Register(objects...) }

// GetRegisteredFunctions returns all functions registered on the global registry.
func GetRegisteredFunctions() []Function { return registry.Functions() }

// GetRegisteredViews returns all views registered on the global registry.
func GetRegisteredViews() []View { return registry.Views() }

// GetRegisteredMaterializedViews returns all materialized views registered on the global registry.
func GetRegisteredMaterializedViews() []MaterializedView { return registry.MaterializedViews() }

// GetRegisteredTriggers returns all triggers registered on the global registry.
func GetRegisteredTriggers() []Trigger { return registry.Triggers() }

// GetRegisteredProcedures returns all procedures registered on the global registry.
func GetRegisteredProcedures() []Procedure { return registry.Procedures() }

// GetRegisteredGrants returns all grants registered on the global registry.
func GetRegisteredGrants() []Grant { return registry.Grants() }

// GetRegisteredPolicies returns all policies registered on the global registry.
func GetRegisteredPolicies() []Policy { return registry.Policies() }

// GetRegisteredExtensions returns all extensions registered on the global registry.
func GetRegisteredExtensions() []Extension { return registry.Extensions() }

// ResetRegistry clears all objects on the global registry. Intended for use in
// tests to guarantee isolation between cases that share the process-global
// registry.
func ResetRegistry() { registry.Reset() }
