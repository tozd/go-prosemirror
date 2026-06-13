// Ported from prosemirror-model/src/schema.ts.

package model

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"unicode/utf16"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

// Attrs is an object holding the attributes of a node or mark. Values are JSON-decoded values: nil, bool, float64, string, []any, or map[string]any. Numeric
// values are always float64.
type Attrs map[string]any

// AttrValidator validates a JSON-decoded attribute value, returning an error when the value is invalid.
type AttrValidator func(value any) errors.E

// defaultAttrs builds a single reusable default attribute object for node/mark types where all attributes have a default value (or which do not have any
// attributes), used for all nodes that do not specify attributes. It returns nil when some attribute has no default.
func defaultAttrs(attrs map[string]*Attribute) Attrs {
	defaults := Attrs{}
	for name, attr := range attrs {
		if !attr.HasDefault {
			return nil
		}
		defaults[name] = attr.Default
	}
	return defaults
}

// computeAttrs completes the given attribute values with defaults from the declared attributes, erroring when a value for an attribute without a default is
// missing.
func computeAttrs(attrs map[string]*Attribute, value Attrs) (Attrs, errors.E) {
	built := Attrs{}
	for _, name := range slices.Sorted(maps.Keys(attrs)) {
		given, ok := value[name]
		if !ok {
			attr := attrs[name]
			if !attr.HasDefault {
				errE := errors.New("no value supplied for attribute")
				errors.Details(errE)["attribute"] = name
				return nil, errE
			}
			given = attr.Default
		}
		built[name] = given
	}
	return built, nil
}

// checkAttrs checks the given attribute values against the declared attributes: undeclared attributes, missing declared attributes, and validation failures
// are errors. A value equal to the declared attribute default is always valid without running the validator: the default is what an absent value decodes to
// and what the onInvalid "drop" policy produces during HTML parsing, so it is acceptable by definition. The kind argument is "node" or "mark" and is used in
// error messages together with the type name.
func checkAttrs(attrs map[string]*Attribute, values Attrs, kind, name string) errors.E {
	for _, attr := range slices.Sorted(maps.Keys(values)) {
		if _, ok := attrs[attr]; !ok {
			errE := errors.New("unsupported attribute")
			details := errors.Details(errE)
			details["attribute"] = attr
			details["kind"] = kind
			details["type"] = name
			return errE
		}
	}
	for _, attr := range slices.Sorted(maps.Keys(attrs)) {
		value, ok := values[attr]
		if !ok {
			errE := errors.New("missing attribute")
			details := errors.Details(errE)
			details["attribute"] = attr
			details["kind"] = kind
			details["type"] = name
			return errE
		}
		if attrs[attr].HasDefault && compareDeep(value, attrs[attr].Default) {
			continue
		}
		if attrs[attr].Validate != nil {
			errE := attrs[attr].Validate(value)
			if errE != nil {
				return errE
			}
		}
	}
	return nil
}

// initAttrs compiles the attribute specs of a node or mark type into Attribute values, resolving validator names against the given validator registry.
func initAttrs(typeName string, attrs map[string]*AttributeSpec, validators map[string]AttrValidator) (map[string]*Attribute, errors.E) {
	result := map[string]*Attribute{}
	for _, name := range slices.Sorted(maps.Keys(attrs)) {
		attr, errE := newAttribute(typeName, name, attrs[name], validators)
		if errE != nil {
			return nil, errE
		}
		result[name] = attr
	}
	return result, nil
}

// NodeType objects are allocated once per Schema and used to tag Node instances. They contain information about the node type, such as its name and what kind
// of node it represents. Fields are read only.
type NodeType struct {
	// Name is the name the node type has in this schema.
	Name string

	// Schema is a link back to the Schema the node type belongs to.
	Schema *Schema

	// Spec is the spec that this type is based on.
	Spec *NodeSpec

	// Groups is the list of groups, from the group field of the spec, that this node type belongs to.
	Groups []string

	// Attrs is the compiled form of the attributes declared for this node type.
	Attrs map[string]*Attribute

	// DefaultAttrs holds the default value of every attribute. It is nil when some attribute has no default.
	DefaultAttrs Attrs

	// ContentMatch is the starting match of the node type's content expression.
	ContentMatch *ContentMatch

	// InlineContent is true if this node type has inline content.
	InlineContent bool

	// IsBlock is true if this is a block type.
	IsBlock bool

	// IsText is true if this is the text node type.
	IsText bool

	// MarkSet is the set of marks allowed in this node. nil means all marks are allowed; empty non-nil means none.
	MarkSet []*MarkType
}

// newNodeType creates a node type from its spec. ContentMatch, InlineContent, and MarkSet are filled in later, by NewSchema.
func newNodeType(name string, schema *Schema, spec *NodeSpec, validators map[string]AttrValidator) (*NodeType, errors.E) {
	var groups []string
	if spec.Group != "" {
		groups = strings.Split(spec.Group, " ")
	}
	attrs, errE := initAttrs(name, spec.Attrs, validators)
	if errE != nil {
		return nil, errE
	}
	return &NodeType{
		Name:          name,
		Schema:        schema,
		Spec:          spec,
		Groups:        groups,
		Attrs:         attrs,
		DefaultAttrs:  defaultAttrs(attrs),
		ContentMatch:  nil,
		InlineContent: false,
		IsBlock:       !(spec.Inline || name == "text"), //nolint:goconst,staticcheck
		IsText:        name == "text",
		MarkSet:       nil,
	}, nil
}

// IsInline is true if this is an inline type.
func (nt *NodeType) IsInline() bool {
	return !nt.IsBlock
}

