# Porting ProseMirror to Go

This document is the contract for porting ProseMirror to Go for server-side use. Each ported module is a Go package
under gitlab.com/tozd/go/prosemirror and mirrors the structure of its TypeScript source (vendored as a git submodule
under prosemirror/), so the two can be compared side by side and kept in sync when a submodule is bumped. The shared
conventions, the way behavior is pinned to the reference, and the deviation policy come first; a section per module then
fixes its scope, file mapping, symbol contract, and deviations. The mapping to the TypeScript is exact enough that a
re-port after an upstream upgrade is a diff, not a rewrite.

## Conventions

These apply to every ported module.

- A module is a Go package at `gitlab.com/tozd/go/prosemirror/<module>`, vendoring its TypeScript source as a git
  submodule under prosemirror/. The house error dependency is gitlab.com/tozd/go/errors; tests use
  github.com/stretchr/testify. Any further dependencies are listed in the module's own section.
- Each Go file starts with a comment `Ported from <source>/src/<name>.ts.` Declarations keep the order of the TypeScript
  source, and the TypeScript doc comments (///) become Go doc comments, kept in meaning in sync with the source: on an
  upgrade the comments are re-derived from the updated TypeScript, not patched ad hoc, so the two stay comparable.
- A comment states only the current behavior, or a concrete difference from the TypeScript; it never narrates how the
  code got there (no "previously", no "found in review", no change history; the git log is for that), and it does not
  restate what the code plainly says. Comment style: ASCII only; no contractions (write "do not", "cannot"); double
  quotes, not backticks, and no markdown links; no spaces around "/" (write "get/set"); references to skipped features
  dropped; lines may use the full 170 character width like the code.
- Lint never drives the code away from the TypeScript shape. A finding whose idiomatic fix would diverge from the source
  structure is suppressed with a bare, specific directive (`//nolint:<linter>`, no explanatory comment); findings that
  are house style and keep the shape (splitting an inline error check, import grouping) are fixed. The golangci config
  requires the linter name and forbids unused directives.
- TypeScript classes become structs with exported fields documented as read only. Values that are persistent in the
  TypeScript library (immutable, sharing structure) keep that discipline in Go: they must not be mutated after
  construction.
- TypeScript getters become methods: node.nodeSize becomes n.NodeSize().
- Validation and other recoverable failures return errors.E (gitlab.com/tozd/go/errors). An error message is a static,
  lowercase, Go-style string with no trailing punctuation. Every dynamic value (the offending value, or a type,
  attribute, or key name) goes into the error details, never interpolated into the message. Construct the error and set
  its details through the mutable map that errors.Details returns: errE := errors.New("unknown key in node spec");
  details := errors.Details(errE); details["key"] = key; details["type"] = name; return errE (for a single detail, set
  it inline without the local, `errors.Details(errE)["key"] = key`). When adding details to an existing error, you can
  also use errors.WithDetails. errors.Errorf is used only when an interpolated value is effectively a constant context
  prefix that keeps the message static-looking. A foreign error is wrapped with errors.Wrap(err, "static message") (its
  cause preserved) if the foreign error is a low-level error or if its error message is interpolated with values, with
  any context added as details the same way. Simple foreign errors with constant messages are wrapped with
  errors.WithStack or errors.WithMessage, adding any details if necessary. A detail key names the kind of value it
  carries and the same concept always uses the same key, so a node or mark type name is always "type", never "node"; the
  recurring keys are type (node or mark type name), kind ("node" or "mark"), attribute, key, value, validator, expected,
  got, and the content-expression key "expression", with purpose-specific keys (such as content, selector, token, marks,
  topNode) used where a distinct value needs naming. Go type is recorded with the
  `errors.Details(errE)["type"] = fmt.Sprintf("%T", value)` pattern. The port adapts the TypeScript messages to this
  style rather than copying them verbatim. Programmer errors that the TypeScript reports by throwing on misuse of an
  index or constructor (out-of-range child access, empty text node construction) panic in Go, mirroring Go slice
  indexing conventions; the panic value is built the same way as a returned error (static message, dynamic values in
  details) and panicked as an errors.E, so a panic carrying a value reads panic(errE) with the value in errE's details
  rather than formatted into the string.
- Dynamic values decoded from JSON use Go's default any types: nil, bool, float64, string, []any, map[string]any.
  Numbers are always float64, including when constructed programmatically in tests.
- Where the TypeScript relies on object insertion order being significant, the Go port preserves it explicitly (decoding
  with json.Decoder tokens and iterating over order-preserving slices) rather than relying on Go map iteration, which is
  randomized.

## Pinning behavior to the reference

A module's behavior is pinned by fixtures generated from the real ProseMirror TypeScript, not from a hand-written spec.
A small Node.js generator (under scripts/, run with npm run generate-fixtures) produces reference outputs at the upstream
version the git submodule is pinned to. The submodule is the committed reference the port is derived from, both its source
(src) and its tests (test), and it stays the single source of truth for the reference version. A module that also consumes
the upstream npm package (to publish a TypeScript counterpart) keeps that package in lockstep with the submodule, enforced
at generation time (see scripts/check-pm-version.ts). Go tests replay the fixtures and assert the port matches byte for
byte. Each module documents its own fixture format and the schemas or inputs it covers.

Upgrade procedure: bump the submodule to the new version, run npm run generate-fixtures to regenerate, and review the
fixture diff. Behavior changes in the upstream package surface as fixture diffs to review before adopting, and the Go
port is reconciled until its tests pass against the regenerated fixtures.

## Deviation policy

What is ported, and where the port deviates, follow a few principles applied per module.

- The line for what to port is drawn at coupling to deferred machinery or genuinely editor-only features, never at what
  a particular downstream consumer happens to use. A module's public surface is as general as the upstream module.
- Behavior that affects valid documents, produced output, or robustness is replicated exactly, and the port never panics
  on input it can decode.
- For malformed input that no conforming producer emits, the port prefers strict rejection over replicating JavaScript
  falsy-coercion quirks: rejecting corrupt input is safer than silently coercing it, and it cannot reject any valid
  round trip.
- Functions cannot live in the shared JSON. Function hooks that are essential become named registries (resolved per
  implementation); the rest are out of scope.

Each module lists its concrete deviations as a numbered list.

## prosemirror-model

The model package ports the ProseMirror document model: the schema (node and mark types, content expressions, attribute
validation), the immutable Node, Fragment, and Mark data structures, HTML fragment parsing (from_dom), and canonical
HTML serialization (to_dom, replaced by a direct doc to string serializer). The TypeScript source is vendored at
prosemirror/prosemirror-model.

The schema JSON dialect also has a TypeScript counterpart, published as the NPM package @tozd/prosemirror (the src
directory): buildSchema turns a parsed schema JSON object and a validator registry into a prosemirror-model Schema, the
browser-usable equivalent of NewSchema, sharing the dialect with this Go port. The module also exports serializeDOM (and
escapeHTML), which stringify the DOM that prosemirror-model's DOMSerializer produces into the same canonical HTML the Go
SerializeHTML emits (same five-character escaping, lowercased tags, double-quoted attributes, void-element handling, and
pre newline rule), so a frontend can compute the canonical form the backend does. It takes prosemirror-model as a peer
dependency. The fixture generator builds its schemas through buildSchema and serializes through serializeDOM, so one
dialect parser and one canonical serializer drive both the published module and the oracle; the npm prosemirror-model
version is pinned to the vendored submodule (scripts/check-pm-version.ts).

### Scope

Ported: schema.ts, content.ts, node.ts, fragment.ts, mark.ts, comparedeep.ts, from_dom.ts (parse semantics reimplemented
over golang.org/x/net/html), to_dom.ts (replaced by a direct doc to string serializer producing the canonical HTML form,
see below).

Not ported (deferred, the design must not preclude them): replace.ts, resolvedpos.ts, diff.ts, and
prosemirror-transform. Not applicable: dom.ts, index.ts.

The parser is as general as the model and the serializer: NewSchema accepts any spec in the dialect, and the parse layer
supports full CSS selectors, style parse rules, and namespace matching. The line for what is not ported is drawn at
coupling to deferred machinery (Slice, replace, ResolvedPos) or genuinely editor-only features.

Skipped members inside ported files (transform-era or editor-only): Node.cut, Node.slice, Node.replace, Node.resolve,
Node.nodesBetween, Node.rangeHasMark, Node.nodeAt, Node.childAfter, Node.childBefore, Node.canReplace,
Node.canReplaceWith, Node.canAppend, Node.textBetween, Fragment.cut, Fragment.cutByIndex, Fragment.replaceChild,
Fragment.addToStart, Fragment.addToEnd, Fragment.nodesBetween, Fragment.descendants, Fragment.textBetween,
Fragment.findDiffStart, Fragment.findDiffEnd, Fragment.findIndex, DOMParser.parseSlice (returns a Slice, deferred with
replace.ts), ParseOptions.findPositions and all find\* methods of ParseContext (editor-only, maps DOM nodes to document
positions to preserve a selection; trivially addable later), ParseOptions.topMatch, ParseOptions.context (a ResolvedPos,
deferred with resolvedpos.ts), ParseOptions.ruleFromNode, TagParseRule.contentElement, TagParseRule.getContent, and the
consuming/clearMark function hooks (functions, which JSON cannot carry).

### File mapping

| TypeScript source              | Go file              | Test file              |
| ------------------------------ | -------------------- | ---------------------- |
| src/comparedeep.ts             | model/comparedeep.go |                        |
| src/mark.ts                    | model/mark.go        | model/mark_test.go     |
| src/fragment.ts                | model/fragment.go    |                        |
| src/node.ts                    | model/node.go        | model/node_test.go     |
| src/content.ts                 | model/content.go     | model/content_test.go  |
| src/schema.ts                  | model/schema.go      | model/schema_test.go   |
| src/from_dom.ts                | model/from_dom.go    | model/fixtures_test.go |
| src/to_dom.ts (replaced)       | model/to_html.go     | model/fixtures_test.go |
| (new) string level entrypoints | model/html.go        | model/html_test.go     |

Fixture driven tests (parse, serialize, canonicalization, property tests) live in model/fixtures_test.go; it is where
the from_dom.go and to_html.go behaviors are exercised. Edge and error paths the fixtures do not reach (schema
construction robustness, JavaScript style attribute value stringification, debug output formatting) live in
model/regression_test.go.

### Module conventions and dependencies

Beyond the shared dependencies, the model package uses golang.org/x/net/html (and html/atom) for HTML parsing,
github.com/andybalholm/cascadia for CSS selector matching in parse rules, and github.com/tdewolff/parse/v2/css for
inline style tokenizing.

- TextNode is merged into Node: a Node with Type.IsText true carries its text in the Text field, has Content ==
  EmptyFragment, and its methods branch where the TypeScript TextNode overrides the base class.
- Node and mark attributes (the Attrs type) are the JSON-decoded values described in the shared conventions, so numbers
  are always float64.
- NodeSize of a text node counts UTF-16 code units (JavaScript string length semantics), so that document positions
  agree with the TypeScript implementation once transform support is added. Helper utf16Length(s string) int lives in
  node.go.
- The schema spec JSON preserves the declaration order of nodes and marks (it determines mark rank, parse rule
  precedence, and group expansion order), so schema.go decodes those two objects with json.Decoder tokens, and
  in-package iteration where order is significant uses the Schema.nodeOrder and Schema.markOrder slices.

### Symbol contract

All symbols below live in package model. These signatures are the contract for the package; the code matches them, and a
re-port after a prosemirror-model upgrade keeps them.

#### comparedeep.go

    func compareDeep(a, b any) bool

Operates on JSON-decoded values. Scalars compare with ==, slices and maps recursively. A []any never equals a
map[string]any.

#### mark.go

    type Mark struct {
        Type  *MarkType
        Attrs Attrs
    }
    func (m *Mark) AddToSet(set []*Mark) []*Mark
    func (m *Mark) RemoveFromSet(set []*Mark) []*Mark
    func (m *Mark) IsInSet(set []*Mark) bool
    func (m *Mark) Eq(other *Mark) bool
    func (m *Mark) ToJSON() map[string]any
    func (m *Mark) MarshalJSON() ([]byte, error)
    func markFromJSON(schema *Schema, value any) (*Mark, errors.E)   // TS Mark.fromJSON
    func SameMarkSet(a, b []*Mark) bool                              // TS Mark.sameSet
    func MarkSetFrom(marks []*Mark) []*Mark                          // TS Mark.setFrom; sorts a copy by rank (stable)
    var NoMarks = []*Mark{}                                          // TS Mark.none

#### fragment.go

    type Fragment struct {
        Content []*Node
        Size    int
    }
    var EmptyFragment = &Fragment{Content: nil, Size: 0}             // TS Fragment.empty
    func newFragment(content []*Node, size int) *Fragment            // size < 0 means compute from content
    func (f *Fragment) Append(other *Fragment) *Fragment
    func (f *Fragment) Eq(other *Fragment) bool
    func (f *Fragment) FirstChild() *Node                            // nil when empty
    func (f *Fragment) LastChild() *Node
    func (f *Fragment) ChildCount() int
    func (f *Fragment) Child(index int) *Node                        // panics when out of range
    func (f *Fragment) MaybeChild(index int) *Node
    func (f *Fragment) ForEach(fn func(node *Node, offset, index int))
    func (f *Fragment) String() string
    func (f *Fragment) toStringInner() string
    func (f *Fragment) ToJSON() []any                                // nil when empty
    func fragmentFromJSON(schema *Schema, value any) (*Fragment, errors.E)
    func FragmentFromArray(array []*Node) *Fragment                  // joins adjacent text nodes with equal markup

Everywhere the TypeScript code calls Fragment.from with an array or a single node, the Go code calls FragmentFromArray
(wrapping a single node in a slice); Fragment.from(null) becomes EmptyFragment.

#### node.go

    type Node struct {
        Type    *NodeType
        Attrs   Attrs
        Content *Fragment   // EmptyFragment for leaf and text nodes
        Marks   []*Mark
        Text    string      // non-empty exactly for text nodes
    }
    func newNode(typ *NodeType, attrs Attrs, content *Fragment, marks []*Mark) *Node  // nil content -> EmptyFragment, nil marks -> NoMarks
    func newTextNode(typ *NodeType, attrs Attrs, text string, marks []*Mark) *Node    // panics on empty text
    func (n *Node) NodeSize() int
    func (n *Node) ChildCount() int
    func (n *Node) Child(index int) *Node
    func (n *Node) MaybeChild(index int) *Node
    func (n *Node) ForEach(fn func(node *Node, offset, index int))
    func (n *Node) TextContent() string        // direct recursion instead of textBetween (textBetween is not ported)
    func (n *Node) FirstChild() *Node
    func (n *Node) LastChild() *Node
    func (n *Node) Eq(other *Node) bool
    func (n *Node) SameMarkup(other *Node) bool
    func (n *Node) HasMarkup(typ *NodeType, attrs Attrs, marks []*Mark) bool
    func (n *Node) Copy(content *Fragment) *Node
    func (n *Node) Mark(marks []*Mark) *Node
    func (n *Node) WithText(text string) *Node  // TS TextNode.withText; panics when not a text node
    func (n *Node) IsBlock() bool
    func (n *Node) IsTextblock() bool
    func (n *Node) InlineContent() bool
    func (n *Node) IsInline() bool
    func (n *Node) IsText() bool
    func (n *Node) IsLeaf() bool
    func (n *Node) IsAtom() bool
    func (n *Node) String() string
    func (n *Node) ContentMatchAt(index int) *ContentMatch  // panics on invalid content, mirroring the TS throw
    func (n *Node) Check() errors.E
    func (n *Node) ToJSON() map[string]any
    func (n *Node) MarshalJSON() ([]byte, error)
    func nodeFromJSON(schema *Schema, value any) (*Node, errors.E)   // TS Node.fromJSON; does not run Check
    func utf16Length(s string) int

#### content.go

    type MatchEdge struct {
        Type *NodeType
        Next *ContentMatch
    }
    type ContentMatch struct {
        ValidEnd bool
        // unexported: next []MatchEdge, wrapCache guarded by a sync.Mutex
    }
    var EmptyContentMatch = &ContentMatch{ValidEnd: true}            // TS ContentMatch.empty
    func parseContentMatch(expr string, schema *Schema) (*ContentMatch, errors.E)  // TS ContentMatch.parse
    func (cm *ContentMatch) MatchType(typ *NodeType) *ContentMatch
    func (cm *ContentMatch) MatchFragment(frag *Fragment, start, end int) *ContentMatch
    func (cm *ContentMatch) InlineContent() bool
    func (cm *ContentMatch) DefaultType() *NodeType
    func (cm *ContentMatch) Compatible(other *ContentMatch) bool
    func (cm *ContentMatch) FillBefore(after *Fragment, toEnd bool, startIndex int) *Fragment  // nil when no fill exists
    func (cm *ContentMatch) FindWrapping(target *NodeType) []*NodeType  // nil: none; empty non-nil: fits directly
    func (cm *ContentMatch) EdgeCount() int
    func (cm *ContentMatch) Edge(n int) MatchEdge                    // panics when out of range
    func (cm *ContentMatch) String() string

The TokenStream, expression parser, nfa, dfa, and checkForDeadEnds helpers are unexported and ported one to one. Group
names in content expressions resolve against node types in schema declaration order (Schema.nodeOrder). The wrapCache is
guarded by a mutex because Go schemas are shared across goroutines. FillBefore creating filler nodes calls CreateAndFill
on generatable types; both error and nil results are impossible there by construction (checkForDeadEnds guarantees it),
so the Go code panics if it happens.

#### schema.go

    type Attrs map[string]any
    type AttrValidator func(value any) errors.E

    type AttributeSpec struct {
        Default    any
        HasDefault bool    // the "default" key was present in JSON
        Validate   string  // validator name; empty means no validation
        OnInvalid  string  // "rejectRule" (default) or "drop"; governs HTML parse behavior only
    }
    type NodeSpec struct {
        Content              string
        Marks                *string  // nil means not given ("" and absent differ, see markSet rules)
        Group                string
        Inline               bool
        Atom                 bool
        Attrs                map[string]*AttributeSpec
        Code                 bool
        Whitespace           string   // "", "pre", or "normal"
        LinebreakReplacement bool
        ToHTML               *ToHTMLSpec
        ParseHTML            []*ParseRule
    }
    type MarkSpec struct {
        Attrs     map[string]*AttributeSpec
        Excludes  *string
        Group     string
        Code      bool
        Spanning  *bool   // nil means default true
        ToHTML    *ToHTMLSpec
        ParseHTML []*ParseRule
    }
    type SchemaSpec struct {
        TopNode string  // "" means "doc"
        Nodes   []*NamedNodeSpec  // type NamedNodeSpec struct { Name string; Spec *NodeSpec }
        Marks   []*NamedMarkSpec  // type NamedMarkSpec struct { Name string; Spec *MarkSpec }
    }

    type Attribute struct {  // compiled form
        HasDefault bool
        Default    any
        Validate   AttrValidator  // nil when no validation
        OnInvalid  string
    }
    func (a *Attribute) IsRequired() bool

    type NodeType struct {
        Name          string
        Schema        *Schema
        Spec          *NodeSpec
        Groups        []string
        Attrs         map[string]*Attribute
        DefaultAttrs  Attrs           // nil when some attribute has no default
        ContentMatch  *ContentMatch
        InlineContent bool
        IsBlock       bool
        IsText        bool
        MarkSet       []*MarkType     // nil means all marks allowed; empty non-nil means none
    }
    func (nt *NodeType) IsInline() bool
    func (nt *NodeType) IsTextblock() bool
    func (nt *NodeType) IsLeaf() bool
    func (nt *NodeType) IsAtom() bool
    func (nt *NodeType) IsInGroup(group string) bool
    func (nt *NodeType) Whitespace() string   // "pre" or "normal"
    func (nt *NodeType) HasRequiredAttrs() bool
    func (nt *NodeType) CompatibleContent(other *NodeType) bool
    func (nt *NodeType) computeAttrs(attrs Attrs) (Attrs, errors.E)
    func (nt *NodeType) Create(attrs Attrs, content *Fragment, marks []*Mark) (*Node, errors.E)
    func (nt *NodeType) CreateChecked(attrs Attrs, content *Fragment, marks []*Mark) (*Node, errors.E)
    func (nt *NodeType) CreateAndFill(attrs Attrs, content *Fragment, marks []*Mark) (*Node, errors.E)  // nil, nil when no fill exists
    func (nt *NodeType) ValidContent(content *Fragment) bool
    func (nt *NodeType) CheckContent(content *Fragment) errors.E
    func (nt *NodeType) CheckAttrs(attrs Attrs) errors.E
    func (nt *NodeType) AllowsMarkType(markType *MarkType) bool
    func (nt *NodeType) AllowsMarks(marks []*Mark) bool
    func (nt *NodeType) AllowedMarks(marks []*Mark) []*Mark

    type MarkType struct {
        Name     string
        Rank     int
        Schema   *Schema
        Spec     *MarkSpec
        Attrs    map[string]*Attribute
        Excluded []*MarkType
        // unexported: instance *Mark
    }
    func (mt *MarkType) Create(attrs Attrs) (*Mark, errors.E)
    func (mt *MarkType) RemoveFromSet(set []*Mark) []*Mark
    func (mt *MarkType) IsInSet(set []*Mark) *Mark
    func (mt *MarkType) CheckAttrs(attrs Attrs) errors.E
    func (mt *MarkType) Excludes(other *MarkType) bool

    type Schema struct {
        Spec                 *SchemaSpec
        Nodes                map[string]*NodeType
        Marks                map[string]*MarkType
        TopNodeType          *NodeType
        LinebreakReplacement *NodeType
        // unexported: nodeOrder, markOrder []string; domParser *DOMParser (built eagerly by NewSchema)
    }
    func NewSchema(specJSON []byte, validators map[string]AttrValidator) (*Schema, errors.E)
    func (s *Schema) Node(typeName string, attrs Attrs, content []*Node, marks []*Mark) (*Node, errors.E)  // createChecked semantics
    func (s *Schema) Text(text string, marks []*Mark) *Node
    func (s *Schema) Mark(typeName string, attrs Attrs) (*Mark, errors.E)
    func (s *Schema) NodeFromJSON(data []byte) (*Node, errors.E)  // nodeFromJSON followed by Check (validates fully)
    func (s *Schema) MarkFromJSON(data []byte) (*Mark, errors.E)
    func (s *Schema) NodeType(name string) (*NodeType, errors.E)

Compilation (NewSchema) follows the TypeScript Schema constructor: NodeType.compile, MarkType.compile (rank is
declaration order), content expression parsing with a per-expression cache, inlineContent, linebreakReplacement checks,
markSet computation (markExpr "\_" is nil, named list gathers marks and mark groups, "" or non-inline content is empty
non-nil), mark excluded computation via gatherMarks. Additional compile time validation specific to this port:

- Strict JSON decoding. Unknown keys anywhere in the spec are errors, except the editor-only keys which are accepted and
  ignored: selectable, draggable, defining, isolating, definingAsContext, definingForContent (nodes) and inclusive
  (marks).
- Validator resolution: a non-empty "validate" name is looked up in the validators map first; otherwise it must be a "|"
  separated union of the builtin type names string, number, boolean, null, which compiles to a type check on the
  JSON-decoded value. Unknown names are compile errors. "undefined" is not supported (JSON has no undefined).
- "onInvalid" must be "rejectRule" or "drop"; "drop" requires the attribute to have a default.
- Every node except the text node and the top node must have toHTML; the top node and text must not have toHTML. Every
  mark must have toHTML. text must have no attrs, no parseHTML.
- Parse rule "attrs" entries must reference declared attributes of the node or mark. A style parse rule produces a mark
  and may only appear on a mark type.
- The DOMParser is built eagerly at the end of NewSchema (this compiles the parse rule selectors and validates rule
  shapes).

checkAttrs (Go CheckAttrs) additionally errors when a declared attribute is missing from the given values (TypeScript
would pass undefined to the validator); computed attrs are always complete, so this only catches malformed JSON input. A
value equal (compareDeep) to the declared attribute default is always valid and skips the validator: the default is what
an absent value decodes to and what the onInvalid "drop" policy produces during HTML parsing, so it is acceptable by
definition (for example a null cite on a blockquote must not be run through the resourceURL validator). Implementations
constructing a ProseMirror schema from the shared JSON must apply the same rule when wiring named validators into
AttributeSpec.validate.

#### from_dom.go

    type PreserveWhitespace int
    const (
        PreserveWhitespaceDefault PreserveWhitespace = iota  // not given
        PreserveWhitespaceFalse                              // JSON false
        PreserveWhitespaceTrue                               // JSON true: preserve, normalize newlines to spaces
        PreserveWhitespaceFull                               // JSON "full": preserve everything
    )
    type ParseRule struct {                            // exactly one of Tag and Style is set
        Tag                string          // a CSS selector: "p", "a[href]", "p.MsoNormal", ...
        Style              string          // an inline CSS declaration: "font-weight" or "font-weight=bold"
        Namespace          *string         // optional element namespace URI (tag rules only); nil means no constraint
        Node               string          // filled in by schemaRules
        Mark               string          // filled in by schemaRules
        Attrs              map[string]any  // tag rule: string value extracts from that HTML attribute, other JSON values are constants; style rule: all constants
        Context            string
        Priority           *int            // nil means 50
        PreserveWhitespace PreserveWhitespace
        // unexported: selector cascadia.Selector (tag rules), styleProp/styleValue/hasStyleValue (style rules)
    }
    type ParseOptions struct {
        PreserveWhitespace PreserveWhitespace
        From               *int            // child index of the top DOM node to start at; nil means first
        To                 *int            // child index to stop at, exclusive; nil means past last
        TopNode            *Node           // top container type and attrs; nil means the schema top node type
    }
    type DOMParser struct {
        Schema *Schema
        Rules  []*ParseRule
        // unexported: tags, styles []*ParseRule, matchedStyles []string, normalizeLists bool
    }
    func newDOMParser(schema *Schema, rules []*ParseRule) (*DOMParser, errors.E)
    func schemaRules(schema *Schema) []*ParseRule   // priority ordered copy, marks first then nodes, declaration order
    func (p *DOMParser) Parse(dom *html.Node, options ParseOptions) (*Node, errors.E)
    func (p *DOMParser) matchTag(dom *html.Node, cx *parseContext, after *ParseRule) (*ParseRule, Attrs)
    func (p *DOMParser) matchStyle(prop, value string, cx *parseContext, after *ParseRule) (*ParseRule, Attrs)
    func ParseHTML(s *Schema, input string, options ParseOptions) (*Node, errors.E)

ParseRule is the declarative subset of ProseMirror's ParseRule union: a tag rule (Tag set) or a style rule (Style set).
newDOMParser compiles each tag rule's CSS selector once with github.com/andybalholm/cascadia (the selector engine behind
goquery, matching Selectors L3/L4 without a browser) and matches it against html.Node values with selector.Match; a bare
"p" compiles fine, so this replaces a hand-rolled tag/attribute matcher rather than supplementing it. Namespace matching
compares the rule's namespace URI, mapped to the short identifier golang.org/x/net/html stores in html.Node.Namespace
("svg", "math", or "" for HTML), against the element's namespace. Style rules read only the element's own inline style
attribute (the raw style="..." value, tokenized with github.com/tdewolff/parse/v2/css into property/value declarations,
last declaration winning), never a computed style: shorthand expansion (style="font: bold ..." exposing font-weight) and
CSSOM value normalization are NOT replicated. This is the single fidelity boundary of the port; everything else parses
identically to the browser. Style rules produce marks, so they may only appear on mark types (a style rule on a node
type is a compile error). readStyles applies the matching style rules' marks to the element's inline content. The cheap
parse options From/To/TopNode are honored by Parse.

