package scanner

type AttrValue struct {
	Flag  bool
	Value string
	Args  []string
}

type Attrs map[string]AttrValue

func (a Attrs) Has(key string) bool {
	v, ok := a[key]
	if !ok {
		return false
	}
	return v.Flag || v.Value != "" || len(v.Args) > 0
}

func (a Attrs) String(key, fallback string) string {
	if v, ok := a[key]; ok && v.Value != "" {
		return v.Value
	}
	return fallback
}

func (a Attrs) Args(key string) []string {
	if v, ok := a[key]; ok {
		return v.Args
	}
	return nil
}

func (a Attrs) AllArgs(key string) [][]string {
	var result [][]string
	if v, ok := a[key]; ok {
		if len(v.Args) > 0 {
			result = append(result, v.Args)
		}
	}
	return result
}
