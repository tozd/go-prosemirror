// Ported from prosemirror-model/src/from_dom.ts.

package model

import (
	"bytes"
	"regexp"
	"slices"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// PreserveWhitespace controls how whitespace inside parsed content is handled.
type PreserveWhitespace int

const (
	// PreserveWhitespaceDefault means no explicit value was given: whitespace is collapsed as per HTML rules, except inside node types whose whitespace is "pre".
	PreserveWhitespaceDefault PreserveWhitespace = iota
	// PreserveWhitespaceFalse means whitespace is collapsed as per HTML rules.
	PreserveWhitespaceFalse
	// PreserveWhitespaceTrue means whitespace is preserved, but newlines are normalized to spaces or, if available, linebreak replacement nodes.
	PreserveWhitespaceTrue
	// PreserveWhitespaceFull means whitespace, including newlines, is preserved entirely.
	PreserveWhitespaceFull
)

// UnmarshalJSON implements the json.Unmarshaler interface. Accepted JSON values are false, true, and "full".
func (w *PreserveWhitespace) UnmarshalJSON(data []byte) error {
	var value any
	errE := x.UnmarshalWithoutUnknownFields(data, &value)
	if errE != nil {
		return errE
	}
	switch v := value.(type) {
	case bool:
		if v {
			*w = PreserveWhitespaceTrue
		} else {
			*w = PreserveWhitespaceFalse
		}
		return nil
	case string:
		if v == "full" {
			*w = PreserveWhitespaceFull
			return nil
		}
	}
	errE = errors.New("invalid preserveWhitespace value")
	errors.Details(errE)["value"] = string(data)
	return errE
}

// ParseOptions are the options recognized by the Parse method.
type ParseOptions struct {
	// PreserveWhitespace controls how whitespace is handled. By default, whitespace is collapsed as per HTML rules.
	PreserveWhitespace PreserveWhitespace

	// From, when set, is the child node index of the top DOM node to start parsing at. nil means the first child.
	From *int

	// To, when set, is the child node index of the top DOM node to stop parsing at (exclusive). nil means past the last child.
	To *int

	// TopNode, when set, provides the type and attributes of a node to use as the top container instead of the schema's default top node type.
	TopNode *Node
}

// ParseRule is a parse rule targeting a DOM element (when Tag is set) or an inline style declaration (when Style is set). Exactly one of Tag and Style is
// set. It is the declarative subset of ProseMirror's ParseRule.
type ParseRule struct {
	// Tag is a CSS selector describing the kind of DOM elements to match, for example "p", "a[href]", or "p.MsoNormal". Empty for style rules.
	Tag string

	// Style is an inline CSS declaration to match, either a property name like "font-weight", matching any value of that property, or a property and
	// value like "font-weight=bold", matching only that value. Empty for tag rules. Only the element's own inline style attribute is consulted; shorthand
	// expansion and CSSOM value normalization are not performed.
	Style string

	// Namespace, when set, restricts a tag rule to elements in the given namespace (the full namespace URI, for example "http://www.w3.org/2000/svg").
	// nil means no namespace constraint.
	Namespace *string

	// Node is the name of the node type to create when this rule matches. It is filled in from the position of the rule in the schema spec. Exactly one
	// of Node and Mark is non-empty.
	Node string

	// Mark is the name of the mark type to wrap the matched content in. It is filled in from the position of the rule in the schema spec.
	Mark string

	// Attrs describes the attributes for the node or mark created by this rule. For tag rules a string value names the HTML attribute to extract the value
	// from and any other JSON value is used as a constant; for style rules every value is used as a constant.
	Attrs map[string]any

	// Context, when non-empty, restricts this rule to only match when the current context, the parent nodes into which the content is being parsed,
	// matches this expression. It should contain one or more node names or node group names followed by single or double slashes. For example
	// "paragraph/" means the rule only matches when the parent node is a paragraph, "blockquote/paragraph/" restricts it to be in a paragraph that is
	// inside a blockquote, and "section//" matches any position inside a section, a double slash matches any sequence of ancestor nodes. To allow
	// multiple different contexts, they can be separated by a pipe character, as in "blockquote/|list_item/".
	Context string

	// Consuming controls whether a matching rule prevents later rules from also matching the same element or style. By default (nil or a pointer to true) a
	// matching rule consumes the element or style, so no further rules are tried. A pointer to false indicates that even when this rule matches, the rules
	// after it should also run. It applies to both tag and style rules.
	Consuming *bool

	// Ignore, when true, ignores the element or style that matches this rule, and, for an element, its content as well. The head, noscript, object, script,
	// style, and title tags are ignored automatically when no rule matches them.
	Ignore bool

	// Skip, when true, ignores the element that matches this rule itself, but still parses its content. The element's own inline styles are not read. It
	// applies to tag rules.
	Skip bool

	// CloseParent, when true, closes the current node when an element matching this rule is found. It applies to tag rules.
	CloseParent bool

	// Priority can be used to change the order in which the parse rules in a schema are tried. Those with higher priority come first. nil counts as
	// priority 50.
	Priority *int

	// PreserveWhitespace controls whether whitespace should be preserved when parsing the content inside the matched element.
	PreserveWhitespace PreserveWhitespace

	// selector is the compiled Tag selector, filled in by newDOMParser for tag rules.
	selector cascadia.Selector
	// styleProp and styleValue are the parsed Style property and expected value, filled in by newDOMParser for style rules; hasStyleValue records whether
	// a value was given (otherwise the rule matches any value of the property).
	styleProp     string
	styleValue    string
	hasStyleValue bool
}

// UnmarshalJSON implements the json.Unmarshaler interface. The allowed keys are tag, style, namespace, attrs, context, consuming, ignore, skip, closeParent,
// priority, and preserveWhitespace; exactly one of tag and style must be present, and namespace is only allowed with tag. Unknown keys are errors. The Node and
// Mark fields are filled in by schemaRules, not from JSON.
func (r *ParseRule) UnmarshalJSON(data []byte) error {
	type parseRule struct {
		Tag                *string            `json:"tag"`
		Style              *string            `json:"style"`
		Namespace          *string            `json:"namespace"`
		Attrs              map[string]any     `json:"attrs"`
		Context            string             `json:"context"`
		Consuming          *bool              `json:"consuming"`
		Ignore             bool               `json:"ignore"`
		Skip               bool               `json:"skip"`
		CloseParent        bool               `json:"closeParent"`
		Priority           *int               `json:"priority"`
		PreserveWhitespace PreserveWhitespace `json:"preserveWhitespace"`
	}
	var raw parseRule
	errE := x.DecodeJSONWithoutUnknownFields(bytes.NewReader(data), &raw)
	if errE != nil {
		return errE
	}
	if (raw.Tag != nil) == (raw.Style != nil) {
		return errors.New("parse rule must have exactly one of tag or style")
	}
	if raw.Tag != nil {
		if *raw.Tag == "" {
			return errors.New("tag parse rule is missing a tag")
		}
		r.Tag = *raw.Tag
	} else {
		if *raw.Style == "" {
			return errors.New("style parse rule is missing a style")
		}
		if raw.Namespace != nil {
			return errors.New("style parse rule must not have a namespace")
		}
		r.Style = *raw.Style
	}
	r.Namespace = raw.Namespace
	r.Attrs = raw.Attrs
	r.Context = raw.Context
	r.Consuming = raw.Consuming
	r.Ignore = raw.Ignore
	r.Skip = raw.Skip
	r.CloseParent = raw.CloseParent
	r.Priority = raw.Priority
	r.PreserveWhitespace = raw.PreserveWhitespace
	return nil
}

// DOMParser represents a strategy for parsing DOM content into a ProseMirror document conforming to a given schema. Its behavior is defined by an array
// of rules.
type DOMParser struct {
	// Schema is the schema into which the parser parses.
	Schema *Schema

	// Rules are the set of parse rules that the parser uses, in order of precedence.
	Rules []*ParseRule

	tags           []*ParseRule
	styles         []*ParseRule
	matchedStyles  []string
	normalizeLists bool
}

var listTagSelectorRegexp = regexp.MustCompile(`^(ul|ol)\b`)

// foreignNamespaceShort maps a namespace URI to the short identifier golang.org/x/net/html stores in html.Node.Namespace ("svg", "math", or "" for HTML).
// An already-short or unknown value is returned unchanged.
func foreignNamespaceShort(uri string) string {
	switch uri {
	case "http://www.w3.org/2000/svg":
		return "svg"
	case "http://www.w3.org/1998/Math/MathML":
		return "math"
	case "http://www.w3.org/1999/xhtml":
		return ""
	}
	return uri
}

// newDOMParser creates a parser that targets the given schema, using the given parsing rules. It compiles the CSS selector of every tag rule and parses
// the declaration of every style rule.
func newDOMParser(schema *Schema, rules []*ParseRule) (*DOMParser, errors.E) {
	p := &DOMParser{Schema: schema, Rules: rules} //nolint:exhaustruct
	for _, rule := range rules {
		if rule.Style != "" {
			prop, value, found := strings.Cut(rule.Style, "=")
			rule.styleProp = prop
			rule.styleValue = value
			rule.hasStyleValue = found
			if !slices.Contains(p.matchedStyles, prop) {
				p.matchedStyles = append(p.matchedStyles, prop)
			}
			p.styles = append(p.styles, rule)
			continue
		}
		selector, err := cascadia.Compile(rule.Tag)
		if err != nil {
			errE := errors.Wrap(err, "unsupported tag selector")
			errors.Details(errE)["selector"] = rule.Tag
			return nil, errE
		}
		rule.selector = selector
		p.tags = append(p.tags, rule)
	}

	// Only normalize list elements when lists in the schema cannot directly contain themselves.
	p.normalizeLists = true
	for _, rule := range p.tags {
		if !listTagSelectorRegexp.MatchString(rule.Tag) || rule.Node == "" {
			continue
		}
		node := schema.Nodes[rule.Node]
		if node.ContentMatch.MatchType(node) != nil {
			p.normalizeLists = false
			break
		}
	}
	return p, nil
}

// Parse parses a document from the content of a DOM node.
func (p *DOMParser) Parse(dom *html.Node, options ParseOptions) (*Node, errors.E) {
	context := newParseContext(p, options, false)
	errE := context.addAll(dom, NoMarks, options.From, options.To)
	if errE != nil {
		return nil, errE
	}
	result, errE := context.finish()
	if errE != nil {
		return nil, errE
	}
	return result.(*Node), nil //nolint:errcheck,forcetypeassert
}

// matchTag finds the first tag rule after the given one (or from the start when after is nil) matching the given element, returning the rule and the
// attributes computed for the match (nil when the rule extracts nothing and has no constant attributes).
func (p *DOMParser) matchTag(dom *html.Node, cx *parseContext, after *ParseRule) (*ParseRule, Attrs) {
	start := 0
	if after != nil {
		for i, rule := range p.tags {
			if rule == after {
				start = i + 1
				break
			}
		}
	}
	for i := start; i < len(p.tags); i++ {
		rule := p.tags[i]
		if !rule.selector.Match(dom) {
			continue
		}
		if rule.Namespace != nil && foreignNamespaceShort(*rule.Namespace) != dom.Namespace {
			continue
		}
		if rule.Context != "" && !cx.matchesContext(rule.Context) {
			continue
		}
		attrs, ok := p.matchedRuleAttrs(dom, rule)
		if !ok {
			continue
		}
		return rule, attrs
	}
	return nil, nil
}

// matchStyle finds the first style rule after the given one (or from the start when after is nil) matching the given inline style property and value,
// returning the rule and the constant attributes it carries. A style rule matches when its property equals prop and, when it specifies a value, that value
// equals value.
func (p *DOMParser) matchStyle(prop, value string, cx *parseContext, after *ParseRule) (*ParseRule, Attrs) {
	start := 0
	if after != nil {
		for i, rule := range p.styles {
			if rule == after {
				start = i + 1
				break
			}
		}
	}
	for i := start; i < len(p.styles); i++ {
		rule := p.styles[i]
		if rule.styleProp != prop {
			continue
		}
		if rule.hasStyleValue && rule.styleValue != value {
			continue
		}
		if rule.Context != "" && !cx.matchesContext(rule.Context) {
			continue
		}
		var attrs Attrs
		for name, constant := range rule.Attrs {
			if attrs == nil {
				attrs = Attrs{}
			}
			attrs[name] = constant
		}
		return rule, attrs
	}
	return nil, nil
}

// matchedRuleAttrs computes the attributes for a match of the given rule against the given element. Constants are taken as-is; extracted attributes read
// the HTML attribute, falling back to the attribute default when absent (the rule is rejected when the attribute is required); when a validator is
// configured and fails, OnInvalid "rejectRule" rejects the rule and "drop" replaces the value with the default. A rule that targets neither a node nor a mark
// (an ignore rule) has no attributes to compute.
func (p *DOMParser) matchedRuleAttrs(dom *html.Node, rule *ParseRule) (Attrs, bool) {
	var attributes map[string]*Attribute
	switch {
	case rule.Node != "":
		attributes = p.Schema.Nodes[rule.Node].Attrs
	case rule.Mark != "":
		attributes = p.Schema.Marks[rule.Mark].Attrs
	default:
		return nil, true
	}
	var result Attrs
	set := func(name string, value any) {
		if result == nil {
			result = Attrs{}
		}
		result[name] = value
	}
	for name, value := range rule.Attrs {
		htmlAttr, isString := value.(string)
		if !isString {
			set(name, value)
			continue
		}
		attribute := attributes[name]
		domValue, present := getAttr(dom, htmlAttr)
		if !present {
			if attribute.IsRequired() {
				return nil, false
			}
			set(name, attribute.Default)
			continue
		}
		if attribute.Validate != nil {
			errE := attribute.Validate(domValue)
			if errE != nil {
				if attribute.OnInvalid == "drop" { //nolint:goconst
					set(name, attribute.Default)
					continue
				}
				return nil, false
			}
		}
		set(name, domValue)
	}
	return result, true
}

// readStyles runs any style rules associated with the element's inline style attribute, returning the given marks extended with the marks the matching style
// rules produce. The boolean result is false when a matching style rule has Ignore set, which signals that the element should be ignored: its content is then
// not parsed. A style rule whose Consuming is a pointer to false does not stop the search, so further style rules for the same property are also tried. Only the
// element's own inline style declarations are read; shorthand expansion and CSSOM value normalization are not performed.
func (cx *parseContext) readStyles(dom *html.Node, marks []*Mark) ([]*Mark, bool, errors.E) {
	if len(cx.parser.matchedStyles) == 0 {
		return marks, true, nil
	}
	style, ok := getAttr(dom, "style")
	if !ok || style == "" {
		return marks, true, nil
	}
	declarations := parseInlineStyle(style)
	for _, name := range cx.parser.matchedStyles {
		value, present := declarations[name]
		if !present || value == "" {
			continue
		}
		var after *ParseRule
		for {
			rule, attrs := cx.parser.matchStyle(name, value, cx, after)
			if rule == nil {
				break
			}
			if rule.Ignore {
				return nil, false, nil
			}
			mark, errE := cx.parser.Schema.Marks[rule.Mark].Create(attrs)
			if errE != nil {
				return nil, false, errE
			}
			marks = append(marks[:len(marks):len(marks)], mark)
			if rule.Consuming != nil && !*rule.Consuming {
				after = rule
			} else {
				break
			}
		}
	}
	return marks, true, nil
}

// parseInlineStyle parses an inline CSS style attribute into a map from property name (lowercased) to value (trimmed). When a property is declared more
// than once the last declaration wins, matching the CSSOM. Values are taken verbatim, not normalized.
func parseInlineStyle(style string) map[string]string {
	result := map[string]string{}
	parser := css.NewParser(parse.NewInput(strings.NewReader(style)), true)
	for {
		grammar, _, data := parser.Next()
		if grammar == css.ErrorGrammar {
			return result
		}
		if grammar != css.DeclarationGrammar {
			continue
		}
		var value strings.Builder
		for _, token := range parser.Values() {
			value.Write(token.Data)
		}
		result[strings.ToLower(string(data))] = strings.TrimSpace(value.String())
	}
}

// schemaRules collects the parse rules listed in the node and mark specs of a schema, ordered by decreasing priority (nil counting as 50), with rules of
// equal priority keeping their iteration order: mark rules before node rules, each in schema declaration order. Each rule is copied and its Node or Mark
// field is filled with the name of the owning type, except for rules with Ignore set, which target neither a node nor a mark.
func schemaRules(schema *Schema) []*ParseRule {
	var result []*ParseRule
	insert := func(rule *ParseRule) {
		priority := 50
		if rule.Priority != nil {
			priority = *rule.Priority
		}
		i := 0
		for ; i < len(result); i++ {
			next := result[i]
			nextPriority := 50
			if next.Priority != nil {
				nextPriority = *next.Priority
			}
			if nextPriority < priority {
				break
			}
		}
		result = append(result, nil)
		copy(result[i+1:], result[i:])
		result[i] = rule
	}

	for _, name := range schema.markOrder {
		for _, rule := range schema.Marks[name].Spec.ParseHTML {
			copied := *rule
			insert(&copied)
			if copied.Mark == "" && !copied.Ignore {
				copied.Mark = name
			}
		}
	}
	for _, name := range schema.nodeOrder {
		for _, rule := range schema.Nodes[name].Spec.ParseHTML {
			copied := *rule
			insert(&copied)
			if copied.Node == "" && copied.Mark == "" && !copied.Ignore {
				copied.Node = name
			}
		}
	}
	return result
}

var blockTags = map[string]bool{ //nolint:gochecknoglobals
	"address": true, "article": true, "aside": true, "blockquote": true, "canvas": true,
	"dd": true, "div": true, "dl": true, "fieldset": true, "figcaption": true, "figure": true, //nolint:goconst
	"footer": true, "form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "header": true, "hgroup": true, "hr": true, "li": true, "noscript": true, "ol": true,
	"output": true, "p": true, "pre": true, "section": true, "table": true, "tfoot": true, "ul": true, //nolint:goconst
}

var ignoreTags = map[string]bool{ //nolint:gochecknoglobals
	"head": true, "noscript": true, "object": true, "script": true, "style": true, "title": true,
}

var listTags = map[string]bool{"ol": true, "ul": true} //nolint:gochecknoglobals

// Node context options are a bitfield.
const (
	optPreserveWS     = 1
	optPreserveWSFull = 2
	optOpenLeft       = 4
)

var (
	notWhitespaceRegexp         = regexp.MustCompile(`[^ \t\r\n\f]`)
	whitespaceRunRegexp         = regexp.MustCompile(`[ \t\r\n\f]+`)
	leadingWhitespaceRegexp     = regexp.MustCompile(`^[ \t\r\n\f]`)
	trailingWhitespaceRegexp    = regexp.MustCompile(`[ \t\r\n\f]$`)
	trailingWhitespaceRunRegexp = regexp.MustCompile(`[ \t\r\n\f]+$`)
	carriageReturnRegexp        = regexp.MustCompile(`\r\n?`)
	newlineRegexp               = regexp.MustCompile(`[\r\n]`)
	newlineSplitRegexp          = regexp.MustCompile(`\r?\n|\r`)
	contextPipeRegexp           = regexp.MustCompile(`\s*\|\s*`)
	// jsNonWhitespaceRegexp matches what the JavaScript regexp class "\S" matches: any character which is not JavaScript whitespace (which includes
	// several Unicode space characters beyond the ASCII ones).
	jsNonWhitespaceRegexp = regexp.MustCompile(`[^\t\n\v\f\r \x{00A0}\x{1680}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}\x{FEFF}]`)
)

func wsOptionsFor(typ *NodeType, preserveWhitespace PreserveWhitespace, base int) int {
	if preserveWhitespace != PreserveWhitespaceDefault {
		options := 0
		if preserveWhitespace == PreserveWhitespaceTrue || preserveWhitespace == PreserveWhitespaceFull {
			options |= optPreserveWS
		}
		if preserveWhitespace == PreserveWhitespaceFull {
			options |= optPreserveWSFull
		}
		return options
	}
	if typ != nil && typ.Whitespace() == "pre" {
		return optPreserveWS | optPreserveWSFull
	}
	return base &^ optOpenLeft
}

// nodeContext tracks one open node while parsing.
type nodeContext struct {
	typ     *NodeType
	attrs   Attrs
	marks   []*Mark
	solid   bool
	match   *ContentMatch
	options int
	content []*Node
}

func newNodeContext(typ *NodeType, attrs Attrs, marks []*Mark, solid bool, match *ContentMatch, options int) *nodeContext {
	if match == nil && options&optOpenLeft == 0 {
		match = typ.ContentMatch
	}
	return &nodeContext{typ: typ, attrs: attrs, marks: marks, solid: solid, match: match, options: options} //nolint:exhaustruct
}

func (nc *nodeContext) findWrapping(node *Node) []*NodeType {
	if nc.match == nil {
		if nc.typ == nil {
			return []*NodeType{}
		}
		fill := nc.typ.ContentMatch.FillBefore(FragmentFromArray([]*Node{node}), false, 0)
		if fill != nil {
			nc.match = nc.typ.ContentMatch.MatchFragment(fill, 0, fill.ChildCount())
		} else {
			start := nc.typ.ContentMatch
			wrap := start.FindWrapping(node.Type)
			if wrap != nil {
				nc.match = start
				return wrap
			}
			return nil
		}
	}
	return nc.match.FindWrapping(node.Type)
}

// finish closes this context, returning a *Node when the context has a type and a *Fragment otherwise.
func (nc *nodeContext) finish(openEnd bool) (any, errors.E) {
	if nc.options&optPreserveWS == 0 { // Strip trailing whitespace.
		if last := len(nc.content) - 1; last >= 0 && nc.content[last].IsText() {
			if m := trailingWhitespaceRunRegexp.FindString(nc.content[last].Text); m != "" {
				text := nc.content[last]
				if len(text.Text) == len(m) {
					nc.content = nc.content[:last]
				} else {
					nc.content[last] = text.WithText(text.Text[:len(text.Text)-len(m)])
				}
			}
		}
	}
	content := FragmentFromArray(nc.content)
	if !openEnd && nc.match != nil {
		fill := nc.match.FillBefore(EmptyFragment, true, 0)
		if fill == nil {
			panic("unreachable: no fill exists for a node closed by the parser")
		}
		content = content.Append(fill)
	}
	if nc.typ == nil {
		return content, nil
	}
	node, errE := nc.typ.Create(nc.attrs, content, nc.marks)
	if errE != nil {
		return nil, errE
	}
	return node, nil
}

func (nc *nodeContext) inlineContext(node *html.Node) bool {
	if nc.typ != nil {
		return nc.typ.InlineContent
	}
	if len(nc.content) > 0 {
		return nc.content[0].IsInline()
	}
	return node.Parent != nil && !blockTags[nodeName(node.Parent)]
}

type parseContext struct {
	open            int
	needsBlock      bool
	nodes           []*nodeContext
	localPreserveWS bool
	isOpen          bool
	parser          *DOMParser
	options         ParseOptions
}

func newParseContext(parser *DOMParser, options ParseOptions, isOpen bool) *parseContext {
	topOptions := wsOptionsFor(nil, options.PreserveWhitespace, 0)
	if isOpen {
		topOptions |= optOpenLeft
	}
	var topContext *nodeContext
	switch {
	case options.TopNode != nil:
		topContext = newNodeContext(options.TopNode.Type, options.TopNode.Attrs, NoMarks, true, options.TopNode.Type.ContentMatch, topOptions)
	case isOpen:
		topContext = newNodeContext(nil, nil, NoMarks, true, nil, topOptions)
	default:
		topContext = newNodeContext(parser.Schema.TopNodeType, nil, NoMarks, true, nil, topOptions)
	}
	return &parseContext{
		open:            0,
		needsBlock:      false,
		nodes:           []*nodeContext{topContext},
		localPreserveWS: false,
		isOpen:          isOpen,
		parser:          parser,
		options:         options,
	}
}

func (cx *parseContext) top() *nodeContext {
	return cx.nodes[cx.open]
}

// addDOM adds a DOM node to the content. Text is inserted as a text node and elements are passed to addElement; all other kinds of nodes (comments,
// doctypes) are ignored.
func (cx *parseContext) addDOM(dom *html.Node, marks []*Mark) errors.E {
	switch dom.Type { //nolint:exhaustive
	case html.TextNode:
		return cx.addTextNode(dom, marks)
	case html.ElementNode:
		return cx.addElement(dom, marks, nil)
	default:
		return nil
	}
}

func (cx *parseContext) addTextNode(dom *html.Node, marks []*Mark) errors.E {
	value := dom.Data
	top := cx.top()
	preserveWSFull := top.options&optPreserveWSFull != 0
	preserveWS := preserveWSFull || cx.localPreserveWS || top.options&optPreserveWS != 0
	schema := cx.parser.Schema
	if preserveWSFull || top.inlineContext(dom) || notWhitespaceRegexp.MatchString(value) { //nolint:nestif
		if !preserveWS {
			value = whitespaceRunRegexp.ReplaceAllString(value, " ")
			// If this starts with whitespace, and there is no node before it, or a hard break, or a text node that ends with whitespace, strip the
			// leading space.
			if leadingWhitespaceRegexp.MatchString(value) && cx.open == len(cx.nodes)-1 {
				var nodeBefore *Node
				if len(top.content) > 0 {
					nodeBefore = top.content[len(top.content)-1]
				}
				domNodeBefore := dom.PrevSibling
				if nodeBefore == nil ||
					(domNodeBefore != nil && domNodeBefore.DataAtom == atom.Br) ||
					(nodeBefore.IsText() && trailingWhitespaceRegexp.MatchString(nodeBefore.Text)) {
					value = value[1:]
				}
			}
		} else if preserveWSFull {
			value = carriageReturnRegexp.ReplaceAllString(value, "\n")
		} else {
			replaced := false
			if schema.LinebreakReplacement != nil && newlineRegexp.MatchString(value) {
				linebreak, errE := schema.LinebreakReplacement.Create(nil, nil, nil)
				if errE != nil {
					return errE
				}
				if cx.top().findWrapping(linebreak) != nil {
					lines := newlineSplitRegexp.Split(value, -1)
					for i, line := range lines {
						if i > 0 {
							lineBreakNode, errE := schema.LinebreakReplacement.Create(nil, nil, nil)
							if errE != nil {
								return errE
							}
							_, errE = cx.insertNode(lineBreakNode, marks, true)
							if errE != nil {
								return errE
							}
						}
						if line != "" {
							_, errE := cx.insertNode(schema.Text(line, nil), marks, !jsNonWhitespaceRegexp.MatchString(line))
							if errE != nil {
								return errE
							}
						}
					}
					value = ""
					replaced = true
				}
			}
			if !replaced {
				value = newlineSplitRegexp.ReplaceAllString(value, " ")
			}
		}
		if value != "" {
			_, errE := cx.insertNode(schema.Text(value, nil), marks, !jsNonWhitespaceRegexp.MatchString(value))
			if errE != nil {
				return errE
			}
		}
	}
	return nil
}

// addElement tries to find a handler for the given tag and uses that to parse. If none is found, the element's content nodes are added directly. A matching
// rule may ignore the element (and its content), skip the element but parse its content, or close the current node before parsing.
func (cx *parseContext) addElement(dom *html.Node, marks []*Mark, matchAfter *ParseRule) errors.E {
	outerWS := cx.localPreserveWS
	defer func() { cx.localPreserveWS = outerWS }()
	top := cx.top()
	name := nodeName(dom)
	if name == "pre" || styleWhiteSpacePre(dom) {
		cx.localPreserveWS = true
	}
	if listTags[name] && cx.parser.normalizeLists {
		normalizeList(dom)
	}
	rule, ruleAttrs := cx.parser.matchTag(dom, cx, matchAfter)
	ignore := ignoreTags[name]
	if rule != nil {
		ignore = rule.Ignore
	}
	switch {
	case ignore:
		return cx.ignoreFallback(dom, marks)
	case rule == nil || rule.Skip || rule.CloseParent:
		if rule != nil && rule.CloseParent {
			cx.open = max(0, cx.open-1)
		}
		sync := false
		oldNeedsBlock := cx.needsBlock
		if blockTags[name] {
			if len(top.content) > 0 && top.content[0].IsInline() && cx.open > 0 {
				cx.open--
				top = cx.top()
			}
			sync = true
			if top.typ == nil {
				cx.needsBlock = true
			}
		} else if dom.FirstChild == nil {
			return cx.leafFallback(dom, marks)
		}
		innerMarks, ok := marks, true
		if rule == nil || !rule.Skip {
			var errE errors.E
			innerMarks, ok, errE = cx.readStyles(dom, marks)
			if errE != nil {
				return errE
			}
		}
		if ok {
			errE := cx.addAll(dom, innerMarks, nil, nil)
			if errE != nil {
				return errE
			}
		}
		if sync {
			cx.sync(top)
		}
		cx.needsBlock = oldNeedsBlock
		return nil
	default:
		innerMarks, ok, errE := cx.readStyles(dom, marks)
		if errE != nil {
			return errE
		}
		if !ok {
			return nil
		}
		var continueAfter *ParseRule
		if rule.Consuming != nil && !*rule.Consuming {
			continueAfter = rule
		}
		return cx.addElementByRule(dom, rule, ruleAttrs, innerMarks, continueAfter)
	}
}

// leafFallback is called for leaf DOM nodes that would otherwise be ignored.
func (cx *parseContext) leafFallback(dom *html.Node, marks []*Mark) errors.E {
	if dom.DataAtom == atom.Br && cx.top().typ != nil && cx.top().typ.InlineContent {
		return cx.addTextNode(&html.Node{Type: html.TextNode, Data: "\n"}, marks) //nolint:exhaustruct
	}
	return nil
}

// ignoreFallback is called for ignored nodes.
func (cx *parseContext) ignoreFallback(dom *html.Node, marks []*Mark) errors.E {
	// Ignored BR nodes should at least create an inline context.
	if dom.DataAtom == atom.Br && (cx.top().typ == nil || !cx.top().typ.InlineContent) {
		_, _, errE := cx.findPlace(cx.parser.Schema.Text("-", nil), marks, true)
		return errE
	}
	return nil
}

// addElementByRule applies the given rule to the element, using its result to drive the way the element's content is wrapped.
func (cx *parseContext) addElementByRule(dom *html.Node, rule *ParseRule, ruleAttrs Attrs, marks []*Mark, continueAfter *ParseRule) errors.E {
	sync := false
	var nodeType *NodeType
	if rule.Node != "" { //nolint:nestif
		nodeType = cx.parser.Schema.Nodes[rule.Node]
		if !nodeType.IsLeaf() {
			inner, ok, errE := cx.enter(nodeType, ruleAttrs, marks, rule.PreserveWhitespace)
			if errE != nil {
				return errE
			}
			if ok {
				sync = true
				marks = inner
			}
		} else {
			leaf, errE := nodeType.Create(ruleAttrs, nil, nil)
			if errE != nil {
				return errE
			}
			inserted, errE := cx.insertNode(leaf, marks, dom.DataAtom == atom.Br)
			if errE != nil {
				return errE
			}
			if !inserted {
				errE = cx.leafFallback(dom, marks)
				if errE != nil {
					return errE
				}
			}
		}
	} else {
		markType := cx.parser.Schema.Marks[rule.Mark]
		mark, errE := markType.Create(ruleAttrs)
		if errE != nil {
			return errE
		}
		marks = append(marks[:len(marks):len(marks)], mark)
	}
	startIn := cx.top()

	if nodeType == nil || !nodeType.IsLeaf() {
		if continueAfter != nil {
			errE := cx.addElement(dom, marks, continueAfter)
			if errE != nil {
				return errE
			}
		} else {
			errE := cx.addAll(dom, marks, nil, nil)
			if errE != nil {
				return errE
			}
		}
	}
	if sync && cx.sync(startIn) {
		cx.open--
	}
	return nil
}

// addAll adds all child nodes of the given parent.
func (cx *parseContext) addAll(parent *html.Node, marks []*Mark, from, to *int) errors.E {
	dom := parent.FirstChild
	if from != nil {
		dom = childAt(parent, *from)
	}
	var end *html.Node
	if to != nil {
		end = childAt(parent, *to)
	}
	for ; dom != end; dom = dom.NextSibling {
		errE := cx.addDOM(dom, marks)
		if errE != nil {
			return errE
		}
	}
	return nil
}

// childAt returns the child node of parent at the given index, or nil when the index is out of range.
func childAt(parent *html.Node, index int) *html.Node {
	dom := parent.FirstChild
	for i := 0; i < index && dom != nil; i++ {
		dom = dom.NextSibling
	}
	return dom
}

// findPlace tries to find a way to fit the given node type into the current context. May add intermediate wrappers and/or leave non-solid nodes that we
// are in.
func (cx *parseContext) findPlace(node *Node, marks []*Mark, cautious bool) ([]*Mark, bool, errors.E) {
	var route []*NodeType
	var sync *nodeContext
	routeFound := false
	for depth, penalty := cx.open, 0; depth >= 0; depth-- {
		nodeCx := cx.nodes[depth]
		found := nodeCx.findWrapping(node)
		if found != nil && (!routeFound || len(route) > len(found)+penalty) {
			route = found
			sync = nodeCx
			routeFound = true
			if len(found) == 0 {
				break
			}
		}
		if nodeCx.solid {
			if cautious {
				break
			}
			penalty += 2
		}
	}
	if !routeFound {
		return nil, false, nil
	}
	cx.sync(sync)
	for _, typ := range route {
		var errE errors.E
		marks, errE = cx.enterInner(typ, nil, marks, false, PreserveWhitespaceDefault)
		if errE != nil {
			return nil, false, errE
		}
	}
	return marks, true, nil
}

// insertNode tries to insert the given node, adjusting the context when needed.
func (cx *parseContext) insertNode(node *Node, marks []*Mark, cautious bool) (bool, errors.E) {
	if node.IsInline() && cx.needsBlock && cx.top().typ == nil {
		block := cx.textblockFromContext()
		if block != nil {
			var errE errors.E
			marks, errE = cx.enterInner(block, nil, marks, false, PreserveWhitespaceDefault)
			if errE != nil {
				return false, errE
			}
		}
	}
	innerMarks, ok, errE := cx.findPlace(node, marks, cautious)
	if errE != nil {
		return false, errE
	}
	if ok {
		errE := cx.closeExtra(false)
		if errE != nil {
			return false, errE
		}
		top := cx.top()
		if top.match != nil {
			top.match = top.match.MatchType(node.Type)
		}
		nodeMarks := NoMarks
		for _, m := range append(innerMarks[:len(innerMarks):len(innerMarks)], node.Marks...) {
			var allows bool
			if top.typ != nil {
				allows = top.typ.AllowsMarkType(m.Type)
			} else {
				allows = markMayApply(m.Type, node.Type)
			}
			if allows {
				nodeMarks = m.AddToSet(nodeMarks)
			}
		}
		top.content = append(top.content, node.Mark(nodeMarks))
		return true, nil
	}
	return false, nil
}

// enter tries to start a node of the given type, adjusting the context when necessary.
func (cx *parseContext) enter(typ *NodeType, attrs Attrs, marks []*Mark, preserveWS PreserveWhitespace) ([]*Mark, bool, errors.E) {
	node, errE := typ.Create(attrs, nil, nil)
	if errE != nil {
		return nil, false, errE
	}
	innerMarks, ok, errE := cx.findPlace(node, marks, false)
	if errE != nil {
		return nil, false, errE
	}
	if ok {
		innerMarks, errE = cx.enterInner(typ, attrs, marks, true, preserveWS)
		if errE != nil {
			return nil, false, errE
		}
	}
	return innerMarks, ok, nil
}

// enterInner opens a node of the given type, returning the marks which could not be applied to it and remain pending.
func (cx *parseContext) enterInner(typ *NodeType, attrs Attrs, marks []*Mark, solid bool, preserveWS PreserveWhitespace) ([]*Mark, errors.E) {
	errE := cx.closeExtra(false)
	if errE != nil {
		return nil, errE
	}
	top := cx.top()
	if top.match != nil {
		top.match = top.match.MatchType(typ)
	}
	options := wsOptionsFor(typ, preserveWS, top.options)
	if top.options&optOpenLeft != 0 && len(top.content) == 0 {
		options |= optOpenLeft
	}
	applyMarks := NoMarks
	var remaining []*Mark
	for _, m := range marks {
		var allows bool
		if top.typ != nil {
			allows = top.typ.AllowsMarkType(m.Type)
		} else {
			allows = markMayApply(m.Type, typ)
		}
		if allows {
			applyMarks = m.AddToSet(applyMarks)
		} else {
			remaining = append(remaining, m)
		}
	}
	cx.nodes = append(cx.nodes, newNodeContext(typ, attrs, applyMarks, solid, nil, options))
	cx.open++
	return remaining, nil
}

// closeExtra makes sure all nodes above the open depth are finished and added to their parents.
func (cx *parseContext) closeExtra(openEnd bool) errors.E {
	i := len(cx.nodes) - 1
	if i > cx.open {
		for ; i > cx.open; i-- {
			result, errE := cx.nodes[i].finish(openEnd)
			if errE != nil {
				return errE
			}
			cx.nodes[i-1].content = append(cx.nodes[i-1].content, result.(*Node)) //nolint:errcheck,forcetypeassert
		}
		cx.nodes = cx.nodes[:cx.open+1]
	}
	return nil
}

func (cx *parseContext) finish() (any, errors.E) {
	cx.open = 0
	errE := cx.closeExtra(cx.isOpen)
	if errE != nil {
		return nil, errE
	}
	return cx.nodes[0].finish(cx.isOpen)
}

func (cx *parseContext) sync(to *nodeContext) bool {
	for i := cx.open; i >= 0; i-- {
		if cx.nodes[i] == to {
			cx.open = i
			return true
		} else if cx.localPreserveWS {
			cx.nodes[i].options |= optPreserveWS
		}
	}
	return false
}

// matchesContext determines whether the given context string matches this context.
func (cx *parseContext) matchesContext(context string) bool {
	if strings.Contains(context, "|") {
		for _, alternative := range contextPipeRegexp.Split(context, -1) { //nolint:modernize
			if cx.matchesContext(alternative) {
				return true
			}
		}
		return false
	}

	parts := strings.Split(context, "/")
	useRoot := !cx.isOpen
	minDepth := 0
	if !useRoot {
		minDepth = 1
	}
	var match func(i, depth int) bool
	match = func(i, depth int) bool {
		for ; i >= 0; i-- {
			part := parts[i]
			if part == "" {
				if i == len(parts)-1 || i == 0 {
					continue
				}
				for ; depth >= minDepth; depth-- {
					if match(i-1, depth) {
						return true
					}
				}
				return false
			}
			var next *NodeType
			if depth > 0 || (depth == 0 && useRoot) {
				next = cx.nodes[depth].typ
			}
			if next == nil || (next.Name != part && !next.IsInGroup(part)) {
				return false
			}
			depth--
		}
		return true
	}
	return match(len(parts)-1, cx.open)
}

// textblockFromContext returns the first textblock node type with non-nil default attributes, in schema declaration order.
func (cx *parseContext) textblockFromContext() *NodeType {
	for _, name := range cx.parser.Schema.nodeOrder {
		typ := cx.parser.Schema.Nodes[name]
		if typ.IsTextblock() && typ.DefaultAttrs != nil {
			return typ
		}
	}
	return nil
}

// normalizeList is a kludge to work around directly nested list nodes produced by some tools and allowed by browsers to mean that the nested list is
// actually part of the list item above it.
func normalizeList(dom *html.Node) {
	var prevItem *html.Node
	for child := dom.FirstChild; child != nil; child = child.NextSibling {
		name := ""
		if child.Type == html.ElementNode {
			name = nodeName(child)
		}
		if name != "" && listTags[name] && prevItem != nil {
			moved := child
			child = prevItem
			dom.RemoveChild(moved)
			prevItem.AppendChild(moved)
		} else if name == "li" {
			prevItem = child
		} else if name != "" {
			prevItem = nil
		}
	}
}

// nodeName returns the lowercase name of the given element.
func nodeName(dom *html.Node) string {
	return strings.ToLower(dom.Data)
}

// getAttr returns the value of the attribute with the given name on the given element, scanning attributes without a namespace and returning the first
// match.
func getAttr(dom *html.Node, name string) (string, bool) {
	for _, attr := range dom.Attr {
		if attr.Namespace == "" && attr.Key == name {
			return attr.Val, true
		}
	}
	return "", false
}

// styleWhiteSpacePre reports whether the style attribute of the given element declares white-space with a keyword value containing "pre". This mirrors
// /pre/.test(dom.style.whiteSpace) in the reference, where dom.style.whiteSpace is the CSSOM parsed value: the property name is matched case-sensitively
// as lowercase, keyword values are normalized to lowercase, and an invalid value yields "". Declarations are split on ";" and the property name and value
// on the first ":"; the last white-space declaration wins.
func styleWhiteSpacePre(dom *html.Node) bool {
	style, ok := getAttr(dom, "style")
	if !ok {
		return false
	}
	const asciiWhitespace = " \t\n\r\f"
	value := ""
	for declaration := range strings.SplitSeq(style, ";") {
		name, declarationValue, found := strings.Cut(declaration, ":")
		if !found {
			continue
		}
		if strings.Trim(name, asciiWhitespace) == "white-space" {
			value = strings.ToLower(strings.Trim(declarationValue, asciiWhitespace))
		}
	}
	switch value {
	case "pre", "pre-wrap", "pre-line", "preserve":
		return true
	default:
		return false
	}
}

// markMayApply is used when finding a mark at the top level of a fragment parse. It checks whether it would be reasonable to apply a given mark type to a
// given node, by looking at the way the mark occurs in the schema.
func markMayApply(markType *MarkType, nodeType *NodeType) bool {
	schema := nodeType.Schema
	for _, name := range schema.nodeOrder {
		parent := schema.Nodes[name]
		if !parent.AllowsMarkType(markType) {
			continue
		}
		var seen []*ContentMatch
		var scan func(match *ContentMatch) bool
		scan = func(match *ContentMatch) bool {
			seen = append(seen, match)
			for i := 0; i < match.EdgeCount(); i++ { //nolint:intrange
				edge := match.Edge(i)
				if edge.Type == nodeType {
					return true
				}
				if !slices.Contains(seen, edge.Next) && scan(edge.Next) {
					return true
				}
			}
			return false
		}
		if scan(parent.ContentMatch) {
			return true
		}
	}
	return false
}

// ParseHTML parses the given HTML input into a document node conforming to the given schema, applying the given parse options. The input is parsed as an HTML
// fragment in a div context, the way a browser parses innerHTML, so malformed HTML is repaired rather than reported as an error. The zero value of options
// reproduces the default parsing behavior.
func ParseHTML(s *Schema, input string, options ParseOptions) (*Node, errors.E) {
	divContext := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"} //nolint:exhaustruct
	nodes, err := html.ParseFragment(strings.NewReader(input), divContext)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse HTML")
	}
	div := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"} //nolint:exhaustruct
	for _, node := range nodes {
		div.AppendChild(node)
	}
	return s.domParser.Parse(div, options)
}