matchTag and matchStyle return the computed per-match attrs as a second value instead of mutating the shared rule (the
TypeScript code stores them on rule.attrs, which is not safe with concurrent parses). Tag rule attribute extraction
implements the declarative getAttrs: constants are taken as-is; extracted attributes read the HTML attribute, falling
back to the attribute default when absent (the rule is rejected when the attribute is required); when a validator is
configured and fails, OnInvalid "rejectRule" rejects the rule (continue to the next rule) and "drop" replaces the value
with the default. Style rule attrs are constants only (extracting from the style value would need a getAttrs function,
which JSON cannot carry).

nodeContext and parseContext are unexported ports of NodeContext and ParseContext, minus the find\* position tracking
and the ResolvedPos based context option. The whitespace option bitfield (optPreserveWS, optPreserveWSFull,
optOpenLeft), wsOptionsFor, blockTags, ignoreTags, listTags, normalizeList (which mutates the x/net/html tree),
markMayApply, leafFallback, ignoreFallback, and the matchesContext machinery (without the options.context branches) are
ported one to one. The ignore list (head, noscript, object, script, style, title) is matched by local tag name
(html.Node.Data, lowercased) regardless of namespace, so a foreign-namespaced script or style, such as the script in
svg>script, is dropped together with its content and never leaks its body as text. The localPreserveWS check for the
style attribute inspects the white-space declaration in a style attribute string directly (last declaration wins), since
there is no CSSOM. It emulates the CSSOM normalization the reference depends on (/pre/.test(dom.style.whiteSpace)): the
property name is matched case-sensitively as lowercase "white-space", the value is lowercased, and only the keyword
values pre, pre-wrap, pre-line, preserve are treated as preserving (an invalid value yields no preservation, matching an
empty CSSOM value). DOM specifics: element name comparisons use html.Node DataAtom/Data (lowercase), text nodes are
html.TextNode, comments and doctypes fall through addDOM unhandled, previousSibling is PrevSibling.