// IsTextblock is true if this is a textblock type, a block that contains inline content.
func (nt *NodeType) IsTextblock() bool {
	return nt.IsBlock && nt.InlineContent
}

// IsLeaf is true for node types that allow no content.
func (nt *NodeType) IsLeaf() bool {
	return nt.ContentMatch == EmptyContentMatch
}

// IsAtom is true when this node is an atom, i.e. when it does not have directly editable content.
func (nt *NodeType) IsAtom() bool {
	return nt.IsLeaf() || nt.Spec.Atom
}

// IsInGroup returns true when this node type is part of the given group.
func (nt *NodeType) IsInGroup(group string) bool {
	return slices.Contains(nt.Groups, group)
}

// Whitespace is the node type's whitespace option, "pre" or "normal".
func (nt *NodeType) Whitespace() string {
	if nt.Spec.Whitespace != "" {
		return nt.Spec.Whitespace
	}
	if nt.Spec.Code {
		return "pre" //nolint:goconst
	}
	return "normal"
}

// HasRequiredAttrs tells you whether this node type has any required attributes.
func (nt *NodeType) HasRequiredAttrs() bool {
	for _, attr := range nt.Attrs {
		if attr.IsRequired() {
			return true
		}
	}
	return false
}

// CompatibleContent indicates whether this node allows some of the same content as the given node type.
func (nt *NodeType) CompatibleContent(other *NodeType) bool {
	return nt == other || nt.ContentMatch.Compatible(other.ContentMatch)
}

// computeAttrs completes the given attribute values with the defaults of this node type, using the shared default attribute object when attrs is nil.
func (nt *NodeType) computeAttrs(attrs Attrs) (Attrs, errors.E) {
	if attrs == nil && nt.DefaultAttrs != nil {
		return nt.DefaultAttrs, nil
	}
	return computeAttrs(nt.Attrs, attrs)
}

// Create creates a Node of this type. The given attributes are checked and defaulted (you can pass nil to use the type's defaults entirely, if no required
// attributes exist). The content may be nil for the empty fragment. Similarly marks may be nil to default to the empty set of marks. Creating a node of the
// text type is an error.
func (nt *NodeType) Create(attrs Attrs, content *Fragment, marks []*Mark) (*Node, errors.E) {
	if nt.IsText {
		return nil, errors.New("cannot construct text nodes")
	}
	computed, errE := nt.computeAttrs(attrs)
	if errE != nil {
		return nil, errE
	}
	if content == nil {
		content = EmptyFragment
	}
	return newNode(nt, computed, content, MarkSetFrom(marks)), nil
}

// CreateChecked is like Create, but checks the given content against the node type's content restrictions, and returns an error if it does not match.
func (nt *NodeType) CreateChecked(attrs Attrs, content *Fragment, marks []*Mark) (*Node, errors.E) {
	if nt.IsText {
		return nil, errors.New("cannot construct text nodes")
	}
	if content == nil {
		content = EmptyFragment
	}
	errE := nt.CheckContent(content)
	if errE != nil {
		return nil, errE
	}
	computed, errE := nt.computeAttrs(attrs)
	if errE != nil {
		return nil, errE
	}
	return newNode(nt, computed, content, MarkSetFrom(marks)), nil
}

// CreateAndFill is like Create, but sees if it is necessary to add nodes to the start or end of the given fragment to make it fit the node. If no fitting
// wrapping can be found, it returns nil (with a nil error). Note that, due to the fact that required nodes can always be created, this will always succeed if
// you pass nil or EmptyFragment as content.
func (nt *NodeType) CreateAndFill(attrs Attrs, content *Fragment, marks []*Mark) (*Node, errors.E) {
	computed, errE := nt.computeAttrs(attrs)
	if errE != nil {
		return nil, errE
	}
	if content == nil {
		content = EmptyFragment
	}
	if content.Size != 0 {
		before := nt.ContentMatch.FillBefore(content, false, 0)
		if before == nil {
			return nil, nil //nolint:nilnil
		}
		content = before.Append(content)
	}
	matched := nt.ContentMatch.MatchFragment(content, 0, content.ChildCount())
	if matched == nil {
		return nil, nil //nolint:nilnil
	}
	after := matched.FillBefore(EmptyFragment, true, 0)
	if after == nil {
		return nil, nil //nolint:nilnil
	}
	return newNode(nt, computed, content.Append(after), MarkSetFrom(marks)), nil
}

// ValidContent returns true if the given fragment is valid content for this node type.
func (nt *NodeType) ValidContent(content *Fragment) bool {
	result := nt.ContentMatch.MatchFragment(content, 0, content.ChildCount())
	if result == nil || !result.ValidEnd {
		return false
	}
	for i := 0; i < content.ChildCount(); i++ { //nolint:intrange
		if !nt.AllowsMarks(content.Child(i).Marks) {
			return false
		}
	}
	return true
}

// CheckContent returns an error when the given fragment is not valid content for this node type.
func (nt *NodeType) CheckContent(content *Fragment) errors.E {
	if !nt.ValidContent(content) {
		str := content.String()
		// Mirror the TypeScript content.toString().slice(0, 50): JavaScript slice counts UTF-16 code units, so a non-BMP code point counts as two.
		if u16 := utf16.Encode([]rune(str)); len(u16) > 50 { //nolint:mnd
			str = string(utf16.Decode(u16[:50]))
		}
		errE := errors.New("invalid content for node")
		details := errors.Details(errE)
		details["type"] = nt.Name
		details["content"] = str
		return errE
	}
	return nil
}

// CheckAttrs checks the given attribute values against the attributes declared for this node type.
func (nt *NodeType) CheckAttrs(attrs Attrs) errors.E {
	return checkAttrs(nt.Attrs, attrs, "node", nt.Name)
}

