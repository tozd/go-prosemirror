// Direct tests of the string level HTML API in html.go, independent of the fixture corpus. Schemas come from the fixtureSchema helper in fixtures_test.go.

package model //nolint:testpackage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalizeHTML(t *testing.T) {
	t.Parallel()
	schema := fixtureSchema(t, "example-schema.json")
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Parsing empty input fills the required doc content with a single empty paragraph (nodeContext.finish calls ContentMatch.FillBefore with an
		// empty fragment), matching the "empty input" fixture case in nodes.json.
		{
			"empty input",
			"",
			"<p></p>",
		},
		{
			"uppercase tags, unquoted attribute, self-closed br",
			`<P>one<BR/>two</P><BLOCKQUOTE CITE=https://example.com/src><P>quoted</P></BLOCKQUOTE>`,
			`<p>one<br>two</p><blockquote cite="https://example.com/src"><p>quoted</p></blockquote>`,
		},
		{
			"raw apostrophe is escaped",
			"<p>it's</p>",
			"<p>it&#39;s</p>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, errE := CanonicalizeHTML(schema, tt.input, ParseOptions{})
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsCanonicalHTML(t *testing.T) {
	t.Parallel()
	schema := fixtureSchema(t, "example-schema.json")
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"canonical empty paragraph", "<p></p>", true},
		{"empty input is not canonical", "", false},
		{"escaped apostrophe is canonical", "<p>it&#39;s</p>", true},
		{"raw apostrophe is not canonical", "<p>it's</p>", false},
		{"uppercase tags are not canonical", "<P>x</P>", false},
		{"self-closed br is not canonical", "<p>a<br/>b</p>", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, errE := IsCanonicalHTML(schema, tt.input, ParseOptions{})
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSerializeHTMLDeterminism(t *testing.T) {
	t.Parallel()
	schema := fixtureSchema(t, "example-schema.json")
	input := `<p><b>bold <i>both</i></b><i> italic</i> <a href="/x">link</a> plain</p><blockquote cite="/src"><p>quoted</p></blockquote>`
	doc, errE := ParseHTML(schema, input, ParseOptions{})
	require.NoError(t, errE, "% -+#.1v", errE)
	first := SerializeHTML(doc)
	second := SerializeHTML(doc)
	assert.Equal(t, first, second)
}

// TestParseHTMLNeverErrors checks the guarantee that ParseHTML repairs malformed HTML instead of returning an error, and that the repaired result is a valid
// document of the schema.
func TestParseHTMLNeverErrors(t *testing.T) {
	t.Parallel()
	schema := fixtureSchema(t, "example-schema.json")
	tests := []struct {
		name  string
		input string
	}{
		{"unclosed tags", "<p><b>unclosed <i>everywhere"},
		{"stray closing tags", "</p>stray</b> text</div>"},
		{"misnested tags", "<b><i>misnested</b></i> tail"},
		{"nested forms", "<form><p>one<form><p>two</form>three</form>"},
		{"null bytes", "a\x00b<p>c\x00d</p>"},
		{"table fragments outside a table", "<td>cell</td><tr><td>row</td></tr>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc, errE := ParseHTML(schema, tt.input, ParseOptions{})
			require.NoError(t, errE, "% -+#.1v", errE)
			require.NotNil(t, doc)
			errE = doc.Check()
			assert.NoError(t, errE, "% -+#.1v", errE)
		})
	}
}