ParseHTML parses the input as an HTML fragment in a div context (the innerHTML parsing of a div) via html.ParseFragment,
appends the returned nodes to a fresh div node, and runs the schema's DOMParser over it with the given parse options. Malformed
HTML never errors.

#### to_html.go

    type ToHTMLSpec struct {
        Tag     string       // may contain "{attrName}" placeholders substituted with the attribute value
        Attrs   []string     // attribute names emitted in this order; nil valued attributes are omitted
        Content *ToHTMLSpec  // nil: content hole is directly in this element; set: this element wraps the nested element
    }
    func SerializeHTML(n *Node) string

SerializeHTML serializes the content of the given node (for a doc that is the document content). It implements the mark
spanning algorithm of DOMSerializer.serializeFragment from to_dom.ts (the keep/rendered reconciliation of the active
mark stack, including spec spanning false closing a mark after every node), emitting strings directly:

- A node or mark serializes as the chain of elements its ToHTMLSpec describes, outermost first, with the node content or
  the marked nodes placed at the content hole, which sits at the innermost spec in the chain (the one with Content nil).
  This is the declarative subset of ProseMirror's DOMOutputSpec: a single wrapper chain with the hole at the innermost
  element and attribute values taken from node or mark attributes. A schema where a node renders as nested elements,
  such as a code block serializing as "pre" wrapping "code" (ToHTMLSpec{Tag: "pre", Content: &ToHTMLSpec{Tag: "code"}}),
  is expressible. Not expressible (no schema in scope needs them, and most would not round trip under the declarative
  parse subset): constant attribute values, constant text children, multiple children, or the hole at a position other
  than the innermost element.
