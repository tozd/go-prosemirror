// Ported from prosemirror-model/test/test-node.ts.
//
// The describe block for cut is not ported because the port skips Node.cut (see PORTING.md). nodesBetween (between) and textBetween are ported
// and covered by TestNodeNodesBetween and TestNodeTextBetween. The toDebugString and leafText spec options do not exist in the schema JSON
// dialect, so the tests exercising them are adapted to assert the default String and TextContent behavior instead (and textBetween supplies leaf
// text through the leafText callback argument rather than a node spec). Fragment behavior reachable through the public API (FragmentFromArray, Append, ForEach,
// Child) is covered here as well, since fragment.ts has no dedicated test file.

package model //nolint:testpackage

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
)

// nodeTestBuilder provides shorthand constructors over the basic schema for building test documents, mirroring the prosemirror-test-builder
// helpers used by the TypeScript tests. All helper names in this file are prefixed with nodeTest to avoid collisions with helpers defined in
// other test files of this package.
type nodeTestBuilder struct {
	t      *testing.T
	schema *Schema
}

func newNodeTestBuilder(t *testing.T) *nodeTestBuilder {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "basic-schema.json"))
	require.NoError(t, err)
	schema, errE := NewSchema(data, SchemaCallbacks{})
	require.NoError(t, errE, "% -+#.1v", errE)
	return &nodeTestBuilder{t: t, schema: schema}
}

func (b *nodeTestBuilder) node(typeName string, attrs Attrs, content ...*Node) *Node {
	b.t.Helper()
	node, errE := b.schema.Node(typeName, attrs, content, nil)
	require.NoError(b.t, errE, "% -+#.1v", errE)
	return node
}

func (b *nodeTestBuilder) mark(typeName string, attrs Attrs) *Mark {
	b.t.Helper()
	mark, errE := b.schema.Mark(typeName, attrs)
	require.NoError(b.t, errE, "% -+#.1v", errE)
	return mark
}

func (b *nodeTestBuilder) text(text string, marks ...*Mark) *Node {
	return b.schema.Text(text, marks)
}

func (b *nodeTestBuilder) doc(content ...*Node) *Node { return b.node("doc", nil, content...) }
func (b *nodeTestBuilder) p(content ...*Node) *Node   { return b.node("paragraph", nil, content...) }
func (b *nodeTestBuilder) blockquote(content ...*Node) *Node {
	return b.node("blockquote", nil, content...)
}
func (b *nodeTestBuilder) ul(content ...*Node) *Node { return b.node("bullet_list", nil, content...) }
func (b *nodeTestBuilder) li(content ...*Node) *Node { return b.node("list_item", nil, content...) }
func (b *nodeTestBuilder) hr() *Node                 { return b.node("horizontal_rule", nil) }
func (b *nodeTestBuilder) br() *Node                 { return b.node("hard_break", nil) }
func (b *nodeTestBuilder) img() *Node                { return b.node("image", Attrs{"src": "img.png"}) }

// nodeTestValidateSchemaJSON is a minimal schema with a validated attribute, mirroring the validate option which prosemirror-schema-basic puts
// on the src attribute of the image node in the TypeScript test suite (the basic-schema.json fixture does not declare validators).
const nodeTestValidateSchemaJSON = `{
	"nodes": {
		"doc": {"content": "block+"},
		"paragraph": {"group": "block", "content": "inline*", "toHTML": {"tag": "p"}},
		"text": {"group": "inline"},
		"image": {
			"inline": true,
			"group": "inline",
			"attrs": {"src": {"validate": "string"}},
			"toHTML": {"tag": "img", "attrs": ["src"]}
		}
	}
}`

