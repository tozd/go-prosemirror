// Ported from prosemirror-model/src/mark.ts.

package model

import (
	"sort"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

// Mark is a piece of information that can be attached to a node, such as it being emphasized, in code font, or a link. It has a type and
// optionally a set of attributes that provide further information (such as the target of the link). Marks are created through a Schema,
// which controls which types exist and which attributes they have.
type Mark struct {
	// Type is the type of this mark. Read only.
	Type *MarkType
	// Attrs are the attributes associated with this mark. Read only.
	Attrs Attrs
}

// AddToSet creates a new set which contains the marks of the given set as well as this one, in the right position. If this mark is already
// in the set, the set itself is returned. If any marks that are set to be exclusive with this mark are present, those are replaced by this
// one.
func (m *Mark) AddToSet(set []*Mark) []*Mark {
	var copied []*Mark
	placed := false
	for i := 0; i < len(set); i++ { //nolint:intrange,modernize
		other := set[i]
		if m.Eq(other) {
			return set
		}
		if m.Type.Excludes(other.Type) {
			if copied == nil {
				copied = append([]*Mark{}, set[:i]...)
			}
		} else if other.Type.Excludes(m.Type) {
			return set
		} else {
			if !placed && other.Type.Rank > m.Type.Rank {
				if copied == nil {
					copied = append([]*Mark{}, set[:i]...)
				}
				copied = append(copied, m)
				placed = true
			}
			if copied != nil {
				copied = append(copied, other)
			}
		}
	}
	if copied == nil {
		copied = append([]*Mark{}, set...)
	}
	if !placed {
		copied = append(copied, m)
	}
	return copied
}

// RemoveFromSet removes this mark from the given set, returning a new set. If this mark is not in the set, the set itself is returned.
func (m *Mark) RemoveFromSet(set []*Mark) []*Mark {
	for i := 0; i < len(set); i++ { //nolint:intrange,modernize
		if m.Eq(set[i]) {
			result := make([]*Mark, 0, len(set)-1)
			result = append(result, set[:i]...)
			result = append(result, set[i+1:]...)
			return result
		}
	}
	return set
}

// IsInSet tests whether this mark is in the given set of marks.
func (m *Mark) IsInSet(set []*Mark) bool {
	for i := 0; i < len(set); i++ { //nolint:intrange,modernize
		if m.Eq(set[i]) {
			return true
		}
	}
	return false
}

// Eq tests whether this mark has the same type and attributes as another mark.
func (m *Mark) Eq(other *Mark) bool {
	return m == other || (m.Type == other.Type && compareDeep(m.Attrs, other.Attrs))
}

// ToJSON converts this mark to a JSON-serializable representation.
func (m *Mark) ToJSON() map[string]any {
	obj := map[string]any{"type": m.Type.Name}
	if len(m.Attrs) > 0 {
		obj["attrs"] = m.Attrs
	}
	return obj
}

// MarshalJSON implements the json.Marshaler interface.
func (m *Mark) MarshalJSON() ([]byte, error) {
	data, errE := x.MarshalWithoutEscapeHTML(m.ToJSON())
	if errE != nil {
		return nil, errE
	}
	return data, nil
}

// markFromJSON deserializes a mark from a JSON-decoded value.
func markFromJSON(schema *Schema, value any) (*Mark, errors.E) {
	if value == nil {
		return nil, errors.New("invalid input for mark JSON")
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("invalid input for mark JSON")
	}
	typeName, _ := obj["type"].(string)
	typ := schema.Marks[typeName]
	if typ == nil {
		errE := errors.New("no such mark type in schema")
		errors.Details(errE)["type"] = obj["type"]
		return nil, errE
	}
	var attrs Attrs
	if a, ok := obj["attrs"].(map[string]any); ok {
		attrs = Attrs(a)
	}
	mark, errE := typ.Create(attrs)
	if errE != nil {
		return nil, errE
	}
	errE = typ.CheckAttrs(mark.Attrs)
	if errE != nil {
		return nil, errE
	}
	return mark, nil
}

// SameMarkSet tests whether two sets of marks are identical.
func SameMarkSet(a, b []*Mark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Eq(b[i]) {
			return false
		}
	}
	return true
}

// MarkSetFrom creates a properly sorted mark set from an unsorted slice of marks.
func MarkSetFrom(marks []*Mark) []*Mark {
	if len(marks) == 0 {
		return NoMarks
	}
	copied := append([]*Mark{}, marks...)
	sort.SliceStable(copied, func(i, j int) bool {
		return copied[i].Type.Rank < copied[j].Type.Rank
	})
	return copied
}

// NoMarks is the empty set of marks.
var NoMarks = []*Mark{} //nolint:gochecknoglobals
