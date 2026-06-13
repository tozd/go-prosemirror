// Ported from prosemirror-model/src/comparedeep.ts.

package model

// compareDeep structurally compares two JSON-decoded values (nil, bool, float64, string, []any, map[string]any). Scalars compare with ==,
// slices and maps recursively. A slice never equals a map. Attrs values are accepted and compared as maps.
func compareDeep(a, b any) bool {
	if av, ok := a.(Attrs); ok {
		a = map[string]any(av)
	}
	if bv, ok := b.(Attrs); ok {
		b = map[string]any(bv)
	}
	switch av := a.(type) {
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return false
		}
		if len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !compareDeep(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return false
		}
		if len(av) != len(bv) {
			return false
		}
		for p, va := range av {
			vb, ok := bv[p]
			if !ok || !compareDeep(va, vb) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