// AllowsMarkType checks whether the given mark type is allowed in this node.
func (nt *NodeType) AllowsMarkType(markType *MarkType) bool {
	return nt.MarkSet == nil || slices.Contains(nt.MarkSet, markType)
}

// AllowsMarks tests whether the given set of marks are allowed in this node.
func (nt *NodeType) AllowsMarks(marks []*Mark) bool {
	if nt.MarkSet == nil {
		return true
	}
	for _, mark := range marks {
		if !nt.AllowsMarkType(mark.Type) {
			return false
		}
	}
	return true
}

// AllowedMarks removes the marks that are not allowed in this node from the given set.
func (nt *NodeType) AllowedMarks(marks []*Mark) []*Mark {
	if nt.MarkSet == nil {
		return marks
	}
	var copied []*Mark
	copying := false
	for i, mark := range marks {
		if !nt.AllowsMarkType(mark.Type) {
			if !copying {
				copied = append([]*Mark{}, marks[:i]...)
				copying = true
			}
		} else if copying {
			copied = append(copied, mark)
		}
	}
	switch {
	case !copying:
		return marks
	case len(copied) > 0:
		return copied
	default:
		return NoMarks
	}
}

// compileNodeTypes creates the node types of the schema in declaration order and validates them: the top node type must be present, a text type must exist,
// the text type must have no attributes and no parse rules, every node type except the text type and the top node type must have a toHTML spec (which the text
// type and the top node type must not have), and parse rule attributes must reference declared attributes.
func compileNodeTypes(nodes []*NamedNodeSpec, schema *Schema, validators map[string]AttrValidator) (map[string]*NodeType, errors.E) {
	result := map[string]*NodeType{}
	for _, named := range nodes {
		typ, errE := newNodeType(named.Name, schema, named.Spec, validators)
		if errE != nil {
			return nil, errE
		}
		result[named.Name] = typ
	}

	topType := schema.Spec.TopNode
	if topType == "" {
		topType = "doc"
	}
	if result[topType] == nil {
		errE := errors.New("schema is missing its top node type")
		errors.Details(errE)["topNode"] = topType
		return nil, errE
	}
	text := result["text"]
	if text == nil {
		return nil, errors.New("schema must have a text type")
	}
	if len(text.Attrs) > 0 {
		return nil, errors.New("the text node type must not have attributes")
	}

	for _, named := range nodes {
		if named.Name == "text" || named.Name == topType {
			if named.Spec.ToHTML != nil {
				errE := errors.New("node type must not have a toHTML spec")
				errors.Details(errE)["type"] = named.Name
				return nil, errE
			}
		} else if named.Spec.ToHTML == nil {
			errE := errors.New("node type must have a toHTML spec")
			errors.Details(errE)["type"] = named.Name
			return nil, errE
		}
		if named.Name == "text" && len(named.Spec.ParseHTML) > 0 {
			return nil, errors.New("the text node type must not have parseHTML rules")
		}
		for _, rule := range named.Spec.ParseHTML {
			if rule.Style != "" {
				errE := errors.New("style parse rules produce marks and may only appear on mark types")
				errors.Details(errE)["type"] = named.Name
				return nil, errE
			}
		}
		errE := checkParseRuleAttrs(named.Spec.ParseHTML, result[named.Name].Attrs, "node", named.Name)
		if errE != nil {
			return nil, errE
		}
	}

	return result, nil
}

// checkParseRuleAttrs verifies that the attrs of every parse rule reference only attributes declared on the node or mark type the rule belongs to.
func checkParseRuleAttrs(rules []*ParseRule, attrs map[string]*Attribute, kind, name string) errors.E {
	for _, rule := range rules {
		for _, attr := range slices.Sorted(maps.Keys(rule.Attrs)) {
			if _, ok := attrs[attr]; !ok {
				errE := errors.New("parse rule references unknown attribute")
				details := errors.Details(errE)
				details["kind"] = kind
				details["type"] = name
				details["attribute"] = attr
				return errE
			}
		}
	}
	return nil
}

// validateType builds an attribute validator from a "|" separated union of the builtin type names string, number, boolean, and null, checking the type of the
// JSON-decoded value.
func validateType(typeName, attrName, typeStr string) (AttrValidator, errors.E) {
	types := strings.Split(typeStr, "|")
	for _, t := range types {
		switch t {
		case "string", "number", "boolean", "null": //nolint:goconst
		default:
			errE := errors.New("unknown validator")
			details := errors.Details(errE)
			details["validator"] = t
			details["attribute"] = attrName
			details["type"] = typeName
			return nil, errE
		}
	}
	expected := strings.Join(types, ",")
	return func(value any) errors.E {
		name := jsonTypeName(value)
		if !slices.Contains(types, name) {
			errE := errors.New("unexpected attribute value type")
			details := errors.Details(errE)
			details["expected"] = expected
			details["attribute"] = attrName
			details["type"] = typeName
			details["got"] = name
			return errE
		}
		return nil
	}, nil
}

// jsonTypeName mirrors the JavaScript typeof result for a JSON-decoded value, with nil reported as "null".
func jsonTypeName(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	default:
		return "object"
	}
}

// Attribute is the compiled form of an attribute specification. Fields are read only.
type Attribute struct {
	// HasDefault is whether the attribute has a default value.
	HasDefault bool

	// Default is the default value of the attribute, meaningful only when HasDefault is true.
	Default any

	// Validate validates attribute values. It is nil when no validation is configured.
	Validate AttrValidator

	// OnInvalid is "rejectRule" or "drop" and governs HTML parse behavior when extraction of this attribute fails validation.
	OnInvalid string
}

