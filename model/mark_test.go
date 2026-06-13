// Ported from prosemirror-model/test/test-mark.ts.

package model //nolint:testpackage

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
)

// markTestCustomSchemaJSON mirrors the custom schema in test-mark.ts: remark is nonexclusive (excludes nothing, not even other instances of itself), user
// excludes every mark including itself, strong excludes the marks in em-group, and em belongs to em-group. The toHTML specs are dummies required by NewSchema.
const markTestCustomSchemaJSON = `{
	"topNode": "doc",
	"nodes": {
		"doc": {"content": "paragraph+"},
		"paragraph": {"content": "text*", "toHTML": {"tag": "p"}},
		"text": {}
	},
	"marks": {
		"remark": {"attrs": {"id": {}}, "excludes": "", "toHTML": {"tag": "span"}},
		"user": {"attrs": {"id": {}}, "excludes": "_", "toHTML": {"tag": "span"}},
		"strong": {"excludes": "em-group", "toHTML": {"tag": "strong"}},
		"em": {"group": "em-group", "toHTML": {"tag": "em"}}
	}
}`

// markTestFixture bundles the marks used by the test-mark.ts cases: em, strong, code, and link come from the basic schema (declared in rank order link, em,
// strong, code), the rest from the custom schema above.
type markTestFixture struct {
	em           *Mark
	strong       *Mark
	code         *Mark
	link         func(href string, title ...string) *Mark
	remark1      *Mark
	remark2      *Mark
	user1        *Mark
	user2        *Mark
	customEm     *Mark
	customStrong *Mark
}

func markTestSetup(t *testing.T) *markTestFixture {
	t.Helper()

	data, err := os.ReadFile("testdata/basic-schema.json")
	require.NoError(t, err)
	basic, errE := NewSchema(data, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	custom, errE := NewSchema([]byte(markTestCustomSchemaJSON), nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	mark := func(s *Schema, name string, attrs Attrs) *Mark {
		m, errE := s.Mark(name, attrs)
		require.NoError(t, errE, "% -+#.1v", errE)
		return m
	}

	return &markTestFixture{
		em:     mark(basic, "em", nil),
		strong: mark(basic, "strong", nil),
		code:   mark(basic, "code", nil),
		link: func(href string, title ...string) *Mark {
			attrs := Attrs{"href": href}
			if len(title) > 0 {
				attrs["title"] = title[0]
			}
			return mark(basic, "link", attrs)
		},
		remark1:      mark(custom, "remark", Attrs{"id": float64(1)}),
		remark2:      mark(custom, "remark", Attrs{"id": float64(2)}),
		user1:        mark(custom, "user", Attrs{"id": float64(1)}),
		user2:        mark(custom, "user", Attrs{"id": float64(2)}),
		customEm:     mark(custom, "em", nil),
		customStrong: mark(custom, "strong", nil),
	}
}

func markTestJSON(t *testing.T, set []*Mark) string {
	t.Helper()
	data, errE := x.MarshalWithoutEscapeHTML(set)
	require.NoError(t, errE, "% -+#.1v", errE)
	return string(data)
}

func TestMarkSameSet(t *testing.T) {
	t.Parallel()
	f := markTestSetup(t)

	cases := []struct {
		name string
		a    []*Mark
		b    []*Mark
		want bool
	}{
		{"returns true for two empty sets", []*Mark{}, []*Mark{}, true},
		{"returns true for simple identical sets", []*Mark{f.em, f.strong}, []*Mark{f.em, f.strong}, true},
		{"returns false for different sets", []*Mark{f.em, f.strong}, []*Mark{f.em, f.code}, false},
		{"returns false when set size differs", []*Mark{f.em, f.strong}, []*Mark{f.em, f.strong, f.code}, false},
		{"recognizes identical links in set", []*Mark{f.link("http://foo"), f.code}, []*Mark{f.link("http://foo"), f.code}, true},
		{"recognizes different links in set", []*Mark{f.link("http://foo"), f.code}, []*Mark{f.link("http://bar"), f.code}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, SameMarkSet(tc.a, tc.b))
		})
	}
}

func TestMarkEq(t *testing.T) {
	t.Parallel()
	f := markTestSetup(t)

	cases := []struct {
		name string
		a    *Mark
		b    *Mark
		want bool
	}{
		{"considers identical links to be the same", f.link("http://foo"), f.link("http://foo"), true},
		{"considers different links to differ", f.link("http://foo"), f.link("http://bar"), false},
		{"considers links with different titles to differ", f.link("http://foo", "A"), f.link("http://foo", "B"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.a.Eq(tc.b))
		})
	}
}

