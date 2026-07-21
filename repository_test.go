package mirage

import (
	"reflect"
	"testing"

	schemapkg "github.com/justblue/mirage/internal/schema"
)

func TestExistsCacheKey_ContentEqualPointerFields(t *testing.T) {
	type Widget struct {
		ID   int64   `json:"id"`
		Name *string `json:"name"`
	}

	repo := &Repository[Widget]{
		table: &schemapkg.Table{Name: "widgets"},
	}

	e1, e2 := "foo", "foo"
	k1, err := repo.existsCacheKey(reflect.ValueOf(Widget{ID: 1, Name: &e1}))
	if err != nil {
		t.Fatal(err)
	}
	k2, err := repo.existsCacheKey(reflect.ValueOf(Widget{ID: 1, Name: &e2}))
	if err != nil {
		t.Fatal(err)
	}
	if k1 != k2 {
		t.Errorf("cache keys differ for content-equal structs:\n  k1=%s\n  k2=%s", k1, k2)
	}

	e3 := "bar"
	k3, err := repo.existsCacheKey(reflect.ValueOf(Widget{ID: 2, Name: &e3}))
	if err != nil {
		t.Fatal(err)
	}
	if k1 == k3 {
		t.Error("cache keys should differ for structs with different values")
	}
}