// newAttribute compiles an attribute spec, resolving the validator name against the given validator registry first and against the builtin type unions
// otherwise.
func newAttribute(typeName, attrName string, options *AttributeSpec, validators map[string]AttrValidator) (*Attribute, errors.E) {
	var validate AttrValidator
	if options.Validate != "" {
		if v, ok := validators[options.Validate]; ok {
			validate = v
		} else {
			var errE errors.E
			validate, errE = validateType(typeName, attrName, options.Validate)
			if errE != nil {
				return nil, errE
			}
		}
	}
	onInvalid := options.OnInvalid
	switch onInvalid {
	case "":
		onInvalid = "rejectRule"
	case "rejectRule", "drop": //nolint:goconst
	default:
		errE := errors.New("unknown onInvalid value")
		details := errors.Details(errE)
		details["value"] = onInvalid
		details["attribute"] = attrName
		details["type"] = typeName
		return nil, errE
	}
	if onInvalid == "drop" && !options.HasDefault {
		errE := errors.New("onInvalid drop requires a default")
		details := errors.Details(errE)
		details["attribute"] = attrName
		details["type"] = typeName
		return nil, errE
	}
	return &Attribute{
		HasDefault: options.HasDefault,
		Default:    options.Default,
		Validate:   validate,
		OnInvalid:  onInvalid,
	}, nil
}

// IsRequired is whether the attribute must be supplied when creating a node or mark of the type it belongs to, that is, whether it has no default value.
func (a *Attribute) IsRequired() bool {
	return !a.HasDefault
}

// MarkType is the type object for marks. Like nodes, marks (which are associated with nodes to signify things like emphasis or being part of a link) are
// tagged with type objects, which are instantiated once per Schema. Fields are read only.
type MarkType struct {
	// Name is the name of the mark type.
	Name string

	// Rank is the position of the mark type in the schema declaration order. It determines the sort order of mark sets.
	Rank int

	// Schema is the schema that this mark type instance is part of.
	Schema *Schema

	// Spec is the spec on which the type is based.
	Spec *MarkSpec

	// Attrs is the compiled form of the attributes declared for this mark type.
	Attrs map[string]*Attribute

	// Excluded is the set of mark types excluded by this one.
	Excluded []*MarkType

	instance *Mark
}

// newMarkType creates a mark type from its spec. Excluded is filled in later, by NewSchema.
func newMarkType(name string, rank int, schema *Schema, spec *MarkSpec, validators map[string]AttrValidator) (*MarkType, errors.E) {
	attrs, errE := initAttrs(name, spec.Attrs, validators)
	if errE != nil {
		return nil, errE
	}
	mt := &MarkType{
		Name:     name,
		Rank:     rank,
		Schema:   schema,
		Spec:     spec,
		Attrs:    attrs,
		Excluded: nil,
		instance: nil,
	}
	if defaults := defaultAttrs(attrs); defaults != nil {
		mt.instance = &Mark{Type: mt, Attrs: defaults}
	}
	return mt, nil
}

// Create creates a mark of this type. attrs may be nil or an object containing only some of the mark's attributes. The others, if they have defaults, will be
// added.
func (mt *MarkType) Create(attrs Attrs) (*Mark, errors.E) {
	if attrs == nil && mt.instance != nil {
		return mt.instance, nil
	}
	computed, errE := computeAttrs(mt.Attrs, attrs)
	if errE != nil {
		return nil, errE
	}
	return &Mark{Type: mt, Attrs: computed}, nil
}

// compileMarkTypes creates the mark types of the schema, with rank following declaration order, and validates them: every mark type must have a toHTML spec
// and parse rule attributes must reference declared attributes.
func compileMarkTypes(marks []*NamedMarkSpec, schema *Schema, validators map[string]AttrValidator) (map[string]*MarkType, errors.E) {
	result := map[string]*MarkType{}
	for rank, named := range marks {
		typ, errE := newMarkType(named.Name, rank, schema, named.Spec, validators)
		if errE != nil {
			return nil, errE
		}
		result[named.Name] = typ
	}

	for _, named := range marks {
		if named.Spec.ToHTML == nil {
			errE := errors.New("mark type must have a toHTML spec")
			errors.Details(errE)["type"] = named.Name
			return nil, errE
		}
		errE := checkParseRuleAttrs(named.Spec.ParseHTML, result[named.Name].Attrs, "mark", named.Name)
		if errE != nil {
			return nil, errE
		}
	}

	return result, nil
}

// RemoveFromSet returns, when there is a mark of this type in the given set, a new set without it. Otherwise, the input set is returned.
func (mt *MarkType) RemoveFromSet(set []*Mark) []*Mark {
	for i := 0; i < len(set); i++ {
		if set[i].Type == mt {
			without := make([]*Mark, 0, len(set)-1)
			without = append(without, set[:i]...)
			without = append(without, set[i+1:]...)
			set = without
			i--
		}
	}
	return set
}

// IsInSet returns the mark of this type in the given set, or nil when there is none.
func (mt *MarkType) IsInSet(set []*Mark) *Mark {
	for _, mark := range set {
		if mark.Type == mt {
			return mark
		}
	}
	return nil
}

// CheckAttrs checks the given attribute values against the attributes declared for this mark type.
func (mt *MarkType) CheckAttrs(attrs Attrs) errors.E {
	return checkAttrs(mt.Attrs, attrs, "mark", mt.Name)
}

// Excludes queries whether the given mark type is excluded by this one.
func (mt *MarkType) Excludes(other *MarkType) bool {
	return slices.Contains(mt.Excluded, other)
}

// NamedNodeSpec pairs a node type name with its spec, preserving schema declaration order.
type NamedNodeSpec struct {
	Name string
	Spec *NodeSpec
}