func TestMarkAddToSet(t *testing.T) {
	t.Parallel()
	f := markTestSetup(t)

	cases := []struct {
		name string
		mark *Mark
		set  []*Mark
		want []*Mark
	}{
		{"can add to the empty set", f.em, []*Mark{}, []*Mark{f.em}},
		{"is a no-op when the added thing is in set", f.em, []*Mark{f.em}, []*Mark{f.em}},
		{"adds marks with lower rank before others", f.em, []*Mark{f.strong}, []*Mark{f.em, f.strong}},
		{"adds marks with higher rank after others", f.strong, []*Mark{f.em}, []*Mark{f.em, f.strong}},
		{
			"replaces different marks with new attributes",
			f.link("http://bar"),
			[]*Mark{f.link("http://foo"), f.em},
			[]*Mark{f.link("http://bar"), f.em},
		},
		{
			"does nothing when adding an existing link",
			f.link("http://foo"),
			[]*Mark{f.em, f.link("http://foo")},
			[]*Mark{f.em, f.link("http://foo")},
		},
		{
			"puts code marks at the end",
			f.code,
			[]*Mark{f.em, f.strong, f.link("http://foo")},
			[]*Mark{f.em, f.strong, f.link("http://foo"), f.code},
		},
		{"puts marks with middle rank in the middle", f.strong, []*Mark{f.em, f.code}, []*Mark{f.em, f.strong, f.code}},
		{"allows nonexclusive instances of marks with the same type", f.remark2, []*Mark{f.remark1}, []*Mark{f.remark1, f.remark2}},
		{"doesn't duplicate identical instances of nonexclusive marks", f.remark1, []*Mark{f.remark1}, []*Mark{f.remark1}},
		{"clears all others when adding a globally-excluding mark", f.user1, []*Mark{f.remark1, f.customEm}, []*Mark{f.user1}},
		{"does not allow adding another mark to a globally-excluding mark", f.customEm, []*Mark{f.user1}, []*Mark{f.user1}},
		{"does overwrite a globally-excluding mark when adding another instance", f.user2, []*Mark{f.user1}, []*Mark{f.user2}},
		{
			"doesn't add anything when another mark excludes the added mark",
			f.customEm,
			[]*Mark{f.remark1, f.customStrong},
			[]*Mark{f.remark1, f.customStrong},
		},
		{
			"remove excluded marks when adding a mark",
			f.customStrong,
			[]*Mark{f.remark1, f.customEm},
			[]*Mark{f.remark1, f.customStrong},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.mark.AddToSet(tc.set)
			assert.True(t, SameMarkSet(got, tc.want), "got %s, want %s", markTestJSON(t, got), markTestJSON(t, tc.want))
		})
	}
}

func TestMarkRemoveFromSet(t *testing.T) {
	t.Parallel()
	f := markTestSetup(t)

	cases := []struct {
		name string
		mark *Mark
		set  []*Mark
		want []*Mark
	}{
		{"is a no-op for the empty set", f.em, []*Mark{}, []*Mark{}},
		{"can remove the last mark from a set", f.em, []*Mark{f.em}, []*Mark{}},
		{"is a no-op when the mark isn't in the set", f.strong, []*Mark{f.em}, []*Mark{f.em}},
		{"can remove a mark with attributes", f.link("http://foo"), []*Mark{f.link("http://foo")}, []*Mark{}},
		{
			"doesn't remove a mark when its attrs differ",
			f.link("http://foo", "title"),
			[]*Mark{f.link("http://foo")},
			[]*Mark{f.link("http://foo")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.mark.RemoveFromSet(tc.set)
			assert.True(t, SameMarkSet(got, tc.want), "got %s, want %s", markTestJSON(t, got), markTestJSON(t, tc.want))
		})
	}
}

// TestMarkIsInSet exercises Mark.IsInSet directly. The isInSet coverage in test-mark.ts goes through ResolvedPos.marks, which depends on resolvedpos.ts (not
// ported), so these cases assert the same membership semantics (type identity plus attribute equality) on plain mark sets.
func TestMarkIsInSet(t *testing.T) {
	t.Parallel()
	f := markTestSetup(t)

	cases := []struct {
		name string
		mark *Mark
		set  []*Mark
		want bool
	}{
		{"returns false for the empty set", f.em, []*Mark{}, false},
		{"recognizes a mark in the set", f.em, []*Mark{f.em, f.strong}, true},
		{"does not recognize a mark absent from the set", f.code, []*Mark{f.em, f.strong}, false},
		{"recognizes a mark with equal attributes", f.link("http://foo"), []*Mark{f.em, f.link("http://foo")}, true},
		{"notices that attributes differ", f.link("http://baz"), []*Mark{f.em, f.link("http://foo")}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.mark.IsInSet(tc.set))
		})
	}
}
