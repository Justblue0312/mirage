package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justblue/mirage/internal/schema"
)

type Graph struct {
	vertices map[string]struct{}
	edges    map[string][]string
}

func Build(tables []schema.Table) *Graph {
	g := &Graph{
		vertices: make(map[string]struct{}),
		edges:    make(map[string][]string),
	}

	for _, t := range tables {
		name := t.SQLName()
		g.vertices[name] = struct{}{}
	}

	for _, t := range tables {
		name := t.SQLName()
		for _, fk := range t.ForeignKeys {
			if fk.ToTable == name {
				continue
			}
			if _, ok := g.vertices[fk.ToTable]; ok {
				g.edges[fk.ToTable] = append(g.edges[fk.ToTable], name)
			}
		}
		for _, c := range t.Columns {
			if c.ReferenceTableName != "" && c.ReferenceTableName != name {
				if _, ok := g.vertices[c.ReferenceTableName]; ok {
					g.edges[c.ReferenceTableName] = append(g.edges[c.ReferenceTableName], name)
				}
			}
		}
	}

	for k := range g.edges {
		g.edges[k] = unique(g.edges[k])
	}

	return g
}

func (g *Graph) Sort() ([]string, error) {
	inDegree := make(map[string]int)
	for v := range g.vertices {
		inDegree[v] = 0
	}

	for _, tos := range g.edges {
		for _, to := range tos {
			if _, ok := g.vertices[to]; ok {
				inDegree[to]++
			}
		}
	}

	var queue []string
	for v := range g.vertices {
		if inDegree[v] == 0 {
			queue = append(queue, v)
		}
	}
	sort.Strings(queue)

	var result []string
	for len(queue) > 0 {
		sort.Strings(queue)
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		for _, neighbor := range g.edges[node] {
			if _, ok := g.vertices[neighbor]; !ok {
				continue
			}
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(result) != len(g.vertices) {
		return nil, fmt.Errorf("circular dependency: %s", strings.Join(g.findCycle(), " -> "))
	}

	return result, nil
}

func (g *Graph) findCycle() []string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var path []string

	var dfs func(string) bool
	dfs = func(v string) bool {
		visited[v] = true
		recStack[v] = true
		path = append(path, v)

		for _, neighbor := range g.edges[v] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				idx := -1
				for i, p := range path {
					if p == neighbor {
						idx = i
						break
					}
				}
				if idx >= 0 {
					path = append(path[idx:], neighbor)
					return true
				}
			}
		}

		recStack[v] = false
		path = path[:len(path)-1]
		return false
	}

	var sorted []string
	for v := range g.vertices {
		sorted = append(sorted, v)
	}
	sort.Strings(sorted)

	for _, v := range sorted {
		if !visited[v] {
			if dfs(v) {
				return path
			}
		}
	}

	return nil
}

func unique(s []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
