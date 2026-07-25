package mirage

import (
	"testing"
)

func TestRepository_NewRepository(t *testing.T) {
	type Widget struct {
		ID   int64  `db:"pk,type:bigserial"`
		Name string `db:"type:text"`
	}

	db := &DB{searchPath: "public"}
	repo := NewRepository[Widget](db)

	if repo.table == nil {
		t.Fatal("expected table to be set")
	}
	if repo.table.Name != "widgets" {
		t.Errorf("expected table name 'widgets', got %q", repo.table.Name)
	}
}

func TestRepository_WithRetry(t *testing.T) {
	type Item struct {
		ID   int64 `db:"pk,type:bigserial"`
		Name string `db:"type:text"`
	}

	db := &DB{searchPath: "public"}
	opts := RetryOptions{MaxAttempts: 5}
	repo := NewRepository[Item](db, WithRetry(opts))

	if !repo.retryEnabled {
		t.Error("expected retryEnabled=true")
	}
	if repo.retry.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts=5, got %d", repo.retry.MaxAttempts)
	}
}

func TestRepository_InRetryTransaction(t *testing.T) {
	type Item struct {
		ID   int64 `db:"pk,type:bigserial"`
		Name string `db:"type:text"`
	}

	db := &DB{searchPath: "public"}

	repoNoRetry := NewRepository[Item](db)
	if repoNoRetry.retryEnabled {
		t.Error("expected retryEnabled=false when retry not configured")
	}

	repoWithRetry := NewRepository[Item](db, WithRetry(RetryOptions{}))
	if !repoWithRetry.retryEnabled {
		t.Error("expected retryEnabled=true when retry configured")
	}
}

func TestRepository_TableFromRegistry(t *testing.T) {
	type CustomTable struct {
		ID int64 `db:"pk,type:bigserial"`
	}

	db := &DB{searchPath: "public"}
	repo := NewRepository[CustomTable](db)

	if repo.table == nil {
		t.Fatal("expected table to be set")
	}
	if repo.table.Name != "custom_tables" {
		t.Errorf("expected table name 'custom_tables', got %q", repo.table.Name)
	}
}
