// Ported from prosemirror-model/src/fragment.ts.

package model

import (
	"strings"

	"gitlab.com/tozd/go/errors"
)

// Fragment represents a node's collection of child nodes.
//
// Like nodes, fragments are persistent data structures, and you should not mutate them or their content.
// Rather, you create new instances whenever needed. The API tries to make this easy.
type Fragment struct {
	// Content is the child nodes in this fragment. Read only.
	Content []*Node

	// Size is the size of the fragment, which is the total of the size of its content nodes. Read only.
	Size int
}

// newFragment constructs a fragment from the given child nodes. A negative size means the size is computed by summing the node sizes of the children.
func newFragment(content []*Node, size int) *Fragment {
	if size < 0 {
		size = 0
		for _, child := range content {
			size += child.NodeSize()
		}
	}
	return &Fragment{Content: content, Size: size}
}

// Append creates a new fragment containing the combined content of this fragment and the other.
func (f *Fragment) Append(other *Fragment) *Fragment {
	if other.Size == 0 {
		return f
	}
	if f.Size == 0 {
		return other
	}
	last, first := f.LastChild(), other.FirstChild()
	content := make([]*Node, len(f.Content), len(f.Content)+len(other.Content))
	copy(content, f.Content)
	i := 0
	if last.IsText() && last.SameMarkup(first) {
		content[len(content)-1] = last.WithText(last.Text + first.Text)
		i = 1
	}
	for ; i < len(other.Content); i++ {
		content = append(content, other.Content[i])
	}
	return newFragment(content, f.Size+other.Size)
}

// Eq compares this fragment to another one.
func (f *Fragment) Eq(other *Fragment) bool {
	if len(f.Content) != len(other.Content) {
		return false
	}
	for i, child := range f.Content {
		if !child.Eq(other.Content[i]) {
			return false
		}
	}
	return true
}

// FirstChild returns the first child of the fragment, or nil if it is empty.
func (f *Fragment) FirstChild() *Node {
	if len(f.Content) == 0 {
		return nil
	}
	return f.Content[0]
}

// LastChild returns the last child of the fragment, or nil if it is empty.
func (f *Fragment) LastChild() *Node {
	if len(f.Content) == 0 {
		return nil
	}
	return f.Content[len(f.Content)-1]
}

// ChildCount returns the number of child nodes in this fragment.
func (f *Fragment) ChildCount() int {
	return len(f.Content)
}

// Child returns the child node at the given index. It panics when the index is out of range.
func (f *Fragment) Child(index int) *Node {
	if index < 0 || index >= len(f.Content) {
		errE := errors.New("index out of range")
		details := errors.Details(errE)
		details["index"] = index
		details["fragment"] = f.String()
		panic(errE)
	}
	return f.Content[index]
}

// MaybeChild returns the child node at the given index, or nil when the index is out of range.
func (f *Fragment) MaybeChild(index int) *Node {
	if index < 0 || index >= len(f.Content) {
		return nil
	}
	return f.Content[index]
}

// ForEach calls fn for every child node, passing the node, its offset into this parent node, and its index.
func (f *Fragment) ForEach(fn func(node *Node, offset, index int)) {
	p := 0
	for i, child := range f.Content {
		fn(child, p, i)
		p += child.NodeSize()
	}
}

// String returns a debugging string that describes this fragment.
func (f *Fragment) String() string {
	return "<" + f.toStringInner() + ">"
}

func (f *Fragment) toStringInner() string {
	parts := make([]string, len(f.Content))
	for i, child := range f.Content {
		parts[i] = child.String()
	}
	return strings.Join(parts, ", ")
}

// ToJSON creates a JSON-serializable representation of this fragment. It returns nil when the fragment is empty, so that an enclosing node omits its content.
func (f *Fragment) ToJSON() []any {
	if len(f.Content) == 0 {
		return nil
	}
	result := make([]any, len(f.Content))
	for i, child := range f.Content {
		result[i] = child.ToJSON()
	}
	return result
}

// fragmentFromJSON deserializes a fragment from its JSON representation. A nil value means the empty fragment.
func fragmentFromJSON(schema *Schema, value any) (*Fragment, errors.E) {
	if value == nil {
		return EmptyFragment, nil
	}
	array, ok := value.([]any)
	if !ok {
		return nil, errors.New("invalid input for fragment JSON")
	}
	nodes := make([]*Node, len(array))
	for i, item := range array {
		node, errE := nodeFromJSON(schema, item)
		if errE != nil {
			return nil, errE
		}
		nodes[i] = node
	}
	return FragmentFromArray(nodes), nil
}

// FragmentFromArray builds a fragment from a slice of nodes. It ensures that adjacent text nodes with the same marks are joined together.
func FragmentFromArray(array []*Node) *Fragment {
	if len(array) == 0 {
		return EmptyFragment
	}
	var joined []*Node
	size := 0
	for i, node := range array {
		size += node.NodeSize()
		if i > 0 && node.IsText() && array[i-1].SameMarkup(node) { //nolint:gosec
			if joined == nil {
				joined = make([]*Node, i)
				copy(joined, array[:i])
			}
			joined[len(joined)-1] = node.WithText(joined[len(joined)-1].Text + node.Text)
		} else if joined != nil {
			joined = append(joined, node) //nolint:makezero
		}
	}
	if joined == nil {
		joined = array
	}
	return newFragment(joined, size)
}

// EmptyFragment is an empty fragment. It is intended to be reused whenever a node does not contain anything (rather than allocating a new empty fragment for
// each leaf node).
var EmptyFragment = &Fragment{Content: nil, Size: 0} //nolint:gochecknoglobals