func TestNodeString(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)
	strong := b.mark("strong", nil)
	code := b.mark("code", nil)

	tests := []struct {
		name string
		node *Node
		want string
	}{
		{
			"nests",
			b.doc(b.ul(b.li(b.p(b.text("hey")), b.p()), b.li(b.p(b.text("foo"))))),
			`doc(bullet_list(list_item(paragraph("hey"), paragraph), list_item(paragraph("foo"))))`,
		},
		{
			"shows inline children",
			b.doc(b.p(b.text("foo"), b.img(), b.br(), b.text("bar"))),
			`doc(paragraph("foo", image, hard_break, "bar"))`,
		},
		{
			"shows marks",
			b.doc(b.p(b.text("foo"), b.text("bar", em), b.text("quux", em, strong), b.text("baz", code))),
			`doc(paragraph("foo", em("bar"), em(strong("quux")), code("baz")))`,
		},
		{
			"should have the default toString method [text]",
			b.text("hello"),
			`"hello"`,
		},
		{
			"should have the default toString method [br]",
			b.br(),
			"hard_break",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.node.String())
		})
	}
}

func TestFragmentString(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)

	t.Run("should be respected by Fragment", func(t *testing.T) {
		t.Parallel()
		frag := FragmentFromArray([]*Node{b.text("hello"), b.br(), b.text("world")})
		assert.Equal(t, `<"hello", hard_break, "world">`, frag.String())
	})

	t.Run("empty fragment", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "<>", EmptyFragment.String())
	})
}

func TestNodeTextContent(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)

	t.Run("works on a whole doc", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "foo", b.doc(b.p(b.text("foo"))).TextContent())
	})

	t.Run("works on a text node", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "foo", b.text("foo").TextContent())
	})

	t.Run("works on a nested element", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "hiab", b.doc(b.ul(b.li(b.p(b.text("hi"))), b.li(b.p(b.text("a", em), b.text("b"))))).TextContent())
	})

	t.Run("is empty for a leaf node", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, b.hr().TextContent())
	})
}

func TestNodeNodesBetween(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)

	t.Run("iterates over the nodes in document order", func(t *testing.T) {
		t.Parallel()
		d := b.doc(b.p(b.text("foo")), b.p(b.text("bar"), b.br(), b.text("baz")))
		var texts []string
		d.NodesBetween(0, d.Content.Size, func(node *Node, _ int, _ *Node, _ int) bool {
			if node.IsText() {
				texts = append(texts, node.Text)
			}
			return true
		})
		assert.Equal(t, []string{"foo", "bar", "baz"}, texts)
	})

	t.Run("does not descend when the callback returns false", func(t *testing.T) {
		t.Parallel()
		d := b.doc(b.blockquote(b.p(b.text("inside"))), b.p(b.text("after")))
		var texts []string
		d.NodesBetween(0, d.Content.Size, func(node *Node, _ int, _ *Node, _ int) bool {
			if node.Type.Name == "blockquote" {
				return false
			}
			if node.IsText() {
				texts = append(texts, node.Text)
			}
			return true
		})
		assert.Equal(t, []string{"after"}, texts)
	})
}

func TestNodeTextBetween(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)

	t.Run("works when passing a custom function as leafText", func(t *testing.T) {
		t.Parallel()
		d := b.doc(b.p(b.text("foo"), b.img(), b.br(), b.text("bar")))
		got := d.TextBetween(0, d.Content.Size, "", func(node *Node) string {
			switch node.Type.Name {
			case "image":
				return "<image>"
			case "hard_break":
				return "<break>"
			default:
				return ""
			}
		})
		assert.Equal(t, "foo<image><break>bar", got)
	})

	t.Run("passes the separator between block nodes", func(t *testing.T) {
		t.Parallel()
		d := b.doc(b.p(b.text("foo")), b.p(b.text("bar")))
		assert.Equal(t, "foo\nbar", d.TextBetween(0, d.Content.Size, "\n", nil))
	})

	t.Run("separates blocks nested in non-textblock blocks", func(t *testing.T) {
		t.Parallel()
		d := b.doc(b.blockquote(b.p(b.text("foo"))), b.blockquote(b.p(b.text("bar"))))
		assert.Equal(t, "foo bar", d.TextBetween(0, d.Content.Size, " ", nil))
	})

	t.Run("concatenates without a separator and drops leaf nodes without leafText", func(t *testing.T) {
		t.Parallel()
		d := b.doc(b.p(b.text("foo"), b.br(), b.text("bar")), b.p(b.text("baz")))
		assert.Equal(t, "foobarbaz", d.TextBetween(0, d.Content.Size, "", nil))
	})

	t.Run("extracts a partial range, slicing the text nodes", func(t *testing.T) {
		t.Parallel()
		// doc(p("hello")): content positions 1..6 cover "hello", so 2..5 is "ell".
		d := b.doc(b.p(b.text("hello")))
		assert.Equal(t, "ell", d.TextBetween(2, 5, "", nil))
	})
}

