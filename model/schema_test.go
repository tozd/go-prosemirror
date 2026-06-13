// Tests for schema spec compilation (NewSchema) and the schema level API. There is no single TypeScript counterpart; the cases follow the PORTING.md contract
// (sections schema.go and "Schema JSON dialect") and the error conditions of the TypeScript Schema constructor in prosemirror-model/src/schema.ts.

package model //nolint:testpackage

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
)

// Named validators used by the example schema, following the regex semantics in the PORTING.md section "Schema JSON dialect". Non-string values are always
// invalid.
var (
	schemaTestLinkURLRegexp     = regexp.MustCompile(`(?i)^(?:/(?:[^/]|$)|https?://[^/]|mailto:[^/])`)
	schemaTestResourceURLRegexp = regexp.MustCompile(`(?i)^(?:/(?:[^/]|$)|https?://[^/])`)
)

func schemaTestLinkURL(value any) errors.E {
	s, ok := value.(string)
	if !ok || !schemaTestLinkURLRegexp.MatchString(s) {
		return errors.Errorf("invalid link URL: %v", value)
	}
	return nil
}

func schemaTestResourceURL(value any) errors.E {
	s, ok := value.(string)
	if !ok || !schemaTestResourceURLRegexp.MatchString(s) {
		return errors.Errorf("invalid resource URL: %v", value)
	}
	return nil
}

func schemaTestExampleValidators() map[string]AttrValidator {
	return map[string]AttrValidator{
		"linkURL":     schemaTestLinkURL,
		"resourceURL": schemaTestResourceURL,
	}
}

func schemaTestLoadSchema(t *testing.T, filename string, validators map[string]AttrValidator) *Schema {
	t.Helper()
	specJSON, err := os.ReadFile(filepath.Join("testdata", filename)) //nolint:gosec
	require.NoError(t, err)
	schema, errE := NewSchema(specJSON, validators)
	require.NoError(t, errE, "% -+#.1v", errE)
	return schema
}

