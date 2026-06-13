//nolint:testpackage
package model

// Tests for behavior on error and edge paths the fixture conformance suite does not reach: schema construction robustness, JavaScript attribute value
// stringification, nested and lowercased HTML serialization, the documented style-matching boundary, foreign-namespace sanitization, debug output
// formatting, and the direct parse options.

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// regressionParseFragment parses an HTML fragment into a fresh div node, the same way ParseHTML does, for tests that drive the DOMParser directly.
func regressionParseFragment(t *testing.T, input string) *html.Node {
	t.Helper()
	div := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"} //nolint:exhaustruct
	nodes, err := html.ParseFragment(strings.NewReader(input), div)
	require.NoError(t, err)
	for _, node := range nodes {
		div.AppendChild(node)
	}
	return div
}

// TestParseOptionsFromToTopNode checks the self-contained parse options: From and To restrict parsing to a child index range of the top DOM node, and
// TopNode parses the content into a different top container type than the schema default.
func TestParseOptionsFromToTopNode(t *testing.T) {
	t.Parallel()

	s := fixtureSchema(t, "basic-schema.json")
	parser, errE := newDOMParser(s, schemaRules(s))
	require.NoError(t, errE, "% -+#.1v", errE)

	div := regressionParseFragment(t, "<p>a</p><p>b</p><p>c</p>")

	one, two := 1, 2
	doc, errE := parser.Parse(div, ParseOptions{From: &one, To: &two}) //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p>b</p>", SerializeHTML(doc))

	doc, errE = parser.Parse(div, ParseOptions{From: &one}) //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p>b</p><p>c</p>", SerializeHTML(doc))

	// TopNode parses the paragraphs into a blockquote container instead of the schema top node.
	top, errE := s.Node("blockquote", nil, []*Node{mustParagraph(t, s)}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	doc, errE = parser.Parse(div, ParseOptions{TopNode: top}) //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)
	// SerializeHTML emits the content of the top node, so the three paragraphs are emitted directly (the blockquote is the container, not a child).
	assert.Equal(t, "<p>a</p><p>b</p><p>c</p>", SerializeHTML(doc))
	assert.Equal(t, "blockquote", doc.Type.Name)

	// The exact prosemirror-model test-dom "accepts from and to options" case: From and To are child indices of the top DOM node, so the leading hr
	// (index 0) and the trailing img (index 3) are skipped.
	div = regressionParseFragment(t, "<hr><p>foo</p><p>bar</p><img>")
	three := 3
	doc, errE = parser.Parse(div, ParseOptions{From: &one, To: &three}) //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p>foo</p><p>bar</p>", SerializeHTML(doc))

	// The prosemirror-model test-dom "accepts the topNode option" case: a bullet_list top node parses the list items directly into the list.
	bulletList, errE := s.Nodes["bullet_list"].CreateAndFill(nil, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	div = regressionParseFragment(t, "<li>wow</li><li>such</li>")
	doc, errE = parser.Parse(div, ParseOptions{TopNode: bulletList}) //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<li><p>wow</p></li><li><p>such</p></li>", SerializeHTML(doc))
	assert.Equal(t, "bullet_list", doc.Type.Name)
}

// TestParseCustomSchemaTopNode checks that a schema whose declared top node (SchemaSpec.TopNode, compiled to Schema.TopNodeType) is not the conventional doc
// drives parsing: the parsed document is an instance of that top node type. It mirrors the prosemirror-model test-dom "uses a custom top node when parsing"
// case. The dialect forbids toHTML on the top node, so the schema uses a minimal blockquote container rather than reusing the basic schema with its
// serializable blockquote. This exercises the parse path of a custom top node, where schema_test.go only checks its construction.
func TestParseCustomSchemaTopNode(t *testing.T) {
	t.Parallel()

	spec := []byte(`{
		"topNode": "blockquote",
		"nodes": {
			"blockquote": {"content": "block+"},
			"paragraph": {"group": "block", "content": "inline*", "toHTML": {"tag": "p"}, "parseHTML": [{"tag": "p"}]},
			"text": {"group": "inline"}
		}
	}`)
	s, errE := NewSchema(spec, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	doc, errE := ParseHTML(s, "<p>hello</p>", ParseOptions{})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "blockquote", doc.Type.Name)
	assert.Equal(t, "<p>hello</p>", SerializeHTML(doc))
}

// TestParseOptionsPreserveWhitespace checks the preserveWhitespace parse option (ParseOptions.PreserveWhitespace), the whole-parse counterpart of the
// per-rule preserveWhitespace. With PreserveWhitespaceTrue runs of whitespace are kept instead of collapsed, but newlines are still normalized to spaces;
// with PreserveWhitespaceFull even a whitespace-only inline node is kept. The fixture suite cannot cover these because it parses every case with default
// options, so they are asserted directly here, like the From/To/TopNode options, mirroring the prosemirror-model test-dom preserveWhitespace cases.
func TestParseOptionsPreserveWhitespace(t *testing.T) {
	t.Parallel()

	s := fixtureSchema(t, "basic-schema.json")
	parser, errE := newDOMParser(s, schemaRules(s))
	require.NoError(t, errE, "% -+#.1v", errE)

	div := regressionParseFragment(t, "foo   bar")

	doc, errE := parser.Parse(div, ParseOptions{PreserveWhitespace: PreserveWhitespaceTrue}) //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p>foo   bar</p>", SerializeHTML(doc))

	// Without the option the default HTML whitespace collapsing applies.
	doc, errE = parser.Parse(div, ParseOptions{})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p>foo bar</p>", SerializeHTML(doc))

	// PreserveWhitespaceTrue keeps runs of spaces but still normalizes newlines to spaces, mirroring the test-dom "normalizes newlines when preserving
	// whitespace" case.
	div = regressionParseFragment(t, "<p>foo  bar\nbaz</p>")
	doc, errE = parser.Parse(div, ParseOptions{PreserveWhitespace: PreserveWhitespaceTrue}) //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p>foo  bar baz</p>", SerializeHTML(doc))

	// PreserveWhitespaceFull keeps even a whitespace-only inline node, mirroring the test-dom "doesn't ignore whitespace-only nodes in preserveWhitespace
	// full mode" case.
	div = regressionParseFragment(t, "<span> </span>x")
	doc, errE = parser.Parse(div, ParseOptions{PreserveWhitespace: PreserveWhitespaceFull}) //nolint:exhaustruct
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p> x</p>", SerializeHTML(doc))
}

// mustParagraph builds an empty paragraph for use as placeholder content.
func mustParagraph(t *testing.T, s *Schema) *Node {
	t.Helper()
	p, errE := s.Node("paragraph", nil, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	return p
}

// TestParseContextRestrictions checks the ParseRule.Context restriction (matched by matchesContext): a rule only applies when the parent nodes into which
// content is being parsed match its context expression. Each case builds a parser with an extra rule mapping a foo element to a horizontal_rule only in the
// given context, so a foo outside that context is dropped instead. It mirrors the prosemirror-model test-dom context restriction cases. The fixture suite
// cannot cover these because it parses every case through the default schema rules, which carry no such extra rule.
func TestParseContextRestrictions(t *testing.T) {
	t.Parallel()

	s := fixtureSchema(t, "basic-schema.json")

	cases := []struct {
		name    string
		context string
		input   string
		want    string
	}{
		{
			"recognizes context restrictions",
			"blockquote/",
			"<foo></foo><blockquote><foo></foo><p><foo></foo></p></blockquote>",
			"<blockquote><hr><p></p></blockquote>",
		},
		{
			"accepts group names in contexts",
			"block/",
			"<foo></foo><blockquote><foo></foo><p></p></blockquote>",
			"<blockquote><hr><p></p></blockquote>",
		},
		{
			"understands nested context restrictions",
			"blockquote/ordered_list//",
			"<foo></foo><blockquote><foo></foo><ol><li><p>a</p><foo></foo></li></ol></blockquote>",
			"<blockquote><ol><li><p>a</p><hr></li></ol></blockquote>",
		},
		{
			"understands double slashes in context restrictions",
			"blockquote//list_item/",
			"<foo></foo><blockquote><foo></foo><ol><foo></foo><li><p>a</p><foo></foo></li></ol></blockquote>",
			"<blockquote><ol><li><p>a</p><hr></li></ol></blockquote>",
		},
		{
			"understands pipes in context restrictions",
			"list_item/|blockquote/",
			"<foo></foo><blockquote><p></p><foo></foo></blockquote><ol><li><p>a</p><foo></foo></li></ol>",
			"<blockquote><p></p><hr></blockquote><ol><li><p>a</p><hr></li></ol>",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rule := &ParseRule{Tag: "foo", Node: "horizontal_rule", Context: c.context} //nolint:exhaustruct
			parser, errE := newDOMParser(s, append([]*ParseRule{rule}, schemaRules(s)...))
			require.NoError(t, errE, "% -+#.1v", errE)
			div := regressionParseFragment(t, c.input)
			doc, errE := parser.Parse(div, ParseOptions{})
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, c.want, SerializeHTML(doc))
		})
	}
}

// TestSchemaRulesOrder checks that schemaRules collects the parse rules in the documented order: mark rules before node rules, each in schema declaration
// order, and, when priorities are given, by decreasing priority with rules of equal priority keeping their order. It mirrors the prosemirror-model test-dom
// schemaRules "defaults to schema order" and "understands priority" cases.
func TestSchemaRulesOrder(t *testing.T) {
	t.Parallel()

	tags := func(t *testing.T, spec string) string {
		t.Helper()
		s, errE := NewSchema([]byte(spec), nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		rules := schemaRules(s)
		names := make([]string, 0, len(rules))
		for _, rule := range rules {
			names = append(names, rule.Tag)
		}
		return strings.Join(names, " ")
	}

	defaultOrder := `{
		"nodes": {
			"doc": {"content": "inline*"},
			"text": {"group": "inline"},
			"foo": {"group": "inline", "inline": true, "toHTML": {"tag": "foo"}, "parseHTML": [{"tag": "foo"}]},
			"bar": {"group": "inline", "inline": true, "toHTML": {"tag": "bar"}, "parseHTML": [{"tag": "bar"}]}
		},
		"marks": {
			"em": {"toHTML": {"tag": "em"}, "parseHTML": [{"tag": "i"}, {"tag": "em"}]}
		}
	}`
	assert.Equal(t, "i em foo bar", tags(t, defaultOrder))

	priority := `{
		"nodes": {
			"doc": {"content": "inline*"},
			"text": {"group": "inline"},
			"foo": {"group": "inline", "inline": true, "toHTML": {"tag": "foo"}, "parseHTML": [{"tag": "foo"}]},
			"bar": {"group": "inline", "inline": true, "toHTML": {"tag": "bar"}, "parseHTML": [{"tag": "bar", "priority": 60}]}
		},
		"marks": {
			"em": {"toHTML": {"tag": "em"}, "parseHTML": [{"tag": "i", "priority": 40}, {"tag": "em", "priority": 70}]}
		}
	}`
	assert.Equal(t, "em bar foo i", tags(t, priority))
}

// TestNewSchemaNullParseRuleEntry checks that a JSON null entry inside a parseHTML array is rejected at NewSchema rather than causing a nil pointer
// dereference. encoding/json decodes a JSON null array element to a nil rule without calling ParseRule.UnmarshalJSON, so the missing-tag guard would
// otherwise be bypassed.
func TestNewSchemaNullParseRuleEntry(t *testing.T) {
	t.Parallel()

	nodeSpec := []byte(`{
		"nodes": {
			"doc": {"content": "paragraph+"},
			"paragraph": {"group": "block", "content": "inline*", "toHTML": {"tag": "p"}, "parseHTML": [null]},
			"text": {"group": "inline"}
		}
	}`)
	_, errE := NewSchema(nodeSpec, nil)
	assert.EqualError(t, errE, "invalid value for key in node spec")
	assert.EqualError(t, errors.Cause(errE), "tag parse rule is missing a tag")

	markSpec := []byte(`{
		"nodes": {
			"doc": {"content": "paragraph+"},
			"paragraph": {"group": "block", "content": "inline*", "toHTML": {"tag": "p"}},
			"text": {"group": "inline"}
		},
		"marks": {
			"bold": {"toHTML": {"tag": "b"}, "parseHTML": [null]}
		}
	}`)
	_, errE = NewSchema(markSpec, nil)
	assert.EqualError(t, errE, "invalid value for key in mark spec")
	assert.EqualError(t, errors.Cause(errE), "tag parse rule is missing a tag")
}

// TestNewSchemaNullSpecObject checks that a JSON null node, mark, or attribute spec object is rejected at NewSchema instead of being coerced into an empty
// spec. The TypeScript reference throws a TypeError on the same input.
func TestNewSchemaNullSpecObject(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		spec    string
		wantErr string
	}{
		"null node spec": {`{"nodes": {"doc": {"content": "text*"}, "text": null}}`, "node spec must be an object"},
		"null mark spec": {`{
			"nodes": {"doc": {"content": "inline*"}, "text": {"group": "inline"}},
			"marks": {"bold": null}
		}`, "mark spec must be an object"},
		"null attribute spec": {`{
			"nodes": {
				"doc": {"content": "inline*"},
				"paragraph": {"group": "block", "content": "inline*", "toHTML": {"tag": "p"}, "attrs": {"id": null}},
				"text": {"group": "inline"}
			}
		}`, "attribute spec must be an object"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, errE := NewSchema([]byte(tc.spec), nil)
			assert.EqualError(t, errE, tc.wantErr)
		})
	}
}