- Text and attribute values escape exactly five characters: & to &amp;, < to &lt;, > to &gt;, " to &#34;, ' to &#39;.
  Everything else is emitted raw, including U+00A0.
- Tag and attribute names are lowercased, matching the DOM createElement and setAttribute path the canonical form is
  defined against (which lowercases both for HTML elements), so a schema declaring upper or mixed case names still
  produces canonical lowercase HTML.
- Attributes are emitted in ToHTMLSpec order, always double-quoted. nil attributes are omitted. Attribute values, and
  the values substituted into tag placeholders, stringify exactly like JavaScript String() for every JSON value type: a
  string as-is; a bool as "true"/"false"; nil as "null"; a float64 as the shortest round-trip decimal, switching to
  exponent notation (with a JavaScript-style exponent, no zero padding) when the magnitude is at least 1e21 or below
  1e-6, with negative zero rendered as "0"; a []any as the comma-joined stringifications of its elements (a nil element
  contributes ""); a map[string]any as "[object Object]". This mirrors the DOM serializer the canonical form is defined
  against, which coerces values with String() via setAttribute.
- Void elements (the full HTML spec list: area, base, br, col, embed, hr, img, input, link, meta, source, track, wbr)
  emit only the open tag, no self-closing slash, no content, no close tag. The full list keeps any schema well formed.
