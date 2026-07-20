package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/justblue/mirage"
)

// Product demonstrates Repository[T] with query caching.
type Product struct {
	_ struct{} `db:"name=products"`

	ID    int64   `db:"pk,identity,type=bigserial"`
	Name  string  `db:"name=name,type=varchar(255),notnull"`
	Price float64 `db:"name=price,type=numeric(10,2),notnull"`
}

func main() {
	connString := os.Getenv("MIRAGE_TEST_DATABASE_URL")
	if connString == "" {
		connString = "postgres://test:test@localhost:15432/mirage_test?sslmode=disable"
	}

	ctx := context.Background()

	db, err := mirage.Open(ctx, connString)
	if err != nil {
		log.Fatal("open db:", err)
	}
	defer db.Close()

	cache := mirage.NewInMemoryCache()
	repo := mirage.NewRepository[Product](db, mirage.WithCache(cache, 5*time.Minute))

	// Insert a product — InsertReturning scans all DB-generated columns back
	p := &Product{Name: "Widget", Price: 9.99}
	if err := repo.InsertReturning(ctx, p); err != nil {
		log.Fatal("insert:", err)
	}
	fmt.Printf("Inserted: id=%d name=%s price=$%.2f\n", p.ID, p.Name, p.Price)

	// Query with cache — first call hits DB, subsequent calls use cache
	products, err := repo.QueryWithCache(ctx, "products:all", 5*time.Minute,
		`SELECT * FROM "public"."products" WHERE "price" < $1`, 100.0)
	if err != nil {
		log.Fatal("query:", err)
	}
	fmt.Printf("Found %d products under $100\n", len(products))

	// Invalidate cache after any mutation
	if err := repo.InvalidateCache(ctx, "products:"); err != nil {
		log.Fatal("invalidate:", err)
	}
	fmt.Println("Cache invalidated")

	// Select by primary key
	found, err := repo.SelectByID(ctx, p.ID)
	if err != nil {
		log.Fatal("select by id:", err)
	}
	fmt.Printf("Found by ID: %s ($%.2f)\n", found.Name, found.Price)
}
