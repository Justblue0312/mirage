package schema

import "github.com/jackc/pgx/v5"

// Quote sanitizes a single SQL identifier (schema, table, or column name)
// for safe interpolation into a query string, correctly escaping any
// embedded double-quote characters. This mirrors the root package's
// QuoteIdentifier and exists here because internal/schema builds its own
// SQL independently and has no dependency on that package.
func Quote(identifier string) string {
	return pgx.Identifier{identifier}.Sanitize()
}