- When the innermost element (the one directly containing the content) is "pre" and the first child of the node is a
  text node with no marks starting with "\n", one extra "\n" is emitted after the open tag (the HTML parser drops one
  newline right after a pre start tag; this keeps the round trip lossless). The check is on the innermost element
  because the parser drops the newline only directly after a pre start tag: with a wrapper such as pre>code the content
  is not directly inside the pre, so the rule does not apply. The first child must carry no marks: a marked first child
  serializes wrapped in a mark element, so the rendered first child the reference inspects is not a text node.
- No indentation, no added whitespace anywhere.

#### html.go

    func CanonicalizeHTML(s *Schema, input string, options ParseOptions) (string, errors.E)  // SerializeHTML(ParseHTML(input, options))
    func IsCanonicalHTML(s *Schema, input string, options ParseOptions) (bool, errors.E)     // CanonicalizeHTML(input, options) == input

### Schema JSON dialect

The schema spec is a single JSON document shareable between this Go implementation and a TypeScript implementation
(which builds a ProseMirror Schema from it plus a validator registry). Functions cannot live in JSON, so validators are
referenced by name and the HTML mapping is declarative.

    {
      "topNode": "doc",
      "nodes": {
        "<name>": {
          "content": "inline*",
          "marks": "link bold",            // optional; "_" all, "" none, absent: default
          "group": "block",
          "inline": true,
          "code": true,
          "whitespace": "pre",             // "pre" or "normal"
          "atom": true,
          "linebreakReplacement": true,
          "attrs": {
            "<attr>": {"default": null, "validate": "resourceURL", "onInvalid": "drop"}
          },
          "toHTML": {"tag": "h{level}", "attrs": ["cite"]},
          // toHTML may nest: {"tag": "pre", "content": {"tag": "code"}} renders a code block as pre wrapping code (the content hole moves to
          // the innermost element). "content" is the declarative subset of a nested DOMOutputSpec.
          "parseHTML": [
            {"tag": "p.MsoNormal", "context": "blockquote/", "priority": 60},   // any CSS selector
            {"tag": "a[href]", "attrs": {"href": "href"}},
            {"tag": "svg", "namespace": "http://www.w3.org/2000/svg"},           // namespace constraint
            {"style": "font-weight=bold"},                                       // a style rule (mark types only)
            {"tag": "pre", "preserveWhitespace": "full"}
          ],
          "selectable": false              // editor-only keys accepted and ignored, see schema.go contract
        }
      },
      "marks": { ... same shape, plus "excludes", "spanning", minus "content"/"inline"/... }
    }

