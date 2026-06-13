// Ported from prosemirror-model/src/to_dom.ts.

package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gitlab.com/tozd/go/errors"
)

// ToHTMLSpec describes how a node or mark type serializes to HTML: an element with the given tag holding, at the position of the content hole, the
// serialized content of the node or the inline nodes the mark applies to. When Content is set the element wraps a nested element instead, so the content
// hole sits at the innermost spec in the chain (for example a code block serializing as "pre" wrapping "code"). This is the declarative subset of
// ProseMirror's DOMOutputSpec, with attribute values taken from node or mark attributes rather than computed by a function.
type ToHTMLSpec struct {
	// Tag is the HTML tag name. It may contain placeholders of the form "{attrName}" which are substituted with the stringified value of
	// the named attribute (used for heading levels). Read only.
	Tag string
	// Attrs lists the names of attributes emitted on the element, in this order. Attributes whose value is nil are omitted. Read only.
	Attrs []string
	// Content, when set, is a nested element this element wraps; the content hole moves to the innermost element of the chain. When nil the content hole
	// is directly in this element. Read only.
	Content *ToHTMLSpec
}

// UnmarshalJSON implements the json.Unmarshaler interface. The "tag" key is required and must be a non-empty string, the "attrs" key is an optional array
// of strings, the "content" key is an optional nested toHTML spec, and any other key is an error.
func (t *ToHTMLSpec) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return errors.Wrap(err, "invalid toHTML spec")
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return errors.New("toHTML must be an object")
	}
	tag := ""
	var attrs []string
	var content *ToHTMLSpec
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return errors.Wrap(err, "invalid toHTML spec")
		}
		key, _ := token.(string)
		switch key {
		case "tag":
			err = decoder.Decode(&tag)
			if err != nil {
				return errors.Wrap(err, "invalid toHTML spec")
			}
		case "attrs": //nolint:goconst
			err = decoder.Decode(&attrs)
			if err != nil {
				return errors.Wrap(err, "invalid toHTML spec")
			}
		case "content":
			err = decoder.Decode(&content)
			if err != nil {
				return errors.Wrap(err, "invalid toHTML spec")
			}
		default:
			errE := errors.New("unknown toHTML key")
			errors.Details(errE)["key"] = key
			return errE
		}
	}
	if tag == "" {
		return errors.New("toHTML requires a non-empty tag")
	}
	t.Tag = tag
	t.Attrs = attrs
	t.Content = content
	return nil
}

// htmlEscaper escapes the characters which the canonical HTML form escapes in text and attribute values. All other characters, including
// U+00A0, are emitted raw.
var htmlEscaper = strings.NewReplacer( //nolint:gochecknoglobals
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\"", "&#34;",
	"'", "&#39;",
)

