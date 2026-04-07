package jsonr

import (
	"fmt"
	"strings"
)

func dotNotationToPath(dot string) ([][]string, error) {
	dot = strings.TrimSpace(dot)
	if dot == "" {
		return nil, fmt.Errorf("empty jsonr path")
	}
	parts := splitPathSegments(dot)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty jsonr path")
	}
	paths := [][]string{{}}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("empty path segment in %q", dot)
		}
		if isMultiFieldSegment(p) {
			keys, err := parseMultiFieldSegment(p)
			if err != nil {
				return nil, fmt.Errorf("jsonr path %q: %w", dot, err)
			}
			next := make([][]string, 0, len(paths)*len(keys))
			for _, pref := range paths {
				for _, k := range keys {
					next = append(next, append(append([]string(nil), pref...), k))
				}
			}
			paths = next
		} else {
			for i := range paths {
				paths[i] = append(paths[i], p)
			}
		}
	}
	return paths, nil
}

// splitPathSegments splits dot on '.' only outside of '[' ... ']'.
func splitPathSegments(s string) []string {
	var parts []string
	start := 0
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func isMultiFieldSegment(p string) bool {
	return len(p) >= 2 && p[0] == '[' && p[len(p)-1] == ']'
}

func parseMultiFieldSegment(p string) ([]string, error) {
	inner := strings.TrimSpace(p[1 : len(p)-1])
	if inner == "" {
		return nil, fmt.Errorf("empty multi-field bracket %q", p)
	}
	raw := strings.Split(inner, ",")
	out := make([]string, 0, len(raw))
	for _, k := range raw {
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, fmt.Errorf("empty key in multi-field %q", p)
		}
		out = append(out, k)
	}
	return out, nil
}
