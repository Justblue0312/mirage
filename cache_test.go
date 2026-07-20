package mirage

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryCache_SetGet(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	type User struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	user := User{ID: 1, Name: "Alice"}
	if err := cache.Set(ctx, "user:1", user, 0); err != nil {
		t.Fatal(err)
	}

	var got User
	found, err := cache.Get(ctx, "user:1", &got)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if got.ID != 1 || got.Name != "Alice" {
		t.Fatalf("expected {1 Alice}, got %+v", got)
	}
}

func TestInMemoryCache_Expiration(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	cache.Set(ctx, "key", "value", 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	var got string
	found, err := cache.Get(ctx, "key", &got)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected found=false after expiration")
	}
}

func TestInMemoryCache_Delete(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	cache.Set(ctx, "key", "value", 0)
	cache.Delete(ctx, "key")

	var got string
	found, _ := cache.Get(ctx, "key", &got)
	if found {
		t.Fatal("expected found=false after delete")
	}
}

func TestInMemoryCache_Invalidate(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	cache.Set(ctx, "users:1", "a", 0)
	cache.Set(ctx, "users:2", "b", 0)
	cache.Set(ctx, "posts:1", "c", 0)

	cache.Invalidate(ctx, "users:")

	var got string
	if found, _ := cache.Get(ctx, "users:1", &got); found {
		t.Fatal("expected users:1 invalidated")
	}
	if found, _ := cache.Get(ctx, "users:2", &got); found {
		t.Fatal("expected users:2 invalidated")
	}
	if found, _ := cache.Get(ctx, "posts:1", &got); !found {
		t.Fatal("expected posts:1 to remain")
	}
}