// voidElements are the HTML void elements, serialized as a bare open tag with no self-closing slash, no content, and no close tag.
var voidElements = map[string]bool{ //nolint:gochecknoglobals
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

// SerializeHTML serializes the content of the given node to its canonical HTML form (for a doc node that is the document content). The
// output is deterministic, a pure function of the document, with no added whitespace or indentation. Marks spanning adjacent inline nodes
// serialize as a single element, following the mark reconciliation of DOMSerializer.serializeFragment; a mark whose spec sets spanning to
// false closes after every node. Parsing the output with ParseHTML yields back an equal document.
func SerializeHTML(n *Node) string {
	var sb strings.Builder
	serializeFragment(&sb, n.Content)
	return sb.String()
}

// activeMark is an entry of the open mark stack of serializeFragment: a mark and the resolved tags of the element chain opened for it (outermost first).
type activeMark struct {
	mark *Mark
	tags []string
}

// serializeFragment serializes the children of the fragment, opening and closing mark elements so that consecutive nodes carrying the same
// mark share one element. For each child it finds the shared prefix of the active mark stack and the marks of the node (the same mark via
// Eq and the mark spec spanning not false), closes the marks beyond the shared prefix in stack order, opens the missing marks, and then
// serializes the node itself. Marks still open after the last child are closed at the end.
func serializeFragment(sb *strings.Builder, fragment *Fragment) {
	var active []activeMark
	fragment.ForEach(func(node *Node, _, _ int) {
		if len(active) > 0 || len(node.Marks) > 0 {
			keep, rendered := 0, 0
			for keep < len(active) && rendered < len(node.Marks) {
				next := node.Marks[rendered]
				if !next.Eq(active[keep].mark) || (next.Type.Spec.Spanning != nil && !*next.Type.Spec.Spanning) {
					break
				}
				keep++
				rendered++
			}
			for keep < len(active) {
				top := active[len(active)-1]
				active = active[:len(active)-1]
				writeCloseTags(sb, top.tags)
			}
			for rendered < len(node.Marks) {
				add := node.Marks[rendered]
				rendered++
				tags := writeOpenTags(sb, add.Type.Spec.ToHTML, add.Attrs)
				active = append(active, activeMark{mark: add, tags: tags})
			}
		}
		serializeNode(sb, node)
	})
	for i := len(active) - 1; i >= 0; i-- { //nolint:modernize
		writeCloseTags(sb, active[i].tags)
	}
}

// serializeNode serializes a single node: a text node as its escaped text, every other node as an element described by its toHTML spec. The
// node content is serialized into the element with a fresh active mark stack, since marks never span block boundaries.
func serializeNode(sb *strings.Builder, node *Node) {
	if node.IsText() {
		sb.WriteString(htmlEscaper.Replace(node.Text))
		return
	}
	tags := writeOpenTags(sb, node.Type.Spec.ToHTML, node.Attrs)
	innermost := tags[len(tags)-1]
	if voidElements[innermost] {
		return
	}
	// The HTML parser drops one newline immediately after the pre open tag, so a leading newline in the text is doubled to survive a parse round trip.
	// This applies only when the innermost element (the one directly containing the content) is pre, since the parser drops a newline only right after a
	// pre start tag; with a wrapper such as pre>code the content is not directly inside the pre. The first child must be a text node with no marks, since a
	// marked first child serializes wrapped in a mark element.
	if innermost == "pre" { //nolint:goconst
		if first := node.Content.FirstChild(); first != nil && first.IsText() && len(first.Marks) == 0 && strings.HasPrefix(first.Text, "\n") {
			sb.WriteString("\n")
		}
	}
	serializeFragment(sb, node.Content)
	writeCloseTags(sb, tags)
}

// writeOpenTags writes the open tags of the toHTML spec chain from outermost to innermost, returning the resolved tag names in that order so the caller can
// close them in reverse. The content hole is at the innermost spec (the one with no Content).
func writeOpenTags(sb *strings.Builder, spec *ToHTMLSpec, attrs Attrs) []string {
	var tags []string
	for s := spec; s != nil; s = s.Content {
		tags = append(tags, writeOpenTag(sb, s, attrs))
	}
	return tags
}

// writeCloseTags writes the close tags for the given tag chain in reverse order (innermost first), skipping void elements, which have no closing tag.
func writeCloseTags(sb *strings.Builder, tags []string) {
	for i := len(tags) - 1; i >= 0; i-- { //nolint:modernize
		if voidElements[tags[i]] {
			continue
		}
		sb.WriteString("</")
		sb.WriteString(tags[i])
		sb.WriteString(">")
	}
}

// writeOpenTag emits the open tag for the given toHTML spec, with placeholders in the tag resolved against the given attributes and the listed attributes
// emitted in spec order, double-quoted, with nil values omitted. The tag and attribute names are lowercased, matching the DOM createElement and
// setAttribute path the canonical form is defined against (which lowercases both for HTML elements). It returns the resolved tag name.
func writeOpenTag(sb *strings.Builder, spec *ToHTMLSpec, attrs Attrs) string {
	tag := strings.ToLower(resolveTag(spec.Tag, attrs))
	sb.WriteString("<")
	sb.WriteString(tag)
	for _, name := range spec.Attrs {
		value := attrs[name]
		if value == nil {
			continue
		}
		sb.WriteString(" ")
		sb.WriteString(strings.ToLower(name))
		sb.WriteString("=\"")
		sb.WriteString(htmlEscaper.Replace(stringifyAttrValue(value)))
		sb.WriteString("\"")
	}
	sb.WriteString(">")
	return tag
}

// resolveTag substitutes "{attrName}" placeholders in a toHTML tag template with the stringified value of the named attribute.
func resolveTag(tag string, attrs Attrs) string {
	open := strings.IndexByte(tag, '{')
	if open < 0 {
		return tag
	}
	var sb strings.Builder
	for open >= 0 {
		end := strings.IndexByte(tag[open:], '}')
		if end < 0 {
			break
		}
		sb.WriteString(tag[:open])
		sb.WriteString(stringifyAttrValue(attrs[tag[open+1:open+end]]))
		tag = tag[open+end+1:]
		open = strings.IndexByte(tag, '{')
	}
	sb.WriteString(tag)
	return sb.String()
}

// stringifyAttrValue converts a JSON-decoded attribute value to the string form used in tags and attribute values, matching JavaScript String() for every
// JSON value type (the DOM serializer the canonical form mirrors coerces attribute values with String() via setAttribute).
func stringifyAttrValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null" //nolint:goconst
	case string:
		return v
	case float64:
		return formatJSNumber(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []any:
		var sb strings.Builder
		for i, e := range v {
			if i > 0 {
				sb.WriteString(",")
			}
			// A null or undefined array element stringifies to "" in JavaScript String(), so only non-nil elements contribute.
			if e != nil {
				sb.WriteString(stringifyAttrValue(e))
			}
		}
		return sb.String()
	case map[string]any:
		return "[object Object]"
	default:
		errE := errors.New("unsupported attribute value type in toHTML")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", value)
		panic(errE)
	}
}

// formatJSNumber formats a float64 like JavaScript String(): the shortest round-trip decimal form, switching to exponent notation when the magnitude is at
// least 1e21 or below 1e-6, with negative zero rendered as "0".
func formatJSNumber(v float64) string {
	if v == 0 {
		return "0"
	}
	abs := math.Abs(v)
	if abs >= 1e21 || abs < 1e-6 {
		s := strconv.FormatFloat(v, 'e', -1, 64)
		// Go pads the exponent to at least two digits ("1e-07") while JavaScript does not ("1e-7"); strip the padding zeros after the sign.
		i := strings.IndexByte(s, 'e')
		return s[:i+2] + strings.TrimLeft(s[i+2:], "0")
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
