// Ported from prosemirror-model/test/test-content.ts.

package model //nolint:testpackage

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contentTestSchema loads the basic schema, an approximation of the prosemirror-test-builder schema used by the TypeScript tests.
func contentTestSchema(t *testing.T) *Schema {
	t.Helper()
	specJSON, err := os.ReadFile("testdata/basic-schema.json")
	require.NoError(t, err)
	schema, errE := NewSchema(specJSON, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	return schema
}

// contentTestBuilder builds nodes the way the prosemirror-test-builder helpers (doc, p, pre, img, br, h1, hr) do in the TypeScript tests. It uses
// NodeType.Create (unchecked content) like those builders, because several cases construct intentionally incomplete documents such as an empty doc.
type contentTestBuilder struct {
	t      *testing.T
	schema *Schema
}

func (b contentTestBuilder) node(typeName string, attrs Attrs, children ...*Node) *Node {
	b.t.Helper()
	typ, errE := b.schema.NodeType(typeName)
	require.NoError(b.t, errE, "% -+#.1v", errE)
	node, errE := typ.Create(attrs, FragmentFromArray(children), nil)
	require.NoError(b.t, errE, "% -+#.1v", errE)
	return node
}

func (b contentTestBuilder) doc(children ...*Node) *Node { return b.node("doc", nil, children...) }

func (b contentTestBuilder) p(children ...*Node) *Node { return b.node("paragraph", nil, children...) }

func (b contentTestBuilder) pre() *Node { return b.node("code_block", nil) }

func (b contentTestBuilder) h1() *Node { return b.node("heading", Attrs{"level": float64(1)}) }

func (b contentTestBuilder) hr() *Node { return b.node("horizontal_rule", nil) }

func (b contentTestBuilder) br() *Node { return b.node("hard_break", nil) }

func (b contentTestBuilder) img() *Node { return b.node("image", Attrs{"src": "img.png"}) }

// contentTestMatch parses the expression and matches the space separated type names against it one by one, reporting whether the resulting state is a
// valid end. It mirrors the match helper of the TypeScript tests.
func contentTestMatch(t *testing.T, schema *Schema, expr, types string) bool {
	t.Helper()
	m, errE := parseContentMatch(expr, schema)
	require.NoError(t, errE, "% -+#.1v", errE)
	if types != "" {
		for name := range strings.SplitSeq(types, " ") {
			if m == nil {
				break
			}
			typ, ok := schema.Nodes[name]
			require.True(t, ok, "unknown node type %q", name)
			m = m.MatchType(typ)
		}
	}
	return m != nil && m.ValidEnd
}

func contentTestTypeNames(types []*NodeType) []string {
	names := make([]string, len(types))
	for i, typ := range types {
		names[i] = typ.Name
	}
	return names
}

func TestContentMatchMatchType(t *testing.T) {
	t.Parallel()
	schema := contentTestSchema(t)

	cases := []struct {
		name  string
		expr  string
		types string
		valid bool
	}{
		{"accepts empty content for the empty expr", "", "", true},
		{"doesn't accept content in the empty expr", "", "image", false},
		{"matches nothing to an asterisk", "image*", "", true},
		{"matches one element to an asterisk", "image*", "image", true},
		{"matches multiple elements to an asterisk", "image*", "image image image image", true}, //nolint:dupword
		{"only matches appropriate elements to an asterisk", "image*", "image text", false},
		{"matches group members to a group", "inline*", "image text", true},
		{"doesn't match non-members to a group", "inline*", "paragraph", false},
		{"matches an element to a choice expression", "(paragraph | heading)", "paragraph", true},
		{"doesn't match unmentioned elements to a choice expr", "(paragraph | heading)", "image", false},
		{"matches a simple sequence", "paragraph horizontal_rule paragraph", "paragraph horizontal_rule paragraph", true},
		{"fails when a sequence is too long", "paragraph horizontal_rule", "paragraph horizontal_rule paragraph", false},
		{"fails when a sequence is too short", "paragraph horizontal_rule paragraph", "paragraph horizontal_rule", false},
		{"fails when a sequence starts incorrectly", "paragraph horizontal_rule", "horizontal_rule paragraph horizontal_rule", false},
		{"accepts a sequence asterisk matching zero elements", "heading paragraph*", "heading", true},
		{"accepts a sequence asterisk matching multiple elts", "heading paragraph*", "heading paragraph paragraph", true}, //nolint:dupword
		{"accepts a sequence plus matching one element", "heading paragraph+", "heading paragraph", true},
		{"accepts a sequence plus matching multiple elts", "heading paragraph+", "heading paragraph paragraph", true}, //nolint:dupword
		{"fails when a sequence plus has no elements", "heading paragraph+", "heading", false},
		{"fails when a sequence plus misses its start", "heading paragraph+", "paragraph paragraph", false}, //nolint:dupword
		{"accepts an optional element being present", "image?", "image", true},
		{"accepts an optional element being missing", "image?", "", true},
		{"fails when an optional element is present twice", "image?", "image image", false},                                                             //nolint:dupword
		{"accepts a nested repeat", "(heading paragraph+)+", "heading paragraph heading paragraph paragraph", true},                                     //nolint:dupword
		{"fails on extra input after a nested repeat", "(heading paragraph+)+", "heading paragraph heading paragraph paragraph horizontal_rule", false}, //nolint:dupword
		{"accepts a matching count", "hard_break{2}", "hard_break hard_break", true},                                                                    //nolint:dupword
		{"rejects a count that comes up short", "hard_break{2}", "hard_break", false},
		{"rejects a count that has too many elements", "hard_break{2}", "hard_break hard_break hard_break", false},      //nolint:dupword
		{"accepts a count on the lower bound", "hard_break{2, 4}", "hard_break hard_break", true},                       //nolint:dupword
		{"accepts a count on the upper bound", "hard_break{2, 4}", "hard_break hard_break hard_break hard_break", true}, //nolint:dupword
		{"accepts a count between the bounds", "hard_break{2, 4}", "hard_break hard_break hard_break", true},            //nolint:dupword
		{"rejects a sequence with too few elements", "hard_break{2, 4}", "hard_break", false},
		{"rejects a sequence with too many elements", "hard_break{2, 4}", "hard_break hard_break hard_break hard_break hard_break", false}, //nolint:dupword
		{"rejects a sequence with a bad element after it", "hard_break{2, 4} text*", "hard_break hard_break image", false},                 //nolint:dupword
		{"accepts a sequence with a matching element after it", "hard_break{2, 4} image?", "hard_break hard_break image", true},            //nolint:dupword
		{"accepts an open range", "hard_break{2,}", "hard_break hard_break", true},                                                         //nolint:dupword
		{"accepts an open range matching many", "hard_break{2,}", "hard_break hard_break hard_break hard_break", true},                     //nolint:dupword
		{"rejects an open range with too few elements", "hard_break{2,}", "hard_break", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.valid {
				assert.True(t, contentTestMatch(t, schema, tc.expr, tc.types))
			} else {
				assert.False(t, contentTestMatch(t, schema, tc.expr, tc.types))
			}
		})
	}
}