// NamedMarkSpec pairs a mark type name with its spec, preserving schema declaration order.
type NamedMarkSpec struct {
	Name string
	Spec *MarkSpec
}

// SchemaSpec is an object describing a schema, as decoded from the schema spec JSON given to NewSchema.
type SchemaSpec struct {
	// TopNode is the name of the default top-level node for the schema. "" means "doc".
	TopNode string

	// Nodes is the node types in this schema, in declaration order. The order is significant: it determines which parse rules take precedence by default, and
	// which nodes come first in a given group.
	Nodes []*NamedNodeSpec

	// Marks is the mark types that exist in this schema, in declaration order. The order determines the order in which mark sets are sorted and in which
	// parse rules are tried.
	Marks []*NamedMarkSpec
}

// NodeSpec is a description of a node type, used when defining a schema.
type NodeSpec struct {
	// Content is the content expression for this node. When empty, the node does not allow any content.
	Content string

	// Marks determines the marks that are allowed inside of this node. It may be a space-separated string referring to mark names or groups, "_" to
	// explicitly allow all marks, or "" to disallow marks. When nil (not given), nodes with inline content default to allowing all marks, other nodes default
	// to not allowing marks.
	Marks *string

	// Group is the group or space-separated groups to which this node belongs, which can be referred to in the content expressions for the schema.
	Group string

	// Inline should be set to true for inline nodes. (Implied for text nodes.)
	Inline bool

	// Atom can be set to true to indicate that, though this is not a leaf node, it does not have directly editable content and should be treated as a single
	// unit.
	Atom bool

	// Attrs is the attributes that nodes of this type get.
	Attrs map[string]*AttributeSpec

	// Code can be used to indicate that this node contains code, which causes some commands to behave differently.
	Code bool

	// Whitespace controls the way whitespace in this node is parsed. The default is "normal", which causes the HTML parser to collapse whitespace in normal
	// mode, and normalize it (replacing newlines and such with spaces) otherwise. "pre" causes the parser to preserve spaces inside the node. When this option
	// is not given, but Code is true, whitespace will default to "pre". Note that this option does not influence the way the node is rendered.
	Whitespace string

	// LinebreakReplacement can be set on a single inline node in a schema to make it the linebreak equivalent, used when converting between block types that
	// support the node and block types that do not but have Whitespace set to "pre".
	LinebreakReplacement bool

	// ToHTML defines the way a node of this type is serialized to HTML. It is required for every node type except the text type and the top node type, which
	// must not have it.
	ToHTML *ToHTMLSpec

	// ParseHTML associates HTML parser information with this node, used to derive the schema's parser. The node name in the rules is implied (the name of
	// this node will be filled in automatically).
	ParseHTML []*ParseRule
}

// MarkSpec is used to define marks when creating a schema.
type MarkSpec struct {
	// Attrs is the attributes that marks of this type get.
	Attrs map[string]*AttributeSpec

	// Excludes determines which other marks this mark can coexist with. It should be a space-separated string naming other marks or groups of marks. When a
	// mark is added to a set, all marks that it excludes are removed in the process. If the set contains any mark that excludes the new mark but is not,
	// itself, excluded by the new mark, the mark can not be added to the set. You can use the value "_" to indicate that the mark excludes all marks in the
	// schema.
	//
	// When nil (not given), defaults to only being exclusive with marks of the same type. You can set it to an empty string (or any string not containing the
	// mark's own name) to allow multiple marks of a given type to coexist (as long as they have different attributes).
	Excludes *string

	// Group is the group or space-separated groups to which this mark belongs.
	Group string

	// Code marks the content of this span as being code, which causes some commands and extensions to treat it differently.
	Code bool

	// Spanning determines whether marks of this type can span multiple adjacent nodes when serialized to HTML. nil means the default, true.
	Spanning *bool

	// ToHTML defines the way marks of this type are serialized to HTML. It is required for every mark type.
	ToHTML *ToHTMLSpec

	// ParseHTML associates HTML parser information with this mark. The mark name in the rules is implied.
	ParseHTML []*ParseRule
}

// AttributeSpec is used to define attributes on nodes or marks.
type AttributeSpec struct {
	// Default is the default value for this attribute, to use when no explicit value is provided. Attributes that have no default must be provided whenever a
	// node or mark of a type that has them is created.
	Default any

	// HasDefault is whether the "default" key was present in the JSON spec.
	HasDefault bool

	// Validate names a validator for values of this attribute, used when deserializing from JSON, when running Node.Check, and when extracting attributes
	// during HTML parsing. The name is looked up first in the validator registry passed to NewSchema; otherwise it must be a "|" separated string of the
	// primitive type names "string", "number", "boolean", and "null", and values not of one of those types are rejected. Empty means no validation.
	Validate string

	// OnInvalid governs HTML parse behavior when extraction of this attribute fails validation: "rejectRule" (the default when empty) rejects the whole parse
	// rule while "drop" replaces the value with the default. "drop" requires the attribute to have a default.
	OnInvalid string
}

