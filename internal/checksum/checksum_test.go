package checksum

import (
	"testing"
)

func TestCompute_Deterministic(t *testing.T) {
	stmts := []string{"CREATE TABLE users (id bigserial PRIMARY KEY);"}
	a := Compute(stmts)
	b := Compute(stmts)
	if a != b {
		t.Errorf("non-deterministic: %s != %s", a, b)
	}
}

func TestCompute_DifferentInputs(t *testing.T) {
	a := Compute([]string{"CREATE TABLE users (id bigserial PRIMARY KEY);"})
	b := Compute([]string{"CREATE TABLE users (id int PRIMARY KEY);"})
	if a == b {
		t.Error("expected different checksums for different inputs")
	}
}

func TestCompute_Prefix(t *testing.T) {
	result := Compute([]string{"SELECT 1;"})
	if len(result) < 7 || result[:7] != "sha256:" {
		t.Errorf("expected sha256: prefix, got %s", result)
	}
}
