package schema

// IndexType represents a PostgreSQL index type.
type IndexType uint8

const (
	InvalidIndex IndexType = iota
	Btree
	Hash
	Gist
	Spgist
	Gin
	Brin
)

var indexTypeText = map[IndexType]string{
	Btree:  "btree",
	Hash:   "hash",
	Gist:   "gist",
	Spgist: "spgist",
	Gin:    "gin",
	Brin:   "brin",
}

func (t IndexType) String() string {
	if s, ok := indexTypeText[t]; ok {
		return s
	}
	return "btree"
}

func (t *IndexType) Scan(src interface{}) error {
	if src == nil {
		*t = InvalidIndex
		return nil
	}
	s, ok := src.(string)
	if !ok {
		*t = InvalidIndex
		return nil
	}
	*t = ParseIndexType(s)
	return nil
}

// ParseIndexType parses a string into an IndexType.
func ParseIndexType(s string) IndexType {
	switch s {
	case "btree":
		return Btree
	case "hash":
		return Hash
	case "gist":
		return Gist
	case "spgist":
		return Spgist
	case "gin":
		return Gin
	case "brin":
		return Brin
	default:
		return InvalidIndex
	}
}