// parseSchemaSpec strictly decodes the schema spec JSON, preserving the declaration order of the nodes and marks objects. Unknown top level keys are errors.
func parseSchemaSpec(specJSON []byte) (*SchemaSpec, errors.E) {
	var top map[string]json.RawMessage
	errE := x.UnmarshalWithoutUnknownFields(specJSON, &top)
	if errE != nil {
		return nil, errE
	}
	spec := &SchemaSpec{}
	var nodesRaw, marksRaw json.RawMessage
	for _, key := range slices.Sorted(maps.Keys(top)) {
		value := top[key]
		switch key {
		case "topNode":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.TopNode)
			if errE != nil {
				return nil, errors.Wrap(errE, "invalid topNode in schema spec")
			}
		case "nodes":
			nodesRaw = value
		case "marks":
			marksRaw = value
		default:
			errE := errors.New("unknown key in schema spec")
			errors.Details(errE)["key"] = key
			return nil, errE
		}
	}
	if nodesRaw == nil {
		return nil, errors.New("missing nodes in schema spec")
	}
	nodeNames, nodeValues, errE := decodeOrderedObject(nodesRaw, "nodes")
	if errE != nil {
		return nil, errE
	}
	for i, name := range nodeNames {
		nodeSpec, errE := parseNodeSpec(name, nodeValues[i])
		if errE != nil {
			return nil, errE
		}
		spec.Nodes = append(spec.Nodes, &NamedNodeSpec{Name: name, Spec: nodeSpec})
	}
	if marksRaw != nil {
		markNames, markValues, errE := decodeOrderedObject(marksRaw, "marks")
		if errE != nil {
			return nil, errE
		}
		for i, name := range markNames {
			markSpec, errE := parseMarkSpec(name, markValues[i])
			if errE != nil {
				return nil, errE
			}
			spec.Marks = append(spec.Marks, &NamedMarkSpec{Name: name, Spec: markSpec})
		}
	}
	return spec, nil
}

// decodeOrderedObject decodes a JSON object into parallel slices of keys and raw values, preserving the declaration order of the keys. Duplicate keys are
// errors. The what argument names the object in error messages.
func decodeOrderedObject(raw json.RawMessage, what string) ([]string, []json.RawMessage, errors.E) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid schema spec object")
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		errE := errors.New("expected an object in schema spec")
		errors.Details(errE)["where"] = what
		return nil, nil, errE
	}
	var names []string
	var values []json.RawMessage
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, nil, errors.Wrap(err, "invalid schema spec object")
		}
		name, ok := keyTok.(string)
		if !ok {
			errE := errors.New("expected a string key in schema spec")
			errors.Details(errE)["where"] = what
			return nil, nil, errE
		}
		if seen[name] {
			errE := errors.New("duplicate key in schema spec")
			details := errors.Details(errE)
			details["key"] = name
			details["where"] = what
			return nil, nil, errE
		}
		seen[name] = true
		var value json.RawMessage
		err = dec.Decode(&value)
		if err != nil {
			return nil, nil, errors.Wrap(err, "invalid schema spec object")
		}
		names = append(names, name)
		values = append(values, value)
	}
	_, err = dec.Token()
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid schema spec object")
	}
	return names, values, nil
}

// parseNodeSpec strictly decodes a single node spec object. Editor-only keys are accepted and ignored. Unknown keys are errors.
func parseNodeSpec(name string, raw json.RawMessage) (*NodeSpec, errors.E) {
	var fields map[string]json.RawMessage
	errE := x.UnmarshalWithoutUnknownFields(raw, &fields)
	if errE != nil {
		errE = errors.Wrap(errE, "invalid node spec")
		errors.Details(errE)["type"] = name
		return nil, errE
	}
	if fields == nil {
		errE = errors.New("node spec must be an object")
		errors.Details(errE)["type"] = name
		return nil, errE
	}
	spec := &NodeSpec{}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value := fields[key]
		var errE errors.E
		switch key {
		case "content":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Content)
		case "marks":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Marks)
		case "group":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Group)
		case "inline":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Inline)
		case "atom":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Atom)
		case "attrs": //nolint:goconst
			attrs, errE := parseAttributeSpecs(value, "node type", name)
			if errE != nil {
				return nil, errE
			}
			spec.Attrs = attrs
		case "code":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Code)
		case "whitespace":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Whitespace)
			if errE == nil && spec.Whitespace != "pre" && spec.Whitespace != "normal" {
				errE = errors.New("invalid whitespace value")
				details := errors.Details(errE)
				details["value"] = spec.Whitespace
				details["type"] = name
				return nil, errE
			}
		case "linebreakReplacement":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.LinebreakReplacement)
		case "toHTML":
			if string(value) != "null" {
				spec.ToHTML = &ToHTMLSpec{}
				errE = x.UnmarshalWithoutUnknownFields(value, spec.ToHTML)
			}
		case "parseHTML":
			spec.ParseHTML, errE = decodeParseRules(value)
		case "selectable", "draggable", "defining", "isolating", "definingAsContext", "definingForContent":
			// Editor-only keys are accepted and ignored.
		default:
			errE = errors.New("unknown key in node spec")
			details := errors.Details(errE)
			details["key"] = key
			details["type"] = name
			return nil, errE
		}
		if errE != nil {
			errE = errors.Wrap(errE, "invalid value for key in node spec")
			details := errors.Details(errE)
			details["key"] = key
			details["type"] = name
			return nil, errE
		}
	}
	return spec, nil
}

// parseMarkSpec strictly decodes a single mark spec object. Editor-only keys are accepted and ignored. Unknown keys are errors.
func parseMarkSpec(name string, raw json.RawMessage) (*MarkSpec, errors.E) {
	var fields map[string]json.RawMessage
	errE := x.UnmarshalWithoutUnknownFields(raw, &fields)
	if errE != nil {
		errE = errors.Wrap(errE, "invalid mark spec")
		errors.Details(errE)["type"] = name
		return nil, errE
	}
	if fields == nil {
		errE = errors.New("mark spec must be an object")
		errors.Details(errE)["type"] = name
		return nil, errE
	}
	spec := &MarkSpec{}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value := fields[key]
		var errE errors.E
		switch key {
		case "attrs":
			attrs, errE := parseAttributeSpecs(value, "mark type", name)
			if errE != nil {
				return nil, errE
			}
			spec.Attrs = attrs
		case "excludes":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Excludes)
		case "group":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Group)
		case "code":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Code)
		case "spanning":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Spanning)
		case "toHTML":
			if string(value) != "null" {
				spec.ToHTML = &ToHTMLSpec{}
				errE = x.UnmarshalWithoutUnknownFields(value, spec.ToHTML)
			}
		case "parseHTML":
			spec.ParseHTML, errE = decodeParseRules(value)
		case "inclusive":
			// Editor-only key is accepted and ignored.
		default:
			errE = errors.New("unknown key in mark spec")
			details := errors.Details(errE)
			details["key"] = key
			details["type"] = name
			return nil, errE
		}
		if errE != nil {
			errE = errors.Wrap(errE, "invalid value for key in mark spec")
			details := errors.Details(errE)
			details["key"] = key
			details["type"] = name
			return nil, errE
		}
	}
	return spec, nil
}