// TestNewSchemaNullAttrsObject checks that an attrs container set to JSON null is accepted (the TypeScript reference guards with "if (attrs)"), so it must
// not be rejected by the stricter spec-object checks.
func TestNewSchemaNullAttrsObject(t *testing.T) {
	t.Parallel()

	spec := []byte(`{
		"nodes": {
			"doc": {"content": "inline*"},
			"paragraph": {"group": "block", "content": "inline*", "toHTML": {"tag": "p"}, "attrs": null},
			"text": {"group": "inline"}
		}
	}`)
	_, errE := NewSchema(spec, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
}

// regressionAttrSchema builds a schema whose box node carries arbitrary attributes emitted both through a tag placeholder and through the attribute list,
// exercising attribute value stringification with value types the dialect permits but the standard schemas do not use.
func regressionAttrSchema(t *testing.T) *Schema {
	t.Helper()
	spec := []byte(`{
		"nodes": {
			"doc": {"content": "box+"},
			"box": {
				"group": "block",
				"content": "text*",
				"attrs": {"data": {"default": null}, "tagval": {"default": null}},
				"toHTML": {"tag": "x-{tagval}", "attrs": ["data"]}
			},
			"text": {"group": "inline"}
		}
	}`)
	s, errE := NewSchema(spec, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	return s
}

// TestSerializeNonScalarAttrValues checks that SerializeHTML stringifies nil, array, and map attribute values like JavaScript String() instead of
// panicking, matching the DOM serializer the canonical form mirrors.
func TestSerializeNonScalarAttrValues(t *testing.T) {
	t.Parallel()

	s := regressionAttrSchema(t)
	cases := []struct {
		name string
		data any
		want string
	}{
		{"nil data attribute is omitted", nil, `<x-null></x-null>`},
		{"array data joins with comma", []any{1.0, 2.0, 3.0}, `<x-null data="1,2,3"></x-null>`},
		{"nested array flattens", []any{1.0, []any{2.0, 3.0}}, `<x-null data="1,2,3"></x-null>`},
		{"array with null element is empty between commas", []any{1.0, nil, 2.0}, `<x-null data="1,,2"></x-null>`},
		{"map renders as object marker", map[string]any{"a": 1.0}, `<x-null data="[object Object]"></x-null>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			box, errE := s.Node("box", Attrs{"data": c.data, "tagval": nil}, nil, nil)
			require.NoError(t, errE, "% -+#.1v", errE)
			doc, errE := s.Node("doc", nil, []*Node{box}, nil)
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, c.want, SerializeHTML(doc))
		})
	}
}

// TestSerializeNestedToHTML checks that a node whose toHTML wraps its content in a nested element (the declarative subset of a nested DOMOutputSpec)
// serializes as the full wrapper chain, with the content placed at the innermost element and the pre newline rule applying only when that innermost
// element is pre.
func TestSerializeNestedToHTML(t *testing.T) {
	t.Parallel()

	spec := []byte(`{
		"nodes": {
			"doc": {"content": "code_block+"},
			"code_block": {
				"content": "text*", "marks": "", "code": true,
				"toHTML": {"tag": "pre", "content": {"tag": "code"}},
				"parseHTML": [{"tag": "pre", "preserveWhitespace": "full"}]
			},
			"text": {"group": "inline"}
		}
	}`)
	s, errE := NewSchema(spec, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	block, errE := s.Node("code_block", nil, []*Node{s.Text("some code", nil)}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	doc, errE := s.Node("doc", nil, []*Node{block}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<pre><code>some code</code></pre>", SerializeHTML(doc))

	// A leading newline is not doubled, because the content is inside the inner code element, not directly after the pre start tag, so the parser does not
	// drop it.
	block2, errE := s.Node("code_block", nil, []*Node{s.Text("\nx", nil)}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	doc2, errE := s.Node("doc", nil, []*Node{block2}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<pre><code>\nx</code></pre>", SerializeHTML(doc2))
	canonical := SerializeHTML(doc2)
	got, errE := CanonicalizeHTML(s, canonical, ParseOptions{})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, canonical, got)
}

// TestSerializeLowercasesTagsAndAttrs checks that the serializer lowercases tag and attribute names, matching the DOM createElement and setAttribute path
// the canonical form is defined against, so a schema declaring upper or mixed case names still produces the canonical lowercase HTML.
func TestSerializeLowercasesTagsAndAttrs(t *testing.T) {
	t.Parallel()

	spec := []byte(`{
		"nodes": {
			"doc": {"content": "box+"},
			"box": {"group": "block", "content": "text*", "attrs": {"dataValue": {"default": null}}, "toHTML": {"tag": "MyBox", "attrs": ["dataValue"]}},
			"text": {"group": "inline"}
		}
	}`)
	s, errE := NewSchema(spec, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	box, errE := s.Node("box", Attrs{"dataValue": "v"}, []*Node{s.Text("x", nil)}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	doc, errE := s.Node("doc", nil, []*Node{box}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, `<mybox datavalue="v">x</mybox>`, SerializeHTML(doc))
}

// TestSerializeTagPlaceholderStringifiesValue checks that a tag placeholder over a non-string attribute value is stringified like JavaScript String(),
// including a null value rendering as "null".
func TestSerializeTagPlaceholderStringifiesValue(t *testing.T) {
	t.Parallel()

	s := regressionAttrSchema(t)
	box, errE := s.Node("box", Attrs{"data": nil, "tagval": 4.0}, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	doc, errE := s.Node("doc", nil, []*Node{box}, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, `<x-4></x-4>`, SerializeHTML(doc))
}

// TestFormatJSNumber checks that float attribute values stringify like JavaScript String(), including exponent notation at the magnitude boundaries and
// negative zero rendering as "0".
func TestFormatJSNumber(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value float64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{1.5, "1.5"},
		{-2, "-2"},
		{100, "100"},
		{0.1, "0.1"},
		{1e20, "100000000000000000000"},
		{1e21, "1e+21"},
		{1e-6, "0.000001"},
		{1e-7, "1e-7"},
		{1e-9, "1e-9"},
		{1e-10, "1e-10"},
		{1.5e21, "1.5e+21"},
		{123456789, "123456789"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, formatJSNumber(c.value))
		})
	}
	// Negative zero (which encoding/json preserves) renders as "0", matching JavaScript String(-0).
	assert.Equal(t, "0", formatJSNumber(math.Copysign(0, -1)))
}

// TestNodeStringJSONStringify checks that the debug string of a text node escapes its content like JavaScript JSON.stringify (used by the TypeScript
// reference), leaving HTML characters raw and escaping C0 control characters as \u00xx.
func TestNodeStringJSONStringify(t *testing.T) {
	t.Parallel()

	s := fixtureSchema(t, "basic-schema.json")
	cases := []struct {
		name string
		text string
		want string
	}{
		{"plain", "plain", `"plain"`},
		{"html characters stay raw", "with <html> & 'q'", `"with <html> & 'q'"`},
		{"tab and newline escaped", "tab\tand\nnewline", `"tab\tand\nnewline"`},
		{"control byte escaped as u00xx", "a\x01b", `"a\u0001b"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			node := s.Text(c.text, nil)
			assert.Equal(t, c.want, node.String())
		})
	}
}

// TestStyleRuleShorthandNotExpanded pins the one documented fidelity boundary of the parser: a style rule matches an inline longhand declaration
// (font-weight: bold), but the same property reached only through a shorthand (font: bold ...) is not matched, because the port does not perform CSSOM
// shorthand expansion. The reference, running in a browser CSSOM, would match the shorthand, so this case cannot be a conformance fixture.
func TestStyleRuleShorthandNotExpanded(t *testing.T) {
	t.Parallel()

	s := fixtureSchema(t, "feature-schema.json")

	longhand, errE := CanonicalizeHTML(s, `<p><span style="font-weight: bold">x</span></p>`, ParseOptions{})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p><b>x</b></p>", longhand)

	shorthand, errE := CanonicalizeHTML(s, `<p><span style="font: bold 12px serif">x</span></p>`, ParseOptions{})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<p>x</p>", shorthand)
}

// TestForeignNamespacedScriptSanitized checks that a foreign-namespaced script or style element is fully ignored, with its body never extracted as text,
// so that for example an svg-namespaced script cannot leak its content. The ignore list is matched by local tag name regardless of namespace.
func TestForeignNamespacedScriptSanitized(t *testing.T) {
	t.Parallel()

	s := fixtureSchema(t, "feature-schema.json")
	out, errE := CanonicalizeHTML(s, "<svg><script>alert(1)</script><style>x{color:red}</style>ok</svg>", ParseOptions{})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "<svg>ok</svg>", out)
	assert.NotContains(t, out, "alert")
	assert.NotContains(t, out, "color")
}

// TestParseContentMatchUnicodeWhitespace checks that the content expression tokenizer treats Unicode whitespace as a separator, mirroring the JavaScript
// "\s" class the reference token stream uses.
func TestParseContentMatchUnicodeWhitespace(t *testing.T) {
	t.Parallel()

	s := fixtureSchema(t, "basic-schema.json")
	// U+00A0 (non-breaking space) and U+2003 (em space), written as escapes, separate the type names exactly like an ASCII space would.
	match, errE := parseContentMatch("paragraph\u00a0paragraph\u2003paragraph", s) //nolint:dupword
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotNil(t, match)
	// The three paragraph tokens form a valid sequence: matching three paragraphs reaches a valid end.
	cur := match
	for range 3 {
		cur = cur.MatchType(s.Nodes["paragraph"])
		require.NotNil(t, cur)
	}
	assert.True(t, cur.ValidEnd)
}