func TestParseContentMatchEmptyExpression(t *testing.T) {
	t.Parallel()
	schema := contentTestSchema(t)

	m, errE := parseContentMatch("", schema)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Same(t, EmptyContentMatch, m)
	assert.True(t, m.ValidEnd)

	m, errE = parseContentMatch("   ", schema)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Same(t, EmptyContentMatch, m)
}

func TestContentMatchFillBefore(t *testing.T) {
	t.Parallel()
	schema := contentTestSchema(t)
	b := contentTestBuilder{t: t, schema: schema}

	// A nil result means that no fill exists.
	cases := []struct {
		name   string
		expr   string
		before *Node
		after  *Node
		result *Node
	}{
		{"returns the empty fragment when things match", "paragraph horizontal_rule paragraph", b.doc(b.p(), b.hr()), b.doc(b.p()), b.doc()},
		{"adds a node when necessary", "paragraph horizontal_rule paragraph", b.doc(b.p()), b.doc(b.p()), b.doc(b.hr())},
		{"accepts an asterisk across the bound", "hard_break*", b.p(b.br()), b.p(b.br()), b.p()},
		{"accepts an asterisk only on the left", "hard_break*", b.p(b.br()), b.p(), b.p()},
		{"accepts an asterisk only on the right", "hard_break*", b.p(), b.p(b.br()), b.p()},
		{"accepts an asterisk with no elements", "hard_break*", b.p(), b.p(), b.p()},
		{"accepts a plus across the bound", "hard_break+", b.p(b.br()), b.p(b.br()), b.p()},
		{"adds an element for a content-less plus", "hard_break+", b.p(), b.p(), b.p(b.br())},
		{"fails for a mismatched plus", "hard_break+", b.p(), b.p(b.img()), nil},
		{"accepts asterisk with content on both sides", "heading* paragraph*", b.doc(b.h1()), b.doc(b.p()), b.doc()},
		{"accepts asterisk with no content after", "heading* paragraph*", b.doc(b.h1()), b.doc(), b.doc()},
		{"accepts plus with content on both sides", "heading+ paragraph+", b.doc(b.h1()), b.doc(b.p()), b.doc()},
		{"accepts plus with no content after", "heading+ paragraph+", b.doc(b.h1()), b.doc(), b.doc(b.p())},
		{"adds elements to match a count", "hard_break{3}", b.p(b.br()), b.p(b.br()), b.p(b.br())},
		{"fails when there are too many elements", "hard_break{3}", b.p(b.br(), b.br()), b.p(b.br(), b.br()), nil},
		{"adds elements for two counted groups", "code_block{2} paragraph{2}", b.doc(b.pre()), b.doc(b.p()), b.doc(b.pre(), b.p())},
		{"doesn't include optional elements", "heading paragraph? horizontal_rule", b.doc(b.h1()), b.doc(), b.doc(b.hr())},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, errE := parseContentMatch(tc.expr, schema)
			require.NoError(t, errE, "% -+#.1v", errE)
			matched := m.MatchFragment(tc.before.Content, 0, tc.before.Content.ChildCount())
			require.NotNil(t, matched)
			filled := matched.FillBefore(tc.after.Content, true, 0)
			if tc.result != nil {
				require.NotNil(t, filled)
				assert.True(t, filled.Eq(tc.result.Content), "filled %v, expected %v", filled, tc.result.Content)
			} else {
				assert.Nil(t, filled)
			}
		})
	}
}

