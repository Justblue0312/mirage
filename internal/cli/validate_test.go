package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeModelFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	full := filepath.Join(dir, filename)
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("writing fixture model %s: %v", filename, err)
	}
}

const validUserModel = `package models

type User struct {
	_ struct{} ` + "`db:\"name=users,comment=Users table\"`" + `

	ID    int64  ` + "`db:\"pk,identity,type=bigserial\"`" + `
	Email string ` + "`db:\"name=email,type=varchar(255),notnull,unique\"`" + `
}
`

// ValidateNotNullNoDefault (internal/validate) only operates on diff
// column-addition events and is wired into `generate`, not `validate` --
// `mirage validate` runs the structural rule set (internal/validate.Validate)
// against a standalone scan, which has no concept of "newly added column"
// to evaluate. This fixture's NOT-NULL-without-default field is therefore
// invisible to `validate` today; the test below locks in that it still
// passes cleanly (no false positive), and doubles as documentation that
// `mirage validate`'s promise to catch issues "before running generate"
// does not currently cover this specific, generate-time-only check.
const warnOnlyModel = `package models

type Order struct {
	_ struct{} ` + "`db:\"name=orders\"`" + `

	ID       int64 ` + "`db:\"pk,identity,type=bigserial\"`" + `
	TenantID int64 ` + "`db:\"name=tenant_id,type=bigint,notnull\"`" + `
}
`

// Two tables mapping to the same SQL name is a hard validation error
// (ErrDuplicateTableName), used to confirm `validate` exits non-zero and
// prints the failure.
const duplicateTableModel = `package models

type Account struct {
	_ struct{} ` + "`db:\"name=accounts\"`" + `
	ID int64 ` + "`db:\"pk,identity,type=bigserial\"`" + `
}

type LegacyAccount struct {
	_ struct{} ` + "`db:\"name=accounts\"`" + `
	ID int64 ` + "`db:\"pk,identity,type=bigserial\"`" + `
}
`

func TestCmdValidate_ValidModel(t *testing.T) {
	dir := t.TempDir()
	writeModelFile(t, dir, "user.go", validUserModel)

	if err := runCLI(t, "validate", "--source", dir); err != nil {
		t.Fatalf("expected validate to pass for a valid model, got: %v", err)
	}
}

func TestCmdValidate_DoesNotCatchGenerateOnlyChecks(t *testing.T) {
	dir := t.TempDir()
	writeModelFile(t, dir, "order.go", warnOnlyModel)

	if err := runCLI(t, "validate", "--source", dir); err != nil {
		t.Fatalf("validate should pass here since ValidateNotNullNoDefault is not wired into it, got: %v", err)
	}
}

func TestCmdValidate_HardErrorFailsTheCommand(t *testing.T) {
	dir := t.TempDir()
	writeModelFile(t, dir, "account.go", duplicateTableModel)

	if err := runCLI(t, "validate", "--source", dir); err == nil {
		t.Fatal("expected validate to fail for two structs mapping to the same table name, got nil error")
	}
}

func TestCmdValidate_RequiresSourceFlag(t *testing.T) {
	if err := runCLI(t, "validate"); err == nil {
		t.Fatal("expected an error when --source is omitted (it is marked Required in the flag definition)")
	}
}

func TestCmdValidate_NonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if err := runCLI(t, "validate", "--source", dir); err == nil {
		t.Fatal("expected an error scanning a nonexistent directory, got nil")
	}
}