A parse rule has exactly one of "tag" (a CSS selector) and "style" (an inline CSS declaration). A tag rule may also
carry a "namespace" (the full namespace URI, matched against the element namespace) and, for non-leaf nodes,
"preserveWhitespace". Tag rule "attrs" values: a string names the HTML attribute to extract; a number, boolean, or null
is a constant. Constant strings are not supported (no schema needs them; an explicit object form can be added later if
one does). A style rule's "style" is either a property name ("font-weight", matching any value) or property and value
("font-weight=bold"); its "attrs" are all constants; it produces a mark, so it may only appear on a mark type (a style
rule on a node type is a compile error), and only the element's own inline style declarations are read (no shorthand
expansion, no CSSOM normalization). "toHTML" tags support "{attr}" placeholders (used for heading levels) and an
optional "content" key holding a nested toHTML spec (the content hole sits at the innermost element). The fixture
schemas live at model/testdata/example-schema.json (an example editor schema with URL-validated links and a restricted
set of marks), model/testdata/basic-schema.json (an approximation of prosemirror-schema-basic plus lists, including a
nested pre>code code block, used by parser test cases ported from test/test-dom.ts),
model/testdata/custom-marks-schema.json (a non-spanning mark, for the corresponding test/test-dom.ts case),
model/testdata/linebreak-schema.json (a linebreakReplacement hard_break and no pre node, for the test/test-dom.ts pre
whitespace and line break replacement cases), and model/testdata/feature-schema.json (a CSS class selector, a style
rule, and a namespace rule, exercising the general parse layer).