func TestNewSchemaExample(t *testing.T) {
	t.Parallel()
	schema := schemaTestLoadSchema(t, "example-schema.json", schemaTestExampleValidators())

	nodeNames := []string{
		"doc", "paragraph", "blockquote", "blockquote_paragraph", "horizontal_rule", "heading", "preformatted",
		"bullet_list", "ordered_list", "list_item", "text", "hard_break",
	}
	markNames := []string{"link", "bold", "italic", "underline", "strikethrough", "monospace"}
	for _, name := range nodeNames {
		require.NotNil(t, schema.Nodes[name], name)
	}
	for _, name := range markNames {
		require.NotNil(t, schema.Marks[name], name)
	}

	t.Run("node and mark presence", func(t *testing.T) {
		t.Parallel()
		assert.Len(t, schema.Nodes, len(nodeNames))
		assert.Len(t, schema.Marks, len(markNames))
		assert.Equal(t, "doc", schema.Spec.TopNode)
		assert.Same(t, schema.Nodes["doc"], schema.TopNodeType)
		assert.Nil(t, schema.LinebreakReplacement)
	})

	t.Run("spec preserves declaration order", func(t *testing.T) {
		t.Parallel()
		specNodeNames := make([]string, 0, len(schema.Spec.Nodes))
		for _, named := range schema.Spec.Nodes {
			specNodeNames = append(specNodeNames, named.Name)
		}
		assert.Equal(t, nodeNames, specNodeNames)
		specMarkNames := make([]string, 0, len(schema.Spec.Marks))
		for _, named := range schema.Spec.Marks {
			specMarkNames = append(specMarkNames, named.Name)
		}
		assert.Equal(t, markNames, specMarkNames)
	})

	t.Run("mark rank follows declaration order", func(t *testing.T) {
		t.Parallel()
		for i, name := range markNames {
			assert.Equal(t, i, schema.Marks[name].Rank, name)
		}
	})

	t.Run("mark allowlists", func(t *testing.T) {
		t.Parallel()
		// blockquote_paragraph lists its allowed marks explicitly and excludes italic.
		bp := schema.Nodes["blockquote_paragraph"]
		require.NotNil(t, bp.MarkSet)
		expected := []*MarkType{
			schema.Marks["link"], schema.Marks["bold"], schema.Marks["underline"], schema.Marks["strikethrough"], schema.Marks["monospace"],
		}
		assert.Equal(t, expected, bp.MarkSet)
		assert.False(t, bp.AllowsMarkType(schema.Marks["italic"]))
		assert.True(t, bp.AllowsMarkType(schema.Marks["bold"]))

		// heading and preformatted declare marks "" and allow none.
		for _, name := range []string{"heading", "preformatted"} {
			markSet := schema.Nodes[name].MarkSet
			require.NotNil(t, markSet, name)
			assert.Empty(t, markSet, name)
			assert.False(t, schema.Nodes[name].AllowsMarkType(schema.Marks["bold"]), name)
		}

		// paragraph has inline content and no marks declaration, which allows all marks through a nil MarkSet.
		assert.Nil(t, schema.Nodes["paragraph"].MarkSet)
		assert.True(t, schema.Nodes["paragraph"].AllowsMarkType(schema.Marks["italic"]))

		// Nodes without inline content allow no marks.
		require.NotNil(t, schema.Nodes["doc"].MarkSet)
		assert.Empty(t, schema.Nodes["doc"].MarkSet)
		require.NotNil(t, schema.Nodes["horizontal_rule"].MarkSet)
		assert.Empty(t, schema.Nodes["horizontal_rule"].MarkSet)
	})

	t.Run("node type properties", func(t *testing.T) {
		t.Parallel()
		doc := schema.Nodes["doc"]
		assert.True(t, doc.IsBlock)
		assert.False(t, doc.IsInline())
		assert.False(t, doc.InlineContent)
		assert.False(t, doc.IsTextblock())
		assert.False(t, doc.IsLeaf())

		paragraph := schema.Nodes["paragraph"]
		assert.True(t, paragraph.IsBlock)
		assert.True(t, paragraph.InlineContent)
		assert.True(t, paragraph.IsTextblock())
		assert.False(t, paragraph.IsLeaf())
		assert.Equal(t, "normal", paragraph.Whitespace())

		text := schema.Nodes["text"]
		assert.True(t, text.IsText)
		assert.False(t, text.IsBlock)
		assert.True(t, text.IsInline())
		assert.True(t, text.IsLeaf())

		hardBreak := schema.Nodes["hard_break"]
		assert.True(t, hardBreak.IsInline())
		assert.True(t, hardBreak.IsLeaf())
		assert.False(t, hardBreak.IsBlock)

		horizontalRule := schema.Nodes["horizontal_rule"]
		assert.True(t, horizontalRule.IsBlock)
		assert.True(t, horizontalRule.IsLeaf())
		assert.False(t, horizontalRule.IsTextblock())

		// preformatted has code true and no whitespace declaration, so whitespace defaults to "pre".
		preformatted := schema.Nodes["preformatted"]
		assert.True(t, preformatted.Spec.Code)
		assert.Equal(t, "pre", preformatted.Whitespace())
	})

	t.Run("attribute compilation", func(t *testing.T) {
		t.Parallel()
		heading := schema.Nodes["heading"]
		assert.Equal(t, Attrs{"level": float64(1)}, heading.DefaultAttrs)
		assert.False(t, heading.HasRequiredAttrs())

		cite := schema.Nodes["blockquote"].Attrs["cite"]
		require.NotNil(t, cite)
		assert.True(t, cite.HasDefault)
		assert.Nil(t, cite.Default)
		assert.Equal(t, "drop", cite.OnInvalid)
		assert.NotNil(t, cite.Validate)

		href := schema.Marks["link"].Attrs["href"]
		require.NotNil(t, href)
		assert.True(t, href.IsRequired())
		assert.Equal(t, "rejectRule", href.OnInvalid)
		assert.NotNil(t, href.Validate)
	})

	t.Run("mark exclusion defaults", func(t *testing.T) {
		t.Parallel()
		bold := schema.Marks["bold"]
		assert.True(t, bold.Excludes(bold))
		assert.False(t, bold.Excludes(schema.Marks["italic"]))
	})
}