func TestNodeEq(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)

	t.Run("equal documents built separately", func(t *testing.T) {
		t.Parallel()
		assert.True(t, b.doc(b.p(b.text("foo"))).Eq(b.doc(b.p(b.text("foo")))))
	})

	t.Run("a node equals itself", func(t *testing.T) {
		t.Parallel()
		node := b.doc(b.p(b.text("foo")))
		assert.True(t, node.Eq(node))
	})

	t.Run("different text", func(t *testing.T) {
		t.Parallel()
		assert.False(t, b.doc(b.p(b.text("foo"))).Eq(b.doc(b.p(b.text("bar")))))
	})

	t.Run("different marks", func(t *testing.T) {
		t.Parallel()
		assert.False(t, b.text("foo").Eq(b.text("foo", em)))
	})

	t.Run("different child count", func(t *testing.T) {
		t.Parallel()
		assert.False(t, b.doc(b.p()).Eq(b.doc(b.p(), b.p())))
	})

	t.Run("different attrs", func(t *testing.T) {
		t.Parallel()
		one := b.node("heading", Attrs{"level": float64(1)}, b.text("x"))
		two := b.node("heading", Attrs{"level": float64(2)}, b.text("x"))
		assert.False(t, one.Eq(two))
	})
}

func TestNodeSameMarkup(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)

	t.Run("same type and attrs with different content", func(t *testing.T) {
		t.Parallel()
		assert.True(t, b.p(b.text("foo")).SameMarkup(b.p(b.text("bar"))))
	})

	t.Run("different types", func(t *testing.T) {
		t.Parallel()
		assert.False(t, b.p(b.text("foo")).SameMarkup(b.node("heading", nil, b.text("foo"))))
	})

	t.Run("same marks on different text", func(t *testing.T) {
		t.Parallel()
		assert.True(t, b.text("a", em).SameMarkup(b.text("b", em)))
	})

	t.Run("different marks", func(t *testing.T) {
		t.Parallel()
		assert.False(t, b.text("a", em).SameMarkup(b.text("a")))
	})

	t.Run("different attrs", func(t *testing.T) {
		t.Parallel()
		assert.False(t, b.node("image", Attrs{"src": "a.png"}).SameMarkup(b.node("image", Attrs{"src": "b.png"})))
	})
}

func TestNodeHasMarkup(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)
	headingType := b.schema.Nodes["heading"]
	textType := b.schema.Nodes["text"]

	t.Run("nil attrs mean the default attrs", func(t *testing.T) {
		t.Parallel()
		assert.True(t, b.node("heading", nil, b.text("x")).HasMarkup(headingType, nil, nil))
		assert.False(t, b.node("heading", Attrs{"level": float64(2)}, b.text("x")).HasMarkup(headingType, nil, nil))
	})

	t.Run("explicit attrs", func(t *testing.T) {
		t.Parallel()
		node := b.node("heading", Attrs{"level": float64(2)}, b.text("x"))
		assert.True(t, node.HasMarkup(headingType, Attrs{"level": float64(2)}, nil))
		assert.False(t, node.HasMarkup(headingType, Attrs{"level": float64(3)}, nil))
	})

	t.Run("marks", func(t *testing.T) {
		t.Parallel()
		marked := b.text("a", em)
		assert.True(t, marked.HasMarkup(textType, nil, []*Mark{em}))
		assert.False(t, marked.HasMarkup(textType, nil, nil))
		assert.False(t, b.text("a").HasMarkup(textType, nil, []*Mark{em}))
	})

	t.Run("different type", func(t *testing.T) {
		t.Parallel()
		assert.False(t, b.p().HasMarkup(headingType, nil, nil))
	})
}

