package openapi

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSpecsParseAndRefsResolve catches YAML mistakes and dangling $refs in the
// embedded specs (local "#/..." refs and "public.yaml#/..." refs from admin).
func TestSpecsParseAndRefsResolve(t *testing.T) {
	docs := map[string]map[string]any{}
	for _, name := range []string{"public.yaml", "admin.yaml"} {
		b, err := assets.ReadFile("spec/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		docs[name] = doc
	}

	for name, doc := range docs {
		var refs []string
		collectRefs(doc, &refs)
		for _, ref := range refs {
			file, pointer, _ := strings.Cut(ref, "#")
			target := doc
			if file != "" {
				target = docs[file]
				if target == nil {
					t.Errorf("%s: $ref %q: unknown file", name, ref)
					continue
				}
			}
			if err := resolvePointer(target, pointer); err != nil {
				t.Errorf("%s: $ref %q: %v", name, ref, err)
			}
		}
	}
}

func collectRefs(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if k == "$ref" {
				if s, ok := child.(string); ok {
					*out = append(*out, s)
				}
				continue
			}
			collectRefs(child, out)
		}
	case []any:
		for _, child := range x {
			collectRefs(child, out)
		}
	}
}

func resolvePointer(doc map[string]any, pointer string) error {
	var cur any = doc
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		m, ok := cur.(map[string]any)
		if !ok {
			return fmt.Errorf("%q is not an object", part)
		}
		if cur, ok = m[part]; !ok {
			return fmt.Errorf("missing %q", part)
		}
	}
	return nil
}