func TestNewSchemaBasic(t *testing.T) {
	t.Parallel()
	schema := schemaTestLoadSchema(t, "basic-schema.json", nil)

	nodeNames := []string{
		"doc", "paragraph", "blockquote", "horizontal_rule", "heading", "code_block", "text", "image", "hard_break",
		"ordered_list", "bullet_list", "list_item",
	}
	markNames := []string{"link", "em", "strong", "code"}
	for _, name := range nodeNames {
		require.NotNil(t, schema.Nodes[name], name)
	}
	for _, name := range markNames {
		require.NotNil(t, schema.Marks[name], name)
	}

	t.Run("node and mark presence", func(t *testing.T) {
		t.Parallel()
		assert.Len(t, schema.Nodes, len(nodeNames))
		assert.Len(t, schema.Marks, len(markNames))
		assert.Same(t, schema.Nodes["doc"], schema.TopNodeType)
	})

	t.Run("mark rank follows declaration order", func(t *testing.T) {
		t.Parallel()
		for i, name := range markNames {
			assert.Equal(t, i, schema.Marks[name].Rank, name)
		}
	})

	t.Run("node type properties", func(t *testing.T) {
		t.Parallel()
		image := schema.Nodes["image"]
		assert.True(t, image.IsInline())
		assert.True(t, image.IsLeaf())
		assert.True(t, image.HasRequiredAttrs())
		assert.Nil(t, image.DefaultAttrs)

		codeBlock := schema.Nodes["code_block"]
		assert.Equal(t, "pre", codeBlock.Whitespace())
		require.NotNil(t, codeBlock.MarkSet)
		assert.Empty(t, codeBlock.MarkSet)

		assert.Nil(t, schema.Nodes["paragraph"].MarkSet)
	})
}

func TestNewSchemaSpecErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		spec    string
		wantErr string
	}{
		{
			name: "unknown top-level key",
			spec: `{
				"nodes": {"doc": {"content": "text*"}, "text": {}},
				"bogus": true
			}`,
			wantErr: "unknown key in schema spec",
		},
		{
			name: "unknown node spec key",
			spec: `{
				"nodes": {"doc": {"content": "text*", "bogus": true}, "text": {}}
			}`,
			wantErr: "unknown key in node spec",
		},
		{
			name: "style parse rule on a node rejected",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "text*", "toHTML": {"tag": "p"}, "parseHTML": [{"style": "font-weight"}]},
					"text": {}
				}
			}`,
			wantErr: "style parse rules produce marks and may only appear on mark types",
		},
		{
			name: "unknown attribute spec key",
			spec: `{
				"nodes": {"doc": {"content": "text*", "attrs": {"foo": {"bogus": 1}}}, "text": {}}
			}`,
			wantErr: "unknown key in attribute spec",
		},
		{
			name: "unknown validator name",
			spec: `{
				"nodes": {"doc": {"content": "text*", "attrs": {"foo": {"validate": "uuid"}}}, "text": {}}
			}`,
			wantErr: "unknown validator",
		},
		{
			name: "content expression syntax error",
			spec: `{
				"nodes": {"doc": {"content": "(text*"}, "text": {}}
			}`,
			wantErr: "missing closing paren",
		},
		{
			name: "content expression unknown type or group",
			spec: `{
				"nodes": {"doc": {"content": "blah+"}, "text": {}}
			}`,
			wantErr: "no node type or group with this name",
		},
		{
			name: "content expression mixing inline and block",
			spec: `{
				"nodes": {
					"doc": {"content": "para text*"},
					"para": {"content": "text*", "toHTML": {"tag": "p"}},
					"text": {}
				}
			}`,
			wantErr: "mixing inline and block content",
		},
		{
			name: "missing top node",
			spec: `{
				"nodes": {"para": {"content": "text*", "toHTML": {"tag": "p"}}, "text": {}}
			}`,
			wantErr: "schema is missing its top node type",
		},
		{
			name: "missing named top node",
			spec: `{
				"topNode": "article",
				"nodes": {"doc": {"content": "text*"}, "text": {}}
			}`,
			wantErr: "schema is missing its top node type",
		},
		{
			name: "missing text node",
			spec: `{
				"nodes": {"doc": {"content": "para+"}, "para": {"content": "", "toHTML": {"tag": "p"}}}
			}`,
			wantErr: "schema must have a text type",
		},
		{
			name: "text node with attrs",
			spec: `{
				"nodes": {"doc": {"content": "text*"}, "text": {"attrs": {"foo": {"default": null}}}}
			}`,
			wantErr: "the text node type must not have attributes",
		},
		{
			name: "text node with parseHTML",
			spec: `{
				"nodes": {"doc": {"content": "text*"}, "text": {"parseHTML": [{"tag": "span"}]}}
			}`,
			wantErr: "the text node type must not have parseHTML rules",
		},
		{
			name: "text node with toHTML",
			spec: `{
				"nodes": {"doc": {"content": "text*"}, "text": {"toHTML": {"tag": "span"}}}
			}`,
			wantErr: "node type must not have a toHTML spec",
		},
		{
			name: "toHTML missing on a regular node",
			spec: `{
				"nodes": {"doc": {"content": "para+"}, "para": {"content": "text*"}, "text": {}}
			}`,
			wantErr: "node type must have a toHTML spec",
		},
		{
			name: "toHTML on the top node",
			spec: `{
				"nodes": {"doc": {"content": "text*", "toHTML": {"tag": "div"}}, "text": {}}
			}`,
			wantErr: "node type must not have a toHTML spec",
		},
		{
			name: "toHTML missing on a mark",
			spec: `{
				"nodes": {"doc": {"content": "text*"}, "text": {}},
				"marks": {"bold": {}}
			}`,
			wantErr: "mark type must have a toHTML spec",
		},
		{
			name: "node also defined as mark",
			spec: `{
				"nodes": {
					"doc": {"content": "text*"},
					"text": {},
					"note": {"inline": true, "toHTML": {"tag": "span"}}
				},
				"marks": {"note": {"toHTML": {"tag": "span"}}}
			}`,
			wantErr: "type is both a node and a mark",
		},
		{
			name: "invalid onInvalid value",
			spec: `{
				"nodes": {"doc": {"content": "text*", "attrs": {"foo": {"default": null, "onInvalid": "explode"}}}, "text": {}}
			}`,
			wantErr: "unknown onInvalid value",
		},
		{
			name: "onInvalid drop without default",
			spec: `{
				"nodes": {"doc": {"content": "text*", "attrs": {"foo": {"validate": "string", "onInvalid": "drop"}}}, "text": {}}
			}`,
			wantErr: "onInvalid drop requires a default",
		},
		{
			name: "parse rule references undeclared node attribute",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "text*", "toHTML": {"tag": "p"}, "parseHTML": [{"tag": "p", "attrs": {"align": "align"}}]},
					"text": {}
				}
			}`,
			wantErr: "parse rule references unknown attribute",
		},
		{
			name: "parse rule references undeclared mark attribute",
			spec: `{
				"nodes": {"doc": {"content": "text*"}, "text": {}},
				"marks": {"bold": {"toHTML": {"tag": "b"}, "parseHTML": [{"tag": "b", "attrs": {"weight": "weight"}}]}}
			}`,
			wantErr: "parse rule references unknown attribute",
		},
		{
			name: "invalid CSS selector",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "text*", "toHTML": {"tag": "p"}, "parseHTML": [{"tag": "p["}]},
					"text": {}
				}
			}`,
			wantErr: "unsupported tag selector",
		},
		{
			name: "parse rule with neither tag nor style",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "text*", "toHTML": {"tag": "p"}, "parseHTML": [{"priority": 5}]},
					"text": {}
				}
			}`,
			wantErr: "invalid value for key in node spec",
		},
		{
			name: "duplicate node names",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "text*", "toHTML": {"tag": "p"}},
					"para": {"content": "text*", "toHTML": {"tag": "p"}},
					"text": {}
				}
			}`,
			wantErr: "duplicate key in schema spec",
		},
		{
			name: "linebreakReplacement on a block node",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "text*", "toHTML": {"tag": "p"}},
					"rule": {"group": "block", "linebreakReplacement": true, "toHTML": {"tag": "hr"}},
					"text": {}
				}
			}`,
			wantErr: "linebreak replacement nodes must be inline leaf nodes",
		},
		{
			name: "linebreakReplacement on a non-leaf inline node",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "inline*", "toHTML": {"tag": "p"}},
					"span": {"inline": true, "group": "inline", "content": "text*", "linebreakReplacement": true, "toHTML": {"tag": "span"}},
					"text": {"group": "inline"}
				}
			}`,
			wantErr: "linebreak replacement nodes must be inline leaf nodes",
		},
		{
			name: "linebreakReplacement on two nodes",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "inline*", "toHTML": {"tag": "p"}},
					"br1": {"inline": true, "group": "inline", "linebreakReplacement": true, "toHTML": {"tag": "br"}},
					"br2": {"inline": true, "group": "inline", "linebreakReplacement": true, "toHTML": {"tag": "br"}},
					"text": {"group": "inline"}
				}
			}`,
			wantErr: "multiple linebreak nodes defined",
		},
		{
			name: "unknown mark in a node marks allowlist",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "text*", "marks": "bogus", "toHTML": {"tag": "p"}},
					"text": {}
				}
			}`,
			wantErr: "unknown mark type",
		},
		{
			name: "unknown mark in excludes",
			spec: `{
				"nodes": {"doc": {"content": "text*"}, "text": {}},
				"marks": {"bold": {"excludes": "bogus", "toHTML": {"tag": "b"}}}
			}`,
			wantErr: "unknown mark type",
		},
		{
			name: "invalid whitespace value",
			spec: `{
				"nodes": {
					"doc": {"content": "para+"},
					"para": {"content": "text*", "whitespace": "weird", "toHTML": {"tag": "p"}},
					"text": {}
				}
			}`,
			wantErr: "invalid whitespace value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			schema, errE := NewSchema([]byte(tc.spec), nil)
			assert.Nil(t, schema)
			assert.EqualError(t, errE, tc.wantErr)
		})
	}
}

func TestNewSchemaEditorOnlyKeys(t *testing.T) {
	t.Parallel()
	spec := `{
		"nodes": {
			"doc": {"content": "para+"},
			"para": {
				"content": "text*",
				"selectable": false,
				"draggable": true,
				"defining": true,
				"isolating": false,
				"definingAsContext": true,
				"definingForContent": false,
				"toHTML": {"tag": "p"}
			},
			"text": {}
		},
		"marks": {
			"bold": {"inclusive": false, "toHTML": {"tag": "b"}}
		}
	}`
	schema, errE := NewSchema([]byte(spec), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotNil(t, schema.Nodes["para"])
	assert.NotNil(t, schema.Marks["bold"])
}

const schemaTestWidgetSchemaJSON = `{
	"nodes": {
		"doc": {"content": "widget+"},
		"widget": {
			"attrs": {
				"count": {"default": 0, "validate": "number"},
				"label": {"default": null, "validate": "string|null"},
				"active": {"default": false, "validate": "boolean"}
			},
			"toHTML": {"tag": "div", "attrs": ["count", "label", "active"]}
		},
		"text": {}
	}
}`

func TestNewSchemaBuiltinValidators(t *testing.T) {
	t.Parallel()
	schema, errE := NewSchema([]byte(schemaTestWidgetSchemaJSON), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	widget := schema.Nodes["widget"]
	require.NotNil(t, widget)

	cases := []struct {
		name      string
		doc       string
		wantAttrs Attrs
		wantErr   string
	}{
		{
			name:      "valid values",
			doc:       `{"type": "doc", "content": [{"type": "widget", "attrs": {"count": 3, "label": "x", "active": true}}]}`,
			wantAttrs: Attrs{"count": float64(3), "label": "x", "active": true},
			wantErr:   "",
		},
		{
			name:      "defaults applied",
			doc:       `{"type": "doc", "content": [{"type": "widget"}]}`,
			wantAttrs: Attrs{"count": float64(0), "label": nil, "active": false},
			wantErr:   "",
		},
		{
			name:      "number accepts a fractional value",
			doc:       `{"type": "doc", "content": [{"type": "widget", "attrs": {"count": 2.5, "label": null, "active": false}}]}`,
			wantAttrs: Attrs{"count": 2.5, "label": nil, "active": false},
			wantErr:   "",
		},
		{
			name:      "number rejects a string",
			doc:       `{"type": "doc", "content": [{"type": "widget", "attrs": {"count": "3", "label": null, "active": false}}]}`,
			wantAttrs: nil,
			wantErr:   "unexpected attribute value type",
		},
		{
			name:      "string|null rejects a number",
			doc:       `{"type": "doc", "content": [{"type": "widget", "attrs": {"count": 0, "label": 5, "active": false}}]}`,
			wantAttrs: nil,
			wantErr:   "unexpected attribute value type",
		},
		{
			name:      "boolean rejects null",
			doc:       `{"type": "doc", "content": [{"type": "widget", "attrs": {"count": 0, "label": null, "active": null}}]}`,
			wantAttrs: nil,
			wantErr:   "unexpected attribute value type",
		},
	}
	for _, tc := range cases {
		t.Run("NodeFromJSON "+tc.name, func(t *testing.T) {
			t.Parallel()
			doc, errE := schema.NodeFromJSON([]byte(tc.doc))
			if tc.wantErr != "" {
				assert.Nil(t, doc)
				assert.EqualError(t, errE, tc.wantErr)
				return
			}
			require.NoError(t, errE, "% -+#.1v", errE)
			require.Equal(t, 1, doc.ChildCount())
			assert.Equal(t, tc.wantAttrs, doc.Child(0).Attrs)
		})
	}

	t.Run("Check reports invalid attribute values", func(t *testing.T) {
		t.Parallel()
		node, errE := widget.Create(Attrs{"count": "bad"}, nil, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.EqualError(t, node.Check(), "unexpected attribute value type")
	})

	t.Run("Check passes for defaulted attribute values", func(t *testing.T) {
		t.Parallel()
		node, errE := widget.Create(nil, nil, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		errE = node.Check()
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, Attrs{"count": float64(0), "label": nil, "active": false}, node.Attrs)
	})

	t.Run("CheckAttrs reports a missing declared attribute", func(t *testing.T) {
		t.Parallel()
		errE := widget.CheckAttrs(Attrs{"count": float64(1)})
		assert.EqualError(t, errE, "missing attribute")
	})

	t.Run("CheckAttrs reports an undeclared attribute", func(t *testing.T) {
		t.Parallel()
		errE := widget.CheckAttrs(Attrs{"count": float64(1), "label": nil, "active": true, "extra": float64(1)})
		assert.EqualError(t, errE, "unsupported attribute")
	})
}

func TestParseHTMLOnInvalid(t *testing.T) {
	t.Parallel()
	schema := schemaTestLoadSchema(t, "example-schema.json", schemaTestExampleValidators())

	t.Run("rejectRule drops the link mark but keeps the text", func(t *testing.T) {
		t.Parallel()
		doc, errE := ParseHTML(schema, `<p><a href="javascript:alert(1)">click</a></p>`, ParseOptions{})
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Equal(t, 1, doc.ChildCount())
		para := doc.Child(0)
		assert.Equal(t, "paragraph", para.Type.Name)
		require.Equal(t, 1, para.ChildCount())
		text := para.Child(0)
		assert.True(t, text.IsText())
		assert.Equal(t, "click", text.Text)
		assert.Empty(t, text.Marks)
	})

	t.Run("drop keeps the blockquote without the cite", func(t *testing.T) {
		t.Parallel()
		doc, errE := ParseHTML(schema, `<blockquote cite="javascript:alert(1)"><p>hi</p></blockquote>`, ParseOptions{})
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Equal(t, 1, doc.ChildCount())
		blockquote := doc.Child(0)
		assert.Equal(t, "blockquote", blockquote.Type.Name)
		assert.Equal(t, Attrs{"cite": nil}, blockquote.Attrs)
		require.Equal(t, 1, blockquote.ChildCount())
		assert.Equal(t, "blockquote_paragraph", blockquote.Child(0).Type.Name)
		assert.Equal(t, "hi", blockquote.TextContent())
	})
}

func TestSchemaNode(t *testing.T) {
	t.Parallel()
	example := schemaTestLoadSchema(t, "example-schema.json", schemaTestExampleValidators())
	basic := schemaTestLoadSchema(t, "basic-schema.json", nil)

	t.Run("defaulted attrs", func(t *testing.T) {
		t.Parallel()
		node, errE := example.Node("heading", nil, []*Node{example.Text("hi", nil)}, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, Attrs{"level": float64(1)}, node.Attrs)
	})

	t.Run("missing required attribute", func(t *testing.T) {
		t.Parallel()
		node, errE := basic.Node("image", nil, nil, nil)
		assert.Nil(t, node)
		assert.EqualError(t, errE, "no value supplied for attribute")
	})

	t.Run("explicit attrs completed with defaults", func(t *testing.T) {
		t.Parallel()
		node, errE := basic.Node("image", Attrs{"src": "img.png"}, nil, nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, Attrs{"src": "img.png", "alt": nil, "title": nil}, node.Attrs)
	})

	t.Run("invalid content", func(t *testing.T) {
		t.Parallel()
		node, errE := example.Node("doc", nil, []*Node{example.Text("hi", nil)}, nil)
		assert.Nil(t, node)
		assert.EqualError(t, errE, "invalid content for node")
	})

	t.Run("Create does not check content", func(t *testing.T) {
		t.Parallel()
		// Schema.Node has createChecked semantics; the plain Create accepts the same invalid content.
		node, errE := example.Nodes["doc"].Create(nil, FragmentFromArray([]*Node{example.Text("hi", nil)}), nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.EqualError(t, node.Check(), "invalid content for node")
	})

	t.Run("disallowed marks are invalid content", func(t *testing.T) {
		t.Parallel()
		bold, errE := example.Mark("bold", nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		node, errE := example.Node("heading", nil, []*Node{example.Text("hi", []*Mark{bold})}, nil)
		assert.Nil(t, node)
		assert.EqualError(t, errE, "invalid content for node")
	})

	t.Run("unknown node type", func(t *testing.T) {
		t.Parallel()
		node, errE := example.Node("bogus", nil, nil, nil)
		assert.Nil(t, node)
		assert.EqualError(t, errE, "unknown node type")
	})

	t.Run("creating a text node fails", func(t *testing.T) {
		t.Parallel()
		node, errE := example.Node("text", nil, nil, nil)
		assert.Nil(t, node)
		assert.Error(t, errE)
	})

	t.Run("cannot construct text nodes", func(t *testing.T) {
		t.Parallel()
		node, errE := example.Nodes["text"].Create(nil, nil, nil)
		assert.Nil(t, node)
		assert.EqualError(t, errE, "cannot construct text nodes")
	})
}

func TestSchemaText(t *testing.T) {
	t.Parallel()
	schema := schemaTestLoadSchema(t, "example-schema.json", schemaTestExampleValidators())

	t.Run("plain text", func(t *testing.T) {
		t.Parallel()
		node := schema.Text("hi", nil)
		assert.True(t, node.IsText())
		assert.Equal(t, "hi", node.Text)
		assert.Empty(t, node.Marks)
	})

	t.Run("marks are sorted by rank", func(t *testing.T) {
		t.Parallel()
		bold, errE := schema.Mark("bold", nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		italic, errE := schema.Mark("italic", nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		node := schema.Text("hi", []*Mark{italic, bold})
		require.Len(t, node.Marks, 2)
		assert.Equal(t, "bold", node.Marks[0].Type.Name)
		assert.Equal(t, "italic", node.Marks[1].Type.Name)
	})
}

func TestSchemaMark(t *testing.T) {
	t.Parallel()
	schema := schemaTestLoadSchema(t, "example-schema.json", schemaTestExampleValidators())

	t.Run("mark with attrs", func(t *testing.T) {
		t.Parallel()
		mark, errE := schema.Mark("link", Attrs{"href": "/foo"})
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, Attrs{"href": "/foo"}, mark.Attrs)
	})

	t.Run("missing required attribute", func(t *testing.T) {
		t.Parallel()
		mark, errE := schema.Mark("link", nil)
		assert.Nil(t, mark)
		assert.EqualError(t, errE, "no value supplied for attribute")
	})

	t.Run("unknown mark type", func(t *testing.T) {
		t.Parallel()
		mark, errE := schema.Mark("bogus", nil)
		assert.Nil(t, mark)
		assert.EqualError(t, errE, "unknown mark type")
	})

	t.Run("default instance is reused", func(t *testing.T) {
		t.Parallel()
		first, errE := schema.Mark("bold", nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		second, errE := schema.Mark("bold", nil)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Same(t, first, second)
	})

	t.Run("unknown node type lookup", func(t *testing.T) {
		t.Parallel()
		typ, errE := schema.NodeType("bogus")
		assert.Nil(t, typ)
		assert.EqualError(t, errE, "unknown node type")
	})
}