// TestContentMatchFillBeforeThreeParts ports the fill3 cases: filling between three pieces of content by composing two FillBefore calls, the first without
// toEnd between before and mid, the second with toEnd between the combined content and after.
func TestContentMatchFillBeforeThreeParts(t *testing.T) {
	t.Parallel()
	schema := contentTestSchema(t)
	b := contentTestBuilder{t: t, schema: schema}

	// A nil left means that no complete fill exists; right is only used when left is non-nil.
	cases := []struct {
		name   string
		expr   string
		before *Node
		mid    *Node
		after  *Node
		left   *Node
		right  *Node
	}{
		{
			"completes a sequence",
			"paragraph horizontal_rule paragraph horizontal_rule paragraph",
			b.doc(b.p()), b.doc(b.p()), b.doc(b.p()), b.doc(b.hr()), b.doc(b.hr()),
		},
		{
			"accepts plus across two bounds",
			"code_block+ paragraph+",
			b.doc(b.pre()), b.doc(b.pre()), b.doc(b.p()), b.doc(), b.doc(),
		},
		{
			"fills a plus from empty input",
			"code_block+ paragraph+",
			b.doc(), b.doc(), b.doc(), b.doc(), b.doc(b.pre(), b.p()),
		},
		{
			"completes a count",
			"code_block{3} paragraph{3}",
			b.doc(b.pre()), b.doc(b.p()), b.doc(), b.doc(b.pre(), b.pre()), b.doc(b.p(), b.p()),
		},
		{
			"fails on non-matching elements",
			"paragraph*",
			b.doc(b.p()), b.doc(b.pre()), b.doc(b.p()), nil, nil,
		},
		{
			"completes a plus across two bounds",
			"paragraph{4}",
			b.doc(b.p()), b.doc(b.p()), b.doc(b.p()), b.doc(), b.doc(b.p()),
		},
		{
			"refuses to complete an overflown count across two bounds",
			"paragraph{2}",
			b.doc(b.p()), b.doc(b.p()), b.doc(b.p()), nil, nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, errE := parseContentMatch(tc.expr, schema)
			require.NoError(t, errE, "% -+#.1v", errE)
			matchedBefore := m.MatchFragment(tc.before.Content, 0, tc.before.Content.ChildCount())
			require.NotNil(t, matchedBefore)
			first := matchedBefore.FillBefore(tc.mid.Content, false, 0)
			var second *Fragment
			if first != nil {
				combined := tc.before.Content.Append(first).Append(tc.mid.Content)
				matchedCombined := m.MatchFragment(combined, 0, combined.ChildCount())
				require.NotNil(t, matchedCombined)
				second = matchedCombined.FillBefore(tc.after.Content, true, 0)
			}
			if tc.left != nil {
				require.NotNil(t, first)
				assert.True(t, first.Eq(tc.left.Content), "first fill %v, expected %v", first, tc.left.Content)
				require.NotNil(t, second)
				assert.True(t, second.Eq(tc.right.Content), "second fill %v, expected %v", second, tc.right.Content)
			} else {
				assert.Nil(t, second)
			}
		})
	}
}