func TestNodeSize(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)

	t.Run("text nodes count UTF-16 code units", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 3, b.text("foo").NodeSize())
		// U+1F600 is outside the basic multilingual plane and counts as two UTF-16 code units.
		assert.Equal(t, 4, b.text("a\U0001F600b").NodeSize())
	})

	t.Run("leaf nodes have size one", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 1, b.hr().NodeSize())
		assert.Equal(t, 1, b.br().NodeSize())
		assert.Equal(t, 1, b.img().NodeSize())
	})

	t.Run("non-leaf nodes add two for the start and end tokens", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 2, b.p().NodeSize())
		assert.Equal(t, 5, b.p(b.text("foo")).NodeSize())
		assert.Equal(t, 7, b.doc(b.p(b.text("foo"))).NodeSize())
		assert.Equal(t, 6, b.p(b.text("a\U0001F600b")).NodeSize())
	})

	t.Run("fragment size sums child sizes", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 10, b.doc(b.p(b.text("foo")), b.p(b.text("bar"))).Content.Size)
	})
}

func TestNodeUTF16Length(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abc", 3},
		{"\u00e9", 1},
		{"\uffff", 1},
		{"\U00010000", 2},
		{"a\U0001F600b", 4},
		{"\U0001F600\U0001F602", 4},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.input), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, utf16Length(tt.input))
		})
	}
}