// parseAttributeSpecs strictly decodes the attrs object of a node or mark spec. The kind and typeName arguments name the owner in error messages.
func parseAttributeSpecs(raw json.RawMessage, kind, typeName string) (map[string]*AttributeSpec, errors.E) {
	var fields map[string]json.RawMessage
	errE := x.UnmarshalWithoutUnknownFields(raw, &fields)
	if errE != nil {
		errE = errors.Wrap(errE, "invalid attrs in spec")
		details := errors.Details(errE)
		details["kind"] = kind
		details["type"] = typeName
		return nil, errE
	}
	result := map[string]*AttributeSpec{}
	for _, attrName := range slices.Sorted(maps.Keys(fields)) {
		spec, errE := parseAttributeSpec(fields[attrName], attrName, kind, typeName)
		if errE != nil {
			return nil, errE
		}
		result[attrName] = spec
	}
	return result, nil
}

// decodeParseRules strictly decodes a parseHTML array. encoding/json decodes a JSON null array element to a nil rule without calling
// ParseRule.UnmarshalJSON, so nil entries are rejected here to reach the same error the missing-tag guard would.
func decodeParseRules(value json.RawMessage) ([]*ParseRule, errors.E) {
	var rules []*ParseRule
	errE := x.UnmarshalWithoutUnknownFields(value, &rules)
	if errE != nil {
		return nil, errE
	}
	for _, rule := range rules {
		if rule == nil {
			return nil, errors.New("tag parse rule is missing a tag")
		}
	}
	return rules, nil
}

// parseAttributeSpec strictly decodes a single attribute spec object. HasDefault records whether the "default" key was present.
func parseAttributeSpec(raw json.RawMessage, attrName, kind, typeName string) (*AttributeSpec, errors.E) {
	var fields map[string]json.RawMessage
	errE := x.UnmarshalWithoutUnknownFields(raw, &fields)
	if errE != nil {
		errE = errors.Wrap(errE, "invalid attribute spec")
		details := errors.Details(errE)
		details["attribute"] = attrName
		details["kind"] = kind
		details["type"] = typeName
		return nil, errE
	}
	if fields == nil {
		errE = errors.New("attribute spec must be an object")
		details := errors.Details(errE)
		details["attribute"] = attrName
		details["kind"] = kind
		details["type"] = typeName
		return nil, errE
	}
	spec := &AttributeSpec{}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value := fields[key]
		var errE errors.E
		switch key {
		case "default":
			spec.HasDefault = true
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Default)
		case "validate":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.Validate)
		case "onInvalid":
			errE = x.UnmarshalWithoutUnknownFields(value, &spec.OnInvalid)
		default:
			errE = errors.New("unknown key in attribute spec")
			details := errors.Details(errE)
			details["key"] = key
			details["attribute"] = attrName
			details["kind"] = kind
			details["type"] = typeName
			return nil, errE
		}
		if errE != nil {
			errE = errors.Wrap(errE, "invalid value for key in attribute spec")
			details := errors.Details(errE)
			details["key"] = key
			details["attribute"] = attrName
			details["kind"] = kind
			details["type"] = typeName
			return nil, errE
		}
	}
	return spec, nil
}

// Schema is a document schema. It holds node and mark type objects for the nodes and marks that may occur in conforming documents, and provides functionality
// for creating and deserializing such documents. Fields are read only.
type Schema struct {
	// Spec is the spec on which the schema is based, with nodes and marks in declaration order.
	Spec *SchemaSpec

	// Nodes is an object mapping the schema's node names to node type objects.
	Nodes map[string]*NodeType

	// Marks is a map from mark names to mark type objects.
	Marks map[string]*MarkType

	// TopNodeType is the type of the default top node for this schema.
	TopNodeType *NodeType

	// LinebreakReplacement is the linebreak replacement node defined in this schema, if any.
	LinebreakReplacement *NodeType

	nodeOrder []string
	markOrder []string
	domParser *DOMParser
}