func TestContentMatchFillBeforeStartIndex(t *testing.T) {
	t.Parallel()
	schema := contentTestSchema(t)
	b := contentTestBuilder{t: t, schema: schema}

	t.Run("skips children before the start index", func(t *testing.T) {
		t.Parallel()
		m, errE := parseContentMatch("paragraph horizontal_rule paragraph", schema)
		require.NoError(t, errE, "% -+#.1v", errE)
		// With startIndex 1, only the trailing paragraph of the fragment is matched, so the fill must supply the leading paragraph and horizontal rule.
		after := b.doc(b.hr(), b.p())
		filled := m.FillBefore(after.Content, true, 1)
		require.NotNil(t, filled)
		expected := b.doc(b.p(), b.hr())
		assert.True(t, filled.Eq(expected.Content), "filled %v, expected %v", filled, expected.Content)
	})
}

func TestContentMatchFindWrapping(t *testing.T) {
	t.Parallel()
	schema := contentTestSchema(t)

	docMatch := schema.Nodes["doc"].ContentMatch
	paragraphMatch := schema.Nodes["paragraph"].ContentMatch
	orderedListMatch := schema.Nodes["ordered_list"].ContentMatch

	t.Run("returns an empty result when the type fits directly", func(t *testing.T) {
		t.Parallel()
		wrapping := docMatch.FindWrapping(schema.Nodes["paragraph"])
		require.NotNil(t, wrapping)
		assert.Empty(t, wrapping)
	})

	t.Run("wraps a list item in a list", func(t *testing.T) {
		t.Parallel()
		// ordered_list is declared before bullet_list, so it is the wrapping found first.
		wrapping := docMatch.FindWrapping(schema.Nodes["list_item"])
		require.NotNil(t, wrapping)
		assert.Equal(t, []string{"ordered_list"}, contentTestTypeNames(wrapping))
	})

	t.Run("wraps text in a paragraph", func(t *testing.T) {
		t.Parallel()
		wrapping := docMatch.FindWrapping(schema.Nodes["text"])
		require.NotNil(t, wrapping)
		assert.Equal(t, []string{"paragraph"}, contentTestTypeNames(wrapping))
	})

	t.Run("wraps through multiple levels", func(t *testing.T) {
		t.Parallel()
		wrapping := orderedListMatch.FindWrapping(schema.Nodes["text"])
		require.NotNil(t, wrapping)
		assert.Equal(t, []string{"list_item", "paragraph"}, contentTestTypeNames(wrapping))
	})

	t.Run("returns nil when no wrapping exists", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, paragraphMatch.FindWrapping(schema.Nodes["horizontal_rule"]))
	})

	t.Run("returns the same result on repeated calls", func(t *testing.T) {
		t.Parallel()
		first := docMatch.FindWrapping(schema.Nodes["list_item"])
		second := docMatch.FindWrapping(schema.Nodes["list_item"])
		assert.Equal(t, first, second)
	})
}

func TestParseContentMatchErrors(t *testing.T) {
	t.Parallel()
	schema := contentTestSchema(t)

	cases := []struct {
		name string
		expr string
		err  string
	}{
		{
			"unclosed paren",
			"(paragraph",
			"missing closing paren",
		},
		{
			"unknown node type or group",
			"foo+",
			"no node type or group with this name",
		},
		{
			"mixing inline and block content",
			"paragraph image",
			"mixing inline and block content",
		},
		{
			"trailing text",
			"paragraph)",
			"unexpected trailing text",
		},
		{
			"unexpected token",
			"|paragraph",
			"unexpected token",
		},
		{
			"missing number in a range",
			"paragraph{}",
			"expected a number",
		},
		{
			"non-number in a range",
			"paragraph{two}",
			"expected a number",
		},
		{
			"unclosed range",
			"paragraph{2",
			"unclosed braced range",
		},
		{
			"non-generatable node in a required position",
			"image+",
			"only non-generatable nodes in a required position, see https://prosemirror.net/docs/guide/#generatable",
		},
		{
			"required text node",
			"text",
			"only non-generatable nodes in a required position, see https://prosemirror.net/docs/guide/#generatable",
		},
		{
			"choice of only non-generatable nodes in a required position",
			"(text | image)+",
			"only non-generatable nodes in a required position, see https://prosemirror.net/docs/guide/#generatable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, errE := parseContentMatch(tc.expr, schema)
			assert.EqualError(t, errE, tc.err)
		})
	}
}