func TestNodeCheck(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)
	strong := b.mark("strong", nil)

	t.Run("passes a valid document", func(t *testing.T) {
		t.Parallel()
		valid := b.doc(
			b.p(b.text("foo"), b.text("bar", em), b.img(), b.br()),
			b.blockquote(b.ul(b.li(b.p(b.text("a")), b.p(b.text("b"))), b.li(b.p()))),
			b.node("heading", Attrs{"level": float64(2)}, b.text("head")),
			b.hr(),
		)
		errE := valid.Check()
		assert.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("notices invalid content", func(t *testing.T) {
		t.Parallel()
		li, errE := b.schema.Nodes["list_item"].Create(nil, FragmentFromArray([]*Node{b.text("x")}), nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		docNode, errE := b.schema.Nodes["doc"].Create(nil, FragmentFromArray([]*Node{li}), nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.EqualError(t, docNode.Check(), "invalid content for node")
	})

	t.Run("notices marks in wrong places", func(t *testing.T) {
		t.Parallel()
		para, errE := b.schema.Nodes["paragraph"].Create(nil, nil, []*Mark{em})
		require.NoError(t, errE, "% -+#.1v", errE)
		docNode, errE := b.schema.Nodes["doc"].Create(nil, FragmentFromArray([]*Node{para}), nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.EqualError(t, docNode.Check(), "invalid content for node")
	})

	t.Run("notices incorrect sets of marks", func(t *testing.T) {
		t.Parallel()
		assert.EqualError(t, b.text("a", em, em).Check(), "invalid collection of marks for node")
	})

	t.Run("notices marks in the wrong order", func(t *testing.T) {
		t.Parallel()
		textType := b.schema.Nodes["text"]
		node := newTextNode(textType, textType.DefaultAttrs, "a", []*Mark{strong, em})
		assert.EqualError(t, node.Check(), "invalid collection of marks for node")
	})

	t.Run("notices disallowed marks", func(t *testing.T) {
		t.Parallel()
		cb, errE := b.schema.Nodes["code_block"].Create(nil, FragmentFromArray([]*Node{b.text("x", em)}), nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.EqualError(t, cb.Check(), "invalid content for node")
	})

	t.Run("notices wrong attribute types", func(t *testing.T) {
		t.Parallel()
		custom, errE := NewSchema([]byte(nodeTestValidateSchemaJSON), SchemaCallbacks{})
		require.NoError(t, errE, "% -+#.1v", errE)
		node, errE := custom.Nodes["image"].Create(Attrs{"src": true}, nil, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.EqualError(t, node.Check(), "unexpected attribute value type")
	})

	t.Run("notices undeclared attributes", func(t *testing.T) {
		t.Parallel()
		node := newNode(b.schema.Nodes["heading"], Attrs{"level": float64(1), "bogus": true}, nil, nil)
		assert.EqualError(t, node.Check(), "unsupported attribute")
	})

	t.Run("notices missing attributes", func(t *testing.T) {
		t.Parallel()
		node := newNode(b.schema.Nodes["image"], Attrs{}, nil, nil)
		assert.EqualError(t, node.Check(), "missing attribute")
	})

	t.Run("notices invalid mark attributes", func(t *testing.T) {
		t.Parallel()
		textType := b.schema.Nodes["text"]
		mark := &Mark{Type: b.schema.Marks["link"], Attrs: Attrs{}}
		node := newTextNode(textType, textType.DefaultAttrs, "a", []*Mark{mark})
		assert.EqualError(t, node.Check(), "missing attribute")
	})
}

func TestNodeToJSON(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)
	link := b.mark("link", Attrs{"href": "http://example.com"})

	t.Run("serializes nested content, attrs, and marks", func(t *testing.T) {
		t.Parallel()
		docNode := b.doc(
			b.node("heading", Attrs{"level": float64(2)}, b.text("foo"), b.text("bar", em)),
			b.p(b.img(), b.text("baz", link)),
		)
		data, errE := x.MarshalWithoutEscapeHTML(docNode)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.JSONEq(t, `{
			"type": "doc",
			"content": [
				{"type": "heading", "attrs": {"level": 2}, "content": [
					{"type": "text", "text": "foo"},
					{"type": "text", "marks": [{"type": "em"}], "text": "bar"}
				]},
				{"type": "paragraph", "content": [
					{"type": "image", "attrs": {"src": "img.png", "alt": null, "title": null}},
					{"type": "text", "marks": [{"type": "link", "attrs": {"href": "http://example.com", "title": null}}], "text": "baz"}
				]}
			]
		}`, string(data))
	})

	t.Run("omits empty content, attrs, and marks", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, map[string]any{"type": "paragraph"}, b.p().ToJSON())
		assert.Equal(t, map[string]any{"type": "text", "text": "hi"}, b.text("hi").ToJSON())
	})
}

func TestNodeJSONRoundTrip(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)
	strong := b.mark("strong", nil)
	link := b.mark("link", Attrs{"href": "foo"})

	tests := []struct {
		name string
		doc  *Node
	}{
		{
			"can serialize a simple node",
			b.doc(b.p(b.text("foo"))),
		},
		{
			"can serialize marks",
			b.doc(b.p(b.text("foo"), b.text("bar", em), b.text("baz", em, strong), b.text(" "), b.text("x", link))),
		},
		{
			"can serialize inline leaf nodes",
			b.doc(b.p(b.text("foo"), b.img().Mark([]*Mark{em}), b.text("bar", em))),
		},
		{
			"can serialize block leaf nodes",
			b.doc(b.p(b.text("a")), b.hr(), b.p(b.text("b")), b.p()),
		},
		{
			"can serialize nested nodes",
			b.doc(b.blockquote(b.ul(b.li(b.p(b.text("a")), b.p(b.text("b"))), b.li(b.p(b.img()))), b.p(b.text("c"))), b.p(b.text("d"))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, errE := x.MarshalWithoutEscapeHTML(tt.doc)
			require.NoError(t, errE, "% -+#.1v", errE)
			restored, errE := b.schema.NodeFromJSON(data)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.True(t, restored.Eq(tt.doc))
		})
	}
}

func TestNodeFromJSONErrors(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"syntactically invalid JSON", `{"type":`, "unexpected EOF"},
		{"null input", `null`, "invalid input for node JSON"},
		{"number input", `42`, "invalid input for node JSON"},
		{"array input", `[]`, "invalid input for node JSON"},
		{"missing type", `{}`, "unknown node type"},
		{"unknown type", `{"type":"bogus"}`, "unknown node type"},
		{"invalid mark data", `{"type":"paragraph","marks":"em"}`, "invalid mark data in node JSON"},
		{"invalid mark item", `{"type":"paragraph","marks":[null]}`, "invalid input for mark JSON"},
		{"unknown mark type", `{"type":"paragraph","marks":[{"type":"bogus"}]}`, "no such mark type in schema"},
		{"mark missing a required attribute", `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link"}]}]}`, "no value supplied for attribute"},
		{"empty text node", `{"type":"text","text":""}`, "empty text nodes are not allowed"},
		{"text node without text", `{"type":"text"}`, "invalid text node in JSON"},
		{"text node with non-string text", `{"type":"text","text":3}`, "invalid text node in JSON"},
		{"attrs not an object", `{"type":"heading","attrs":[]}`, "invalid attrs in node JSON"},
		{"content not an array", `{"type":"doc","content":{}}`, "invalid input for fragment JSON"},
		{"node missing a required attribute", `{"type":"image"}`, "no value supplied for attribute"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, errE := b.schema.NodeFromJSON([]byte(tt.input))
			assert.EqualError(t, errE, tt.wantErr)
		})
	}

	t.Run("rejects attribute values failing validation", func(t *testing.T) {
		t.Parallel()
		custom, errE := NewSchema([]byte(nodeTestValidateSchemaJSON), SchemaCallbacks{})
		require.NoError(t, errE, "% -+#.1v", errE)
		_, errE = custom.NodeFromJSON([]byte(`{"type":"image","attrs":{"src":5}}`))
		assert.EqualError(t, errE, "unexpected attribute value type")
	})
}