Named validators used by the example schema (each implementation registers its own functions with these semantics):

    linkURL      matches (?i)^(?:/(?:[^/]|$)|https?://[^/]|mailto:[^/])
    resourceURL  matches (?i)^(?:/(?:[^/]|$)|https?://[^/])

Non-string values are always invalid for both.

### Fixtures

The model fixtures live at `model/testdata/fixtures/<category>.json`, generated by scripts/generate-fixtures.ts (see
"Pinning behavior to the reference"). Each file has the shape:

    {
      "schema": "example-schema.json",
      "cases": [
        {"name": "<case name>", "input": "<input HTML>", "doc": { ... Node JSON ... }, "canonical": "<canonical HTML>"}
      ]
    }

"doc" is the JSON of the document produced by parsing "input" through a jsdom div innerHTML and the ProseMirror
DOMParser over the schema; "canonical" is the canonical HTML serialization of that document. Canonicalization is not
always idempotent: a document can serialize to HTML which parses back to a different document (for example a paragraph
whose text keeps a leading space because of the block structure it was parsed out of); when re-canonicalizing
"canonical" differs from it, the case additionally records that result as "recanonical". Go tests assert, for every
case: ParseHTML(input) marshals to JSON structurally equal to doc; SerializeHTML(ParseHTML(input)) == canonical;
IsCanonicalHTML(canonical) is true exactly when "recanonical" is absent; when "recanonical" is present,
CanonicalizeHTML(canonical) == recanonical. The corpus fixture is generated from model/testdata/corpus/t\*.html.input (a
corpus of HTML sanitization test inputs; a single trailing newline is stripped from each file).

### Deviations

1. TextNode is merged into Node (Go has no subclassing).
2. matchTag and matchStyle return matched attrs instead of mutating rule.attrs (concurrency safety).
3. ContentMatch.FindWrapping guards its cache with a mutex (schemas are shared across goroutines).
4. The DOMParser is built eagerly per schema in NewSchema instead of being cached on demand (Schema.cached does not
   exist).
5. CSS selector matching uses github.com/andybalholm/cascadia instead of the browser element.matches, and style rules
   read the inline style attribute (tokenized with github.com/tdewolff/parse/v2/css) instead of the CSSOM. The one
   resulting behavioral boundary is that style shorthands are not expanded and CSSOM values are not normalized: a style
   rule matches only the element's own inline longhand declarations (a font-weight rule matches style="font-weight:
   bold" but not style="font: bold ..."), where the browser-backed reference matches the shorthand too. Everything else
   parses identically. contentElement, getContent, ruleFromNode, findPositions, parseSlice, the ResolvedPos context
   option, and the consuming/clearMark function hooks are not ported (coupled to deferred machinery, editor-only, or JS
   functions the JSON dialect cannot carry). The white-space: pre detection for the style attribute is likewise read
   directly over the attribute string, emulating the CSSOM keyword normalization the reference depends on
   (case-sensitive lowercase property name, lowercased value matched against the white-space keyword set), so it agrees
   with the reference on uppercase and invalid values.
6. to_dom.ts DOM construction is replaced by direct string serialization (to_html.go) with the same mark spanning
   semantics.
7. Text node sizes count UTF-16 code units.
8. CheckAttrs errors on missing declared attributes instead of passing undefined to validators.
9. Schema.NodeFromJSON runs a full Check after deserialization (TypeScript Node.fromJSON validates only attrs and
   marks); collaboration patches arrive through this path, so JSON input must never bypass validation.
10. The void element list in the serializer is the full HTML spec list, so the serializer is well formed for any schema.
11. The schema spec is strict JSON: unknown fields are rejected; editor-only fields are accepted and ignored; validators
    are named.
12. NodeContext.activeMarks (dead code in 1.25.8) is not ported.
13. CreateChecked refuses to construct text nodes like Create does (the TypeScript createChecked lacks the guard and
    would silently build a broken text node without text).
14. Deserialization and node construction reject malformed JSON values that TypeScript silently coerces through
    JavaScript falsy quirks, rather than replicating the coercion (extending deviation 9: collaboration patch JSON must
    not bypass validation, and no conforming toJSON ever emits these forms, so no valid document round trip is
    affected). Specifically: fragmentFromJSON and nodeFromJSON require a present content or marks value to be null or a
    JSON array (TypeScript treats falsy non-null values false/0/"" as empty); nodeFromJSON requires a present attrs
    value to be null or a JSON object and computeAttrs errors on a missing required attribute even when attrs is nil
    (TypeScript computeAttrs, via "value && value[name]", silently null-fills required attributes or discards a
    non-object attrs and falls back to defaults); this same required-attribute strictness applies to Create,
    CreateChecked, CreateAndFill, and MarkType.Create with nil attrs.
15. The toHTML serializer lowercases tag and attribute names, matching the DOM createElement and setAttribute path the
    canonical form is defined against (which lowercases both for HTML elements). It does not deduplicate a repeated
    attribute name (the DOM path would, via setAttribute); no schema lists an attribute name twice, so this is
    unreachable in practice.
16. Node.Copy on a text node with content other than the empty fragment keeps it a text node (yielding an empty text
    node, a consequence of deviation 1), where TypeScript's TextNode inherits Node.copy and returns a base leaf node.
    Copying a text node to other content does not occur in the ported API.
17. parseContentMatch rejects a braced-range bound that overflows a 64-bit integer (for example
    "a{99999999999999999999}") with a clean error, where TypeScript Number() coerces it to a float and then attempts to
    build an astronomically large automaton.
18. parseContentMatch reports an end-of-stream malformed content expression (a trailing operator such as "text|") as
    "Unexpected token ''" rather than the TypeScript "No node type or group 'undefined' found" (a JavaScript
    undefined-coercion artifact); both are errors.