// NewSchema constructs a schema from its JSON specification. The validators map registers named attribute validators which attribute specs can reference by
// name; it may be nil when no named validators are used.
func NewSchema(specJSON []byte, validators map[string]AttrValidator) (*Schema, errors.E) {
	spec, errE := parseSchemaSpec(specJSON)
	if errE != nil {
		return nil, errE
	}
	s := &Schema{
		Spec:                 spec,
		Nodes:                nil,
		Marks:                nil,
		TopNodeType:          nil,
		LinebreakReplacement: nil,
		nodeOrder:            make([]string, 0, len(spec.Nodes)),
		markOrder:            make([]string, 0, len(spec.Marks)),
		domParser:            nil,
	}
	for _, named := range spec.Nodes {
		s.nodeOrder = append(s.nodeOrder, named.Name)
	}
	for _, named := range spec.Marks {
		s.markOrder = append(s.markOrder, named.Name)
	}

	s.Nodes, errE = compileNodeTypes(spec.Nodes, s, validators)
	if errE != nil {
		return nil, errE
	}
	s.Marks, errE = compileMarkTypes(spec.Marks, s, validators)
	if errE != nil {
		return nil, errE
	}

	contentExprCache := map[string]*ContentMatch{}
	for _, name := range s.nodeOrder {
		if _, ok := s.Marks[name]; ok {
			errE := errors.New("type is both a node and a mark")
			errors.Details(errE)["type"] = name
			return nil, errE
		}
		typ := s.Nodes[name]
		contentExpr := typ.Spec.Content
		markExpr := typ.Spec.Marks
		contentMatch, ok := contentExprCache[contentExpr]
		if !ok {
			contentMatch, errE = parseContentMatch(contentExpr, s)
			if errE != nil {
				return nil, errE
			}
			contentExprCache[contentExpr] = contentMatch
		}
		typ.ContentMatch = contentMatch
		typ.InlineContent = contentMatch.InlineContent()
		if typ.Spec.LinebreakReplacement {
			if s.LinebreakReplacement != nil {
				return nil, errors.New("multiple linebreak nodes defined")
			}
			if !typ.IsInline() || !typ.IsLeaf() {
				return nil, errors.New("linebreak replacement nodes must be inline leaf nodes")
			}
			s.LinebreakReplacement = typ
		}
		switch {
		case markExpr != nil && *markExpr == "_":
			typ.MarkSet = nil
		case markExpr != nil && *markExpr != "":
			typ.MarkSet, errE = gatherMarks(s, strings.Split(*markExpr, " "))
			if errE != nil {
				return nil, errE
			}
		case (markExpr != nil && *markExpr == "") || !typ.InlineContent:
			typ.MarkSet = []*MarkType{}
		default:
			typ.MarkSet = nil
		}
	}
	for _, name := range s.markOrder {
		typ := s.Marks[name]
		excludes := typ.Spec.Excludes
		switch {
		case excludes == nil:
			typ.Excluded = []*MarkType{typ}
		case *excludes == "":
			typ.Excluded = []*MarkType{}
		default:
			typ.Excluded, errE = gatherMarks(s, strings.Split(*excludes, " "))
			if errE != nil {
				return nil, errE
			}
		}
	}

	topNode := spec.TopNode
	if topNode == "" {
		topNode = "doc"
	}
	s.TopNodeType = s.Nodes[topNode]

	rules := schemaRules(s)
	s.domParser, errE = newDOMParser(s, rules)
	if errE != nil {
		return nil, errE
	}
	return s, nil
}

// Node creates a node in this schema. The node type is looked up by name. Attributes will be extended with defaults, content may be nil or a slice of nodes.
// The content is checked against the content restrictions of the node type.
func (s *Schema) Node(typeName string, attrs Attrs, content []*Node, marks []*Mark) (*Node, errors.E) {
	typ, errE := s.NodeType(typeName)
	if errE != nil {
		return nil, errE
	}
	return typ.CreateChecked(attrs, FragmentFromArray(content), marks)
}

// Text creates a text node in the schema. Empty text nodes are not allowed; it panics on an empty text string.
func (s *Schema) Text(text string, marks []*Mark) *Node {
	typ := s.Nodes["text"]
	return newTextNode(typ, typ.DefaultAttrs, text, MarkSetFrom(marks))
}

// Mark creates a mark with the given type name and attributes.
func (s *Schema) Mark(typeName string, attrs Attrs) (*Mark, errors.E) {
	typ, ok := s.Marks[typeName]
	if !ok {
		errE := errors.New("unknown mark type")
		errors.Details(errE)["type"] = typeName
		return nil, errE
	}
	return typ.Create(attrs)
}

// NodeFromJSON deserializes a node from its JSON representation and fully validates it.
func (s *Schema) NodeFromJSON(data []byte) (*Node, errors.E) {
	var value any
	errE := x.UnmarshalWithoutUnknownFields(data, &value)
	if errE != nil {
		return nil, errE
	}
	node, errE := nodeFromJSON(s, value)
	if errE != nil {
		return nil, errE
	}
	errE = node.Check()
	if errE != nil {
		return nil, errE
	}
	return node, nil
}

// MarkFromJSON deserializes a mark from its JSON representation.
func (s *Schema) MarkFromJSON(data []byte) (*Mark, errors.E) {
	var value any
	errE := x.UnmarshalWithoutUnknownFields(data, &value)
	if errE != nil {
		return nil, errE
	}
	return markFromJSON(s, value)
}

// NodeType returns the node type with the given name, or an error when the schema does not have a node type with that name.
func (s *Schema) NodeType(name string) (*NodeType, errors.E) {
	found, ok := s.Nodes[name]
	if !ok {
		errE := errors.New("unknown node type")
		errors.Details(errE)["type"] = name
		return nil, errE
	}
	return found, nil
}

// gatherMarks resolves a list of mark names and mark group names ("_" means all marks) into mark types, expanding group names in schema declaration order.
// Unknown names are errors.
func gatherMarks(schema *Schema, marks []string) ([]*MarkType, errors.E) {
	found := []*MarkType{}
	for _, name := range marks {
		mark, ok := schema.Marks[name]
		if ok {
			found = append(found, mark)
		} else {
			for _, prop := range schema.markOrder {
				m := schema.Marks[prop]
				if name == "_" || (m.Spec.Group != "" && slices.Contains(strings.Split(m.Spec.Group, " "), name)) {
					found = append(found, m)
					ok = true
				}
			}
		}
		if !ok {
			errE := errors.New("unknown mark type")
			errors.Details(errE)["type"] = name
			return nil, errE
		}
	}
	return found, nil
}
