package mirage

import (
	"testing"
)

func TestRegisterAndGetFunctions(t *testing.T) {
	registry.mu.Lock()
	registry.functions = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register(
		Function{Name: "fn1", Language: "plpgsql", Body: "BEGIN END;", ReturnType: "void"},
		Function{Name: "fn2", Language: "sql", Body: "SELECT 1", ReturnType: "integer"},
	)

	Register(
		Function{Name: "fn1", Language: "plpgsql", Body: "BEGIN END;", ReturnType: "void"},
		Function{Name: "fn2", Language: "sql", Body: "SELECT 1", ReturnType: "integer"},
	)

	funcs := GetRegisteredFunctions()
	if len(funcs) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(funcs))
	}
	if funcs[0].Name != "fn1" {
		t.Errorf("expected fn1, got %s", funcs[0].Name)
	}
	if funcs[1].Name != "fn2" {
		t.Errorf("expected fn2, got %s", funcs[1].Name)
	}
}

func TestRegisterAndGetViews(t *testing.T) {
	registry.mu.Lock()
	registry.views = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register(View{Name: "v_active", Query: "SELECT * FROM users WHERE active"})

	views := GetRegisteredViews()
	if len(views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(views))
	}
	if views[0].Name != "v_active" {
		t.Errorf("expected v_active, got %s", views[0].Name)
	}
}

func TestRegisterAndGetMaterializedViews(t *testing.T) {
	registry.mu.Lock()
	registry.materializedViews = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register(MaterializedView{Name: "mv_stats", Query: "SELECT count(*) FROM users"})

	mvs := GetRegisteredMaterializedViews()
	if len(mvs) != 1 {
		t.Fatalf("expected 1 mat view, got %d", len(mvs))
	}
	if mvs[0].Name != "mv_stats" {
		t.Errorf("expected mv_stats, got %s", mvs[0].Name)
	}
}

func TestRegisterAndGetTriggers(t *testing.T) {
	registry.mu.Lock()
	registry.triggers = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register(Trigger{Name: "trg_upd", Table: "users", Timing: "BEFORE", Events: []string{"UPDATE"}, Function: "fn1"})

	triggers := GetRegisteredTriggers()
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	if triggers[0].Name != "trg_upd" {
		t.Errorf("expected trg_upd, got %s", triggers[0].Name)
	}
}

func TestRegisterAndGetProcedures(t *testing.T) {
	registry.mu.Lock()
	registry.procedures = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register(Procedure{Name: "refresh", Language: "plpgsql", Body: "BEGIN REFRESH MATERIALIZED VIEW mv_stats; END;"})

	procs := GetRegisteredProcedures()
	if len(procs) != 1 {
		t.Fatalf("expected 1 procedure, got %d", len(procs))
	}
	if procs[0].Name != "refresh" {
		t.Errorf("expected refresh, got %s", procs[0].Name)
	}
}

func TestRegisterAndGetGrants(t *testing.T) {
	registry.mu.Lock()
	registry.grants = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register(Grant{ObjectType: "table", ObjectName: "users", Privileges: []string{"SELECT"}, Roles: []string{"app_user"}})

	grants := GetRegisteredGrants()
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(grants))
	}
	if grants[0].ObjectName != "users" {
		t.Errorf("expected users, got %s", grants[0].ObjectName)
	}
}

func TestRegisterAndGetPolicies(t *testing.T) {
	registry.mu.Lock()
	registry.policies = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register(Policy{Name: "user_isolation", Table: "users", Command: "ALL", Using: "auth.uid() = id"})

	policies := GetRegisteredPolicies()
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}
	if policies[0].Name != "user_isolation" {
		t.Errorf("expected user_isolation, got %s", policies[0].Name)
	}
}

func TestRegisterMultipleTypes(t *testing.T) {
	registry.mu.Lock()
	registry.functions = nil
	registry.views = nil
	registry.triggers = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register(
		Function{Name: "fn1", Language: "plpgsql", Body: "BEGIN END;", ReturnType: "void"},
		View{Name: "v1", Query: "SELECT 1"},
		Trigger{Name: "t1", Table: "x", Timing: "BEFORE", Events: []string{"INSERT"}, Function: "fn1"},
	)

	if len(GetRegisteredFunctions()) != 1 {
		t.Errorf("expected 1 function")
	}
	if len(GetRegisteredViews()) != 1 {
		t.Errorf("expected 1 view")
	}
	if len(GetRegisteredTriggers()) != 1 {
		t.Errorf("expected 1 trigger")
	}
}

func TestRegisterIgnoresUnknownTypes(t *testing.T) {
	registry.mu.Lock()
	registry.functions = nil
	registry.mu.Unlock()
	defer ResetRegistry()

	Register("not a function", 42)

	funcs := GetRegisteredFunctions()
	if len(funcs) != 0 {
		t.Errorf("expected 0 functions, got %d", len(funcs))
	}
}

func TestGetReturnsCopy(t *testing.T) {
	registry.mu.Lock()
	registry.functions = []Function{{Name: "fn1"}}
	registry.mu.Unlock()
	defer ResetRegistry()

	funcs := GetRegisteredFunctions()
	funcs[0].Name = "modified"

	original := GetRegisteredFunctions()
	if original[0].Name != "fn1" {
		t.Errorf("expected original to be unchanged, got %s", original[0].Name)
	}
}

func TestRegisterRejectsUnsupportedType(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	err := Register(struct{ X int }{X: 1})
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
}

func TestRegistry_InstancesAreIsolated(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	priv := NewRegistry()
	if err := priv.Register(Function{Name: "only_on_private"}); err != nil {
		t.Fatal(err)
	}

	if got := priv.Functions(); len(got) != 1 {
		t.Fatalf("private registry should see its own registration, got %d", len(got))
	}
	if got := GetRegisteredFunctions(); len(got) != 0 {
		t.Fatalf("private registry must not leak into the global registry, got %d", len(got))
	}

	if err := Register(Function{Name: "only_on_global"}); err != nil {
		t.Fatal(err)
	}
	if got := priv.Functions(); len(got) != 1 {
		t.Fatalf("global registration must not leak into private instance, got %d", len(got))
	}
}

func TestRegisterAndGetExtensions(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	Register(Extension{Name: "uuid-ossp", Schema: "public", Version: "1.1"})

	exts := GetRegisteredExtensions()
	if len(exts) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(exts))
	}
	if exts[0].Name != "uuid-ossp" {
		t.Errorf("expected uuid-ossp, got %s", exts[0].Name)
	}
	if exts[0].Version != "1.1" {
		t.Errorf("expected 1.1, got %s", exts[0].Version)
	}
}

func TestResetRegistryClearsAll(t *testing.T) {
	Register(Function{Name: "fn1"})
	if len(GetRegisteredFunctions()) != 1 {
		t.Fatal("expected 1 function before reset")
	}
	ResetRegistry()
	if len(GetRegisteredFunctions()) != 0 {
		t.Fatalf("expected 0 functions after reset, got %d", len(GetRegisteredFunctions()))
	}
}