func TestSchemaNodeFromJSONValidates(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	input := []byte(`{"type":"doc","content":[{"type":"list_item","content":[{"type":"paragraph"}]}]}`)

	// nodeFromJSON does not run Check, so a structurally well formed but schema invalid document deserializes.
	var value any
	errE := x.UnmarshalWithoutUnknownFields(input, &value)
	require.NoError(t, errE, "% -+#.1v", errE)
	node, errE := nodeFromJSON(b.schema, value)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "doc(list_item(paragraph))", node.String())

	// Schema.NodeFromJSON runs a full Check after deserialization.
	_, errE = b.schema.NodeFromJSON(input)
	assert.EqualError(t, errE, "invalid content for node")
}

func TestFragmentFrom(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)

	from := func(t *testing.T, arg []*Node, expect *Node) {
		t.Helper()
		assert.True(t, expect.Copy(FragmentFromArray(arg)).Eq(expect))
	}

	t.Run("wraps a single node", func(t *testing.T) {
		t.Parallel()
		from(t, []*Node{b.p()}, b.doc(b.p()))
	})

	t.Run("wraps an array", func(t *testing.T) {
		t.Parallel()
		from(t, []*Node{b.br(), b.text("foo")}, b.p(b.br(), b.text("foo")))
	})

	t.Run("preserves a fragment", func(t *testing.T) {
		t.Parallel()
		from(t, b.doc(b.p(b.text("foo"))).Content.Content, b.doc(b.p(b.text("foo"))))
	})

	t.Run("accepts null", func(t *testing.T) {
		t.Parallel()
		assert.Same(t, EmptyFragment, FragmentFromArray(nil))
		from(t, nil, b.p())
	})

	t.Run("joins adjacent text", func(t *testing.T) {
		t.Parallel()
		from(t, []*Node{b.text("a"), b.text("b")}, b.p(b.text("ab")))
		frag := FragmentFromArray([]*Node{b.text("a"), b.text("b")})
		assert.Equal(t, 1, frag.ChildCount())
		assert.Equal(t, "ab", frag.Child(0).Text)
	})

	t.Run("joins adjacent text through Schema.Node", func(t *testing.T) {
		t.Parallel()
		para := b.node("paragraph", nil, b.text("a"), b.text("b"))
		assert.Equal(t, 1, para.ChildCount())
		assert.Equal(t, "ab", para.FirstChild().Text)
	})

	t.Run("does not join text with different markup", func(t *testing.T) {
		t.Parallel()
		frag := FragmentFromArray([]*Node{b.text("a"), b.text("b", em)})
		assert.Equal(t, 2, frag.ChildCount())
	})
}

