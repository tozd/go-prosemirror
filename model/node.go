// Ported from prosemirror-model/src/node.ts.

package model

import (
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

var emptyAttrs = Attrs{} //nolint:gochecknoglobals

// Node represents a node in the tree that makes up a ProseMirror document. So a document is an instance of Node, with children that are also instances of Node.
//
// Nodes are persistent data structures. Instead of changing them, you create new ones with the content you want. Old ones keep pointing at the old document
// shape. This is made cheaper by sharing structure between the old and new data as much as possible, which a tree shape like this (without back pointers)
// makes easy.
//
// Do not directly mutate the fields of a Node object.
type Node struct {
	// The type of node that this is.
	Type *NodeType

	// An object mapping attribute names to values. The kind of attributes allowed and required are determined by the node type.
	Attrs Attrs

	// A container holding the node's children. EmptyFragment for leaf and text nodes.
	Content *Fragment

	// The marks (things like whether it is emphasized or part of a link) applied to this node.
	Marks []*Mark

	// For text nodes, this contains the node's text content. It is non-empty exactly for text nodes.
	Text string
}

func newNode(typ *NodeType, attrs Attrs, content *Fragment, marks []*Mark) *Node {
	if content == nil {
		content = EmptyFragment
	}
	if marks == nil {
		marks = NoMarks
	}
	return &Node{Type: typ, Attrs: attrs, Content: content, Marks: marks, Text: ""}
}

func newTextNode(typ *NodeType, attrs Attrs, text string, marks []*Mark) *Node {
	if text == "" {
		panic("empty text nodes are not allowed")
	}
	if marks == nil {
		marks = NoMarks
	}
	return &Node{Type: typ, Attrs: attrs, Content: EmptyFragment, Marks: marks, Text: text}
}

// NodeSize returns the size of this node, as defined by the integer-based indexing scheme. For text nodes, this is the amount of characters
// (UTF-16 code units). For other leaf nodes, it is one. For non-leaf nodes, it is the size of the content plus two (the start and end token).
func (n *Node) NodeSize() int {
	if n.IsText() {
		return utf16Length(n.Text)
	}
	if n.IsLeaf() {
		return 1
	}
	return 2 + n.Content.Size //nolint:mnd
}

// ChildCount returns the number of children that the node has.
func (n *Node) ChildCount() int {
	return n.Content.ChildCount()
}

// Child returns the child node at the given index. It panics when the index is out of range.
func (n *Node) Child(index int) *Node {
	return n.Content.Child(index)
}

// MaybeChild returns the child node at the given index, or nil when it does not exist.
func (n *Node) MaybeChild(index int) *Node {
	return n.Content.MaybeChild(index)
}

// ForEach calls fn for every child node, passing the node, its offset into this parent node, and its index.
func (n *Node) ForEach(fn func(node *Node, offset, index int)) {
	n.Content.ForEach(fn)
}

// TextContent concatenates all the text nodes found in this node and its children.
func (n *Node) TextContent() string {
	if n.IsText() {
		return n.Text
	}
	if n.IsLeaf() {
		return ""
	}
	var sb strings.Builder
	for _, child := range n.Content.Content {
		sb.WriteString(child.TextContent())
	}
	return sb.String()
}

// FirstChild returns this node's first child, or nil when there are no children.
func (n *Node) FirstChild() *Node {
	return n.Content.FirstChild()
}

// LastChild returns this node's last child, or nil when there are no children.
func (n *Node) LastChild() *Node {
	return n.Content.LastChild()
}

// Eq tests whether two nodes represent the same piece of document.
func (n *Node) Eq(other *Node) bool {
	if n == other {
		return true
	}
	if n.IsText() {
		return n.SameMarkup(other) && n.Text == other.Text
	}
	return n.SameMarkup(other) && n.Content.Eq(other.Content)
}

// SameMarkup compares the markup (type, attributes, and marks) of this node to those of another. It returns true when both have the same markup.
func (n *Node) SameMarkup(other *Node) bool {
	return n.HasMarkup(other.Type, other.Attrs, other.Marks)
}

// HasMarkup checks whether this node's markup corresponds to the given type, attributes, and marks.
func (n *Node) HasMarkup(typ *NodeType, attrs Attrs, marks []*Mark) bool {
	if attrs == nil {
		attrs = typ.DefaultAttrs
		if attrs == nil {
			attrs = emptyAttrs
		}
	}
	if marks == nil {
		marks = NoMarks
	}
	return n.Type == typ && compareDeep(map[string]any(n.Attrs), map[string]any(attrs)) && SameMarkSet(n.Marks, marks)
}

// Copy creates a new node with the same markup as this node, containing the given content (or empty, when nil is given).
func (n *Node) Copy(content *Fragment) *Node {
	if content == n.Content {
		return n
	}
	return newNode(n.Type, n.Attrs, content, n.Marks)
}

// Mark creates a copy of this node, with the given set of marks instead of the node's own marks.
func (n *Node) Mark(marks []*Mark) *Node {
	if SameMarkSet(marks, n.Marks) {
		return n
	}
	if n.IsText() {
		return newTextNode(n.Type, n.Attrs, n.Text, marks)
	}
	return newNode(n.Type, n.Attrs, n.Content, marks)
}

// WithText creates a copy of this text node with the given text. It panics when called on a non-text node.
func (n *Node) WithText(text string) *Node {
	if !n.IsText() {
		panic("WithText called on a non-text node")
	}
	if text == n.Text {
		return n
	}
	return newTextNode(n.Type, n.Attrs, text, n.Marks)
}

// IsBlock reports whether this is a block (non-inline) node.
func (n *Node) IsBlock() bool {
	return n.Type.IsBlock
}

// IsTextblock reports whether this is a textblock node, a block node with inline content.
func (n *Node) IsTextblock() bool {
	return n.Type.IsTextblock()
}

// InlineContent reports whether this node allows inline content.
func (n *Node) InlineContent() bool {
	return n.Type.InlineContent
}

// IsInline reports whether this is an inline node (a text node or a node that can appear among text).
func (n *Node) IsInline() bool {
	return n.Type.IsInline()
}

// IsText reports whether this is a text node.
func (n *Node) IsText() bool {
	return n.Type.IsText
}

// IsLeaf reports whether this is a leaf node.
func (n *Node) IsLeaf() bool {
	return n.Type.IsLeaf()
}

// IsAtom reports whether this is an atom, i.e. when it does not have directly editable content. This is usually the same as IsLeaf,
// but can be configured with the atom property on a node's spec.
func (n *Node) IsAtom() bool {
	return n.Type.IsAtom()
}

// String returns a string representation of this node for debugging purposes.
func (n *Node) String() string {
	if n.IsText() {
		return wrapMarks(n.Marks, jsonStringifyString(n.Text))
	}
	name := n.Type.Name
	if n.Content.Size > 0 {
		name += "(" + n.Content.toStringInner() + ")"
	}
	return wrapMarks(n.Marks, name)
}

// ContentMatchAt returns the content match in this node at the given index. It panics when the node's content is not valid.
func (n *Node) ContentMatchAt(index int) *ContentMatch {
	match := n.Type.ContentMatch.MatchFragment(n.Content, 0, index)
	if match == nil {
		panic("Called contentMatchAt on a node with invalid content")
	}
	return match
}

// Check checks whether this node and its descendants conform to the schema, and returns an error when they do not.
func (n *Node) Check() errors.E {
	errE := n.Type.CheckContent(n.Content)
	if errE != nil {
		return errE
	}
	errE = n.Type.CheckAttrs(n.Attrs)
	if errE != nil {
		return errE
	}
	copied := NoMarks
	for _, mark := range n.Marks {
		errE = mark.Type.CheckAttrs(mark.Attrs)
		if errE != nil {
			return errE
		}
		copied = mark.AddToSet(copied)
	}
	if !SameMarkSet(copied, n.Marks) {
		names := make([]string, len(n.Marks))
		for i, mark := range n.Marks {
			names[i] = mark.Type.Name
		}
		errE := errors.New("invalid collection of marks for node")
		details := errors.Details(errE)
		details["type"] = n.Type.Name
		details["marks"] = names
		return errE
	}
	for _, child := range n.Content.Content {
		errE = child.Check()
		if errE != nil {
			return errE
		}
	}
	return nil
}

// ToJSON returns a JSON-serializeable representation of this node.
func (n *Node) ToJSON() map[string]any {
	obj := map[string]any{"type": n.Type.Name}
	if len(n.Attrs) > 0 {
		obj["attrs"] = n.Attrs
	}
	if n.Content.Size > 0 {
		obj["content"] = n.Content.ToJSON()
	}
	if len(n.Marks) > 0 {
		marks := make([]any, len(n.Marks))
		for i, mark := range n.Marks {
			marks[i] = mark.ToJSON()
		}
		obj["marks"] = marks
	}
	if n.IsText() {
		obj["text"] = n.Text
	}
	return obj
}

// MarshalJSON implements the json.Marshaler interface.
func (n *Node) MarshalJSON() ([]byte, error) {
	data, errE := x.MarshalWithoutEscapeHTML(n.ToJSON())
	if errE != nil {
		return nil, errE
	}
	return data, nil
}

// nodeFromJSON deserializes a node from its JSON-decoded representation. It does not run Check.
func nodeFromJSON(schema *Schema, value any) (*Node, errors.E) {
	obj, ok := value.(map[string]any)
	if !ok || obj == nil {
		return nil, errors.New("invalid input for node JSON")
	}
	var marks []*Mark
	if m, present := obj["marks"]; present && m != nil {
		array, ok := m.([]any)
		if !ok {
			return nil, errors.New("invalid mark data in node JSON")
		}
		marks = make([]*Mark, len(array))
		for i, item := range array {
			mark, errE := markFromJSON(schema, item)
			if errE != nil {
				return nil, errE
			}
			marks[i] = mark
		}
	}
	typeName, _ := obj["type"].(string)
	if typeName == "text" { //nolint:goconst
		text, ok := obj["text"].(string)
		if !ok {
			return nil, errors.New("invalid text node in JSON")
		}
		if text == "" {
			return nil, errors.New("empty text nodes are not allowed")
		}
		return schema.Text(text, marks), nil
	}
	content, errE := fragmentFromJSON(schema, obj["content"])
	if errE != nil {
		return nil, errE
	}
	var attrs Attrs
	if a, present := obj["attrs"]; present && a != nil {
		m, ok := a.(map[string]any)
		if !ok {
			return nil, errors.New("invalid attrs in node JSON")
		}
		attrs = Attrs(m)
	}
	typ, errE := schema.NodeType(typeName)
	if errE != nil {
		return nil, errE
	}
	node, errE := typ.Create(attrs, content, marks)
	if errE != nil {
		return nil, errE
	}
	errE = typ.CheckAttrs(node.Attrs)
	if errE != nil {
		return nil, errE
	}
	return node, nil
}

func wrapMarks(marks []*Mark, str string) string {
	for i := len(marks) - 1; i >= 0; i-- { //nolint:modernize
		str = marks[i].Type.Name + "(" + str + ")"
	}
	return str
}

// jsonStringifyString quotes a string the way JavaScript JSON.stringify does: HTML characters are left raw and only the JSON-mandated characters are
// escaped, so the debug output matches the TypeScript reference (which wraps text node contents with JSON.stringify).
func jsonStringifyString(s string) string {
	// Encoding a string value cannot fail, so the error is ignored.
	data, _ := x.MarshalWithoutEscapeHTML(s)
	return string(data)
}

// utf16Length returns the length of the string in UTF-16 code units (JavaScript string length semantics).
func utf16Length(s string) int {
	length := 0
	for _, r := range s {
		if r > 0xFFFF { //nolint:mnd
			length += 2
		} else {
			length++
		}
	}
	return length
}