func TestFragmentAppend(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	em := b.mark("em", nil)

	foo := b.p(b.text("foo")).Content
	bar := b.p(b.text("bar")).Content

	t.Run("merges adjacent text with the same markup", func(t *testing.T) {
		t.Parallel()
		appended := foo.Append(bar)
		assert.Equal(t, 1, appended.ChildCount())
		assert.Equal(t, "foobar", appended.Child(0).Text)
		assert.Equal(t, 6, appended.Size)
	})

	t.Run("does not merge text with different markup", func(t *testing.T) {
		t.Parallel()
		appended := foo.Append(b.p(b.text("bar", em)).Content)
		assert.Equal(t, 2, appended.ChildCount())
		assert.Equal(t, 6, appended.Size)
	})

	t.Run("does not merge non-text nodes", func(t *testing.T) {
		t.Parallel()
		appended := b.p(b.br()).Content.Append(foo)
		assert.Equal(t, 2, appended.ChildCount())
	})

	t.Run("returns the non-empty side for empty fragments", func(t *testing.T) {
		t.Parallel()
		assert.Same(t, foo, foo.Append(EmptyFragment))
		assert.Same(t, foo, EmptyFragment.Append(foo))
	})
}

func TestFragmentForEach(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)

	t.Run("reports block offsets and indexes", func(t *testing.T) {
		t.Parallel()
		docNode := b.doc(b.p(b.text("hi")), b.hr(), b.p(b.text("bye")))
		var names []string
		var offsets, indexes []int
		docNode.ForEach(func(node *Node, offset, index int) {
			names = append(names, node.Type.Name)
			offsets = append(offsets, offset)
			indexes = append(indexes, index)
		})
		assert.Equal(t, []string{"paragraph", "horizontal_rule", "paragraph"}, names)
		assert.Equal(t, []int{0, 4, 5}, offsets)
		assert.Equal(t, []int{0, 1, 2}, indexes)
	})

	t.Run("reports inline offsets", func(t *testing.T) {
		t.Parallel()
		para := b.p(b.text("foo"), b.img(), b.br(), b.text("bar"))
		var offsets []int
		para.Content.ForEach(func(_ *Node, offset, _ int) {
			offsets = append(offsets, offset)
		})
		assert.Equal(t, []int{0, 3, 4, 5}, offsets)
	})

	t.Run("counts astral plane text as two units in offsets", func(t *testing.T) {
		t.Parallel()
		para := b.p(b.text("a\U0001F600"), b.img())
		var offsets []int
		para.ForEach(func(_ *Node, offset, _ int) {
			offsets = append(offsets, offset)
		})
		assert.Equal(t, []int{0, 3}, offsets)
	})
}

func TestFragmentChild(t *testing.T) {
	t.Parallel()
	b := newNodeTestBuilder(t)
	docNode := b.doc(b.p())

	t.Run("panics out of range", func(t *testing.T) {
		t.Parallel()
		assert.Panics(t, func() { docNode.Content.Child(1) })
		assert.Panics(t, func() { docNode.Content.Child(-1) })
		assert.Panics(t, func() { docNode.Child(1) })
	})

	t.Run("MaybeChild returns nil out of range", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, docNode.Content.MaybeChild(1))
		assert.Nil(t, docNode.Content.MaybeChild(-1))
		assert.NotNil(t, docNode.MaybeChild(0))
	})

	t.Run("returns the child in range", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "paragraph", docNode.Child(0).Type.Name)
	})
}
