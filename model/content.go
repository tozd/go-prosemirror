// Ported from prosemirror-model/src/content.ts.

package model

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"gitlab.com/tozd/go/errors"
)

// MatchEdge is an outgoing edge of a ContentMatch in the finite automaton that describes a content expression: a node type and the match state after it.
type MatchEdge struct {
	Type *NodeType
	Next *ContentMatch
}

// wrapCacheEntry pairs a wrapping target with the computed wrapping (nil when no wrapping exists).
type wrapCacheEntry struct {
	target   *NodeType
	computed []*NodeType
}

// ContentMatch represents a match state of a node type's content expression, and can be used to find out whether further content matches here, and whether
// a given position is a valid end of the node.
type ContentMatch struct {
	// ValidEnd is true when this match state represents a valid end of the node. Read only.
	ValidEnd bool

	next []MatchEdge

	wrapCacheMu sync.Mutex
	wrapCache   []wrapCacheEntry
}

// parseContentMatch parses a content expression into a ContentMatch automaton, resolving node type and group names against the given schema.
func parseContentMatch(expr string, schema *Schema) (*ContentMatch, errors.E) {
	stream := newTokenStream(expr, schema)
	if stream.next() == "" {
		return EmptyContentMatch, nil
	}
	parsed, errE := parseExpr(stream)
	if errE != nil {
		return nil, errE
	}
	if stream.next() != "" {
		return nil, stream.err("unexpected trailing text")
	}
	match := dfa(nfa(parsed))
	errE = checkForDeadEnds(match, stream)
	if errE != nil {
		return nil, errE
	}
	return match, nil
}

// MatchType matches a node type, returning a match after that node if successful, or nil otherwise.
func (cm *ContentMatch) MatchType(typ *NodeType) *ContentMatch {
	for i := 0; i < len(cm.next); i++ { //nolint:intrange
		if cm.next[i].Type == typ {
			return cm.next[i].Next
		}
	}
	return nil
}

// MatchFragment tries to match the children of the given fragment between the start and end child indexes. It returns the resulting match when successful,
// or nil otherwise.
func (cm *ContentMatch) MatchFragment(frag *Fragment, start, end int) *ContentMatch {
	cur := cm
	for i := start; cur != nil && i < end; i++ {
		cur = cur.MatchType(frag.Child(i).Type)
	}
	return cur
}

// InlineContent reports whether this match state expects inline content.
func (cm *ContentMatch) InlineContent() bool {
	return len(cm.next) != 0 && cm.next[0].Type.IsInline()
}

// DefaultType returns the first matching node type at this match position that can be generated, or nil when there is none.
func (cm *ContentMatch) DefaultType() *NodeType {
	for i := 0; i < len(cm.next); i++ { //nolint:intrange
		typ := cm.next[i].Type
		if !(typ.IsText || typ.HasRequiredAttrs()) { //nolint:staticcheck
			return typ
		}
	}
	return nil
}

// Compatible reports whether this match state and the given one share a matching node type.
func (cm *ContentMatch) Compatible(other *ContentMatch) bool {
	for i := 0; i < len(cm.next); i++ { //nolint:intrange
		for j := 0; j < len(other.next); j++ { //nolint:intrange
			if cm.next[i].Type == other.next[j].Type {
				return true
			}
		}
	}
	return false
}

// FillBefore tries to match the given fragment, and if that fails, sees if it can be made to match by inserting nodes in front of it. When successful, it
// returns a fragment of inserted nodes (which may be empty if nothing had to be inserted). It returns nil when no fill exists. When toEnd is true, it only
// returns a fragment if the resulting match goes to the end of the content expression.
func (cm *ContentMatch) FillBefore(after *Fragment, toEnd bool, startIndex int) *Fragment {
	seen := []*ContentMatch{cm}
	var search func(match *ContentMatch, types []*NodeType) *Fragment
	search = func(match *ContentMatch, types []*NodeType) *Fragment {
		finished := match.MatchFragment(after, startIndex, after.ChildCount())
		if finished != nil && (!toEnd || finished.ValidEnd) {
			nodes := make([]*Node, len(types))
			for i, tp := range types {
				// checkForDeadEnds guarantees that CreateAndFill on a generatable type can neither error nor come up empty here.
				node, errE := tp.CreateAndFill(nil, nil, nil)
				if errE != nil {
					panic(errE)
				}
				if node == nil {
					errE = errors.New("cannot create filler node")
					errors.Details(errE)["type"] = tp.Name
					panic(errE)
				}
				nodes[i] = node
			}
			return FragmentFromArray(nodes)
		}
		for i := 0; i < len(match.next); i++ { //nolint:intrange
			typ, next := match.next[i].Type, match.next[i].Next
			if !(typ.IsText || typ.HasRequiredAttrs()) && !slices.Contains(seen, next) { //nolint:staticcheck
				seen = append(seen, next)
				withType := make([]*NodeType, len(types)+1)
				copy(withType, types)
				withType[len(types)] = typ
				found := search(next, withType)
				if found != nil {
					return found
				}
			}
		}
		return nil
	}
	return search(cm, nil)
}

// FindWrapping finds a set of wrapping node types that would allow a node of the given type to appear at this position. The result may be empty (when it
// fits directly) and will be nil when no such wrapping exists.
func (cm *ContentMatch) FindWrapping(target *NodeType) []*NodeType {
	cm.wrapCacheMu.Lock()
	defer cm.wrapCacheMu.Unlock()
	for _, entry := range cm.wrapCache {
		if entry.target == target {
			return entry.computed
		}
	}
	computed := cm.computeWrapping(target)
	cm.wrapCache = append(cm.wrapCache, wrapCacheEntry{target: target, computed: computed})
	return computed
}

func (cm *ContentMatch) computeWrapping(target *NodeType) []*NodeType {
	type activeEntry struct {
		match *ContentMatch
		typ   *NodeType
		via   *activeEntry
	}
	seen := map[string]bool{}
	active := []*activeEntry{{match: cm, typ: nil, via: nil}}
	for len(active) > 0 {
		current := active[0]
		active = active[1:]
		match := current.match
		if match.MatchType(target) != nil {
			result := []*NodeType{}
			for obj := current; obj.typ != nil; obj = obj.via {
				result = append(result, obj.typ)
			}
			slices.Reverse(result)
			return result
		}
		for i := 0; i < len(match.next); i++ { //nolint:intrange
			typ, next := match.next[i].Type, match.next[i].Next
			if !typ.IsLeaf() && !typ.HasRequiredAttrs() && !seen[typ.Name] && (current.typ == nil || next.ValidEnd) {
				active = append(active, &activeEntry{match: typ.ContentMatch, typ: typ, via: current})
				seen[typ.Name] = true
			}
		}
	}
	return nil
}

// EdgeCount returns the number of outgoing edges this node has in the finite automaton that describes the content expression.
func (cm *ContentMatch) EdgeCount() int {
	return len(cm.next)
}

// Edge returns the nth outgoing edge from this node in the finite automaton that describes the content expression. It panics when n is out of range.
func (cm *ContentMatch) Edge(n int) MatchEdge {
	if n >= len(cm.next) {
		errE := errors.New("no such edge in this content match")
		errors.Details(errE)["edge"] = n
		panic(errE)
	}
	return cm.next[n]
}

// String returns a debugging string that describes this match state and all states reachable from it.
func (cm *ContentMatch) String() string {
	var seen []*ContentMatch
	var scan func(m *ContentMatch)
	scan = func(m *ContentMatch) {
		seen = append(seen, m)
		for i := 0; i < len(m.next); i++ { //nolint:intrange
			if !slices.Contains(seen, m.next[i].Next) {
				scan(m.next[i].Next)
			}
		}
	}
	scan(cm)
	lines := make([]string, len(seen))
	for i, m := range seen {
		out := strconv.Itoa(i)
		if m.ValidEnd {
			out += "* "
		} else {
			out += "  "
		}
		for j := 0; j < len(m.next); j++ { //nolint:intrange
			if j > 0 {
				out += ", " //nolint:modernize
			}
			out += m.next[j].Type.Name + "->" + strconv.Itoa(slices.Index(seen, m.next[j].Next)) //nolint:perfsprint
		}
		lines[i] = out
	}
	return strings.Join(lines, "\n")
}

// EmptyContentMatch is the match state of an empty content expression: it matches no further content and is a valid end.
var EmptyContentMatch = &ContentMatch{ValidEnd: true} //nolint:exhaustruct,gochecknoglobals

type tokenStream struct {
	str    string
	schema *Schema
	inline *bool
	pos    int
	tokens []string
}

func newTokenStream(str string, schema *Schema) *tokenStream {
	return &tokenStream{str: str, schema: schema, inline: nil, pos: 0, tokens: tokenize(str)}
}

func isWordChar(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isWordToken(tok string) bool {
	if tok == "" {
		return false
	}
	for i := 0; i < len(tok); i++ { //nolint:intrange
		if !isWordChar(tok[i]) {
			return false
		}
	}
	return true
}

// tokenize splits a content expression into tokens: Unicode whitespace separates tokens (mirroring JavaScript "\s"), a token is either a maximal run of
// word characters (letters, digits, and underscore) or a single non-word character.
func tokenize(str string) []string {
	var tokens []string
	for i := 0; i < len(str); {
		c := str[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++
		case isWordChar(c):
			start := i
			for i < len(str) && isWordChar(str[i]) {
				i++
			}
			tokens = append(tokens, str[start:i])
		case c < utf8.RuneSelf:
			tokens = append(tokens, str[i:i+1])
			i++
		default:
			r, size := utf8.DecodeRuneInString(str[i:])
			// JavaScript "\s", used by the reference token stream, treats all Unicode whitespace plus U+FEFF as separators. unicode.IsSpace covers every
			// case except U+FEFF, which is handled explicitly.
			if unicode.IsSpace(r) || r == '\ufeff' {
				i += size
			} else {
				tokens = append(tokens, str[i:i+size])
				i += size
			}
		}
	}
	return tokens
}

// next returns the current token, or an empty string at the end of the stream.
func (ts *tokenStream) next() string {
	if ts.pos >= len(ts.tokens) {
		return ""
	}
	return ts.tokens[ts.pos]
}

func (ts *tokenStream) eat(tok string) bool {
	if ts.next() == tok {
		ts.pos++
		return true
	}
	return false
}

func (ts *tokenStream) err(message string, kv ...any) errors.E {
	errE := errors.New(message)

	details := errors.Details(errE)
	details["expression"] = ts.str

	if len(kv)%2 != 0 {
		panic(errors.New("odd number of arguments for initial details"))
	}

	for i := 0; i < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			errE := errors.New("key must be a string")
			errors.Details(errE)["value"] = kv[i]
			errors.Details(errE)["type"] = fmt.Sprintf("%T", kv[i])
			panic(errE)
		}
		details[key] = kv[i+1] //nolint:gosec
	}

	return errE
}

type exprKind int

const (
	exprChoice exprKind = iota
	exprSeq
	exprPlus
	exprStar
	exprOpt
	exprRange
	exprName
)

// expr is a parsed content expression. The kind selects which fields are meaningful: exprs for choice and seq, expr for plus, star, opt, and range,
// min and max for range (max -1 means unbounded), and value for name.
type expr struct {
	kind  exprKind
	exprs []*expr
	expr  *expr
	min   int
	max   int
	value *NodeType
}

func parseExpr(stream *tokenStream) (*expr, errors.E) {
	var exprs []*expr
	for {
		e, errE := parseExprSeq(stream)
		if errE != nil {
			return nil, errE
		}
		exprs = append(exprs, e)
		if !stream.eat("|") {
			break
		}
	}
	if len(exprs) == 1 {
		return exprs[0], nil
	}
	return &expr{kind: exprChoice, exprs: exprs}, nil //nolint:exhaustruct
}

func parseExprSeq(stream *tokenStream) (*expr, errors.E) {
	var exprs []*expr
	for {
		e, errE := parseExprSubscript(stream)
		if errE != nil {
			return nil, errE
		}
		exprs = append(exprs, e)
		if next := stream.next(); next == "" || next == ")" || next == "|" {
			break
		}
	}
	if len(exprs) == 1 {
		return exprs[0], nil
	}
	return &expr{kind: exprSeq, exprs: exprs}, nil //nolint:exhaustruct
}

func parseExprSubscript(stream *tokenStream) (*expr, errors.E) {
	e, errE := parseExprAtom(stream)
	if errE != nil {
		return nil, errE
	}
	for {
		if stream.eat("+") {
			e = &expr{kind: exprPlus, expr: e} //nolint:exhaustruct
		} else if stream.eat("*") {
			e = &expr{kind: exprStar, expr: e} //nolint:exhaustruct
		} else if stream.eat("?") {
			e = &expr{kind: exprOpt, expr: e} //nolint:exhaustruct
		} else if stream.eat("{") {
			e, errE = parseExprRange(stream, e)
			if errE != nil {
				return nil, errE
			}
		} else {
			break
		}
	}
	return e, nil
}

func parseNum(stream *tokenStream) (int, errors.E) {
	tok := stream.next()
	allDigits := tok != ""
	for i := 0; i < len(tok); i++ { //nolint:intrange
		if tok[i] < '0' || tok[i] > '9' {
			allDigits = false
			break
		}
	}
	if !allDigits {
		return 0, stream.err("expected a number", "got", tok)
	}
	result, err := strconv.Atoi(tok)
	if err != nil {
		return 0, stream.err("expected a number", "got", tok)
	}
	stream.pos++
	return result, nil
}

func parseExprRange(stream *tokenStream, e *expr) (*expr, errors.E) {
	minVal, errE := parseNum(stream)
	if errE != nil {
		return nil, errE
	}
	maxVal := minVal
	if stream.eat(",") {
		if stream.next() != "}" {
			maxVal, errE = parseNum(stream)
			if errE != nil {
				return nil, errE
			}
		} else {
			maxVal = -1
		}
	}
	if !stream.eat("}") {
		return nil, stream.err("unclosed braced range")
	}
	return &expr{kind: exprRange, min: minVal, max: maxVal, expr: e}, nil //nolint:exhaustruct
}

// resolveName resolves a name to the node type of that name, or to all node types (in schema declaration order) whose groups contain the name.
func resolveName(stream *tokenStream, name string) ([]*NodeType, errors.E) {
	if typ, ok := stream.schema.Nodes[name]; ok {
		return []*NodeType{typ}, nil
	}
	var result []*NodeType
	for _, typeName := range stream.schema.nodeOrder {
		typ := stream.schema.Nodes[typeName]
		if typ.IsInGroup(name) {
			result = append(result, typ)
		}
	}
	if len(result) == 0 {
		return nil, stream.err("no node type or group with this name", "name", name)
	}
	return result, nil
}

func parseExprAtom(stream *tokenStream) (*expr, errors.E) {
	if stream.eat("(") {
		e, errE := parseExpr(stream)
		if errE != nil {
			return nil, errE
		}
		if !stream.eat(")") {
			return nil, stream.err("missing closing paren")
		}
		return e, nil
	} else if isWordToken(stream.next()) {
		types, errE := resolveName(stream, stream.next())
		if errE != nil {
			return nil, errE
		}
		exprs := make([]*expr, 0, len(types))
		for _, typ := range types {
			if stream.inline == nil {
				inline := typ.IsInline()
				stream.inline = &inline
			} else if *stream.inline != typ.IsInline() {
				return nil, stream.err("mixing inline and block content")
			}
			exprs = append(exprs, &expr{kind: exprName, value: typ}) //nolint:exhaustruct
		}
		stream.pos++
		if len(exprs) == 1 {
			return exprs[0], nil
		}
		return &expr{kind: exprChoice, exprs: exprs}, nil //nolint:exhaustruct
	}
	return nil, stream.err("unexpected token", "token", stream.next())
}

// The code below helps compile a regular-expression-like language into a deterministic finite automaton. For a good introduction to these concepts,
// see https://swtch.com/~rsc/regexp/regexp1.html

// nfaEdge is an edge in the NFA. A nil term means a null edge. A to of -1 means the target state has not been connected yet.
type nfaEdge struct {
	term *NodeType
	to   int
}

// nfa constructs an NFA from an expression as returned by the parser. The NFA is represented as a slice of states, which are themselves slices of edges.
// The first state is the entry state and the last state is the success state.
//
// Note that unlike typical NFAs, the edge ordering in this one is significant, in that it is used to construct filler content when necessary.
func nfa(e *expr) [][]*nfaEdge {
	states := [][]*nfaEdge{{}}
	node := func() int {
		states = append(states, []*nfaEdge{})
		return len(states) - 1
	}
	edge := func(from, to int, term *NodeType) *nfaEdge {
		ed := &nfaEdge{term: term, to: to}
		states[from] = append(states[from], ed)
		return ed
	}
	connect := func(edges []*nfaEdge, to int) {
		for _, ed := range edges {
			ed.to = to
		}
	}
	var compile func(e *expr, from int) []*nfaEdge
	compile = func(e *expr, from int) []*nfaEdge {
		switch e.kind {
		case exprChoice:
			var out []*nfaEdge //nolint:prealloc
			for _, sub := range e.exprs {
				out = append(out, compile(sub, from)...)
			}
			return out
		case exprSeq:
			for i := 0; ; i++ {
				next := compile(e.exprs[i], from)
				if i == len(e.exprs)-1 {
					return next
				}
				from = node()
				connect(next, from)
			}
		case exprStar:
			loop := node()
			edge(from, loop, nil)
			connect(compile(e.expr, loop), loop)
			return []*nfaEdge{edge(loop, -1, nil)}
		case exprPlus:
			loop := node()
			connect(compile(e.expr, from), loop)
			connect(compile(e.expr, loop), loop)
			return []*nfaEdge{edge(loop, -1, nil)}
		case exprOpt:
			return append([]*nfaEdge{edge(from, -1, nil)}, compile(e.expr, from)...)
		case exprRange:
			cur := from
			for i := 0; i < e.min; i++ { //nolint:intrange
				next := node()
				connect(compile(e.expr, cur), next)
				cur = next
			}
			if e.max == -1 {
				connect(compile(e.expr, cur), cur)
			} else {
				for i := e.min; i < e.max; i++ {
					next := node()
					edge(cur, next, nil)
					connect(compile(e.expr, cur), next)
					cur = next
				}
			}
			return []*nfaEdge{edge(cur, -1, nil)}
		case exprName:
			return []*nfaEdge{edge(from, -1, e.value)}
		default:
			panic("Unknown expr type")
		}
	}
	connect(compile(e, 0), node())
	return states
}

func cmp(a, b int) int { return b - a }

// nullFrom gets the set of states reachable by null edges from the given state, sorted in descending order. It omits states with only a single null
// out-edge, since they may lead to needless duplicated states.
func nullFrom(states [][]*nfaEdge, node int) []int {
	var result []int
	var scan func(node int)
	scan = func(node int) {
		edges := states[node]
		if len(edges) == 1 && edges[0].term == nil {
			scan(edges[0].to)
			return
		}
		result = append(result, node)
		for i := 0; i < len(edges); i++ { //nolint:intrange,modernize
			term, to := edges[i].term, edges[i].to
			if term == nil && !slices.Contains(result, to) {
				scan(to)
			}
		}
	}
	scan(node)
	slices.SortFunc(result, cmp)
	return result
}

func joinStates(states []int) string {
	var b strings.Builder
	for i, s := range states {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(s))
	}
	return b.String()
}

// dfa compiles an NFA as produced by nfa into a DFA, modeled as a set of state objects (ContentMatch instances) with transitions between them.
func dfa(states [][]*nfaEdge) *ContentMatch {
	labeled := map[string]*ContentMatch{}
	type outEntry struct {
		term *NodeType
		set  []int
	}
	var explore func(stateSet []int) *ContentMatch
	explore = func(stateSet []int) *ContentMatch {
		var out []*outEntry
		for _, node := range stateSet {
			for _, ed := range states[node] {
				if ed.term == nil {
					continue
				}
				var set *outEntry
				for _, o := range out {
					if o.term == ed.term {
						set = o
					}
				}
				for _, target := range nullFrom(states, ed.to) {
					if set == nil {
						set = &outEntry{term: ed.term} //nolint:exhaustruct
						out = append(out, set)
					}
					if !slices.Contains(set.set, target) {
						set.set = append(set.set, target)
					}
				}
			}
		}
		state := &ContentMatch{ValidEnd: slices.Contains(stateSet, len(states)-1)} //nolint:exhaustruct
		labeled[joinStates(stateSet)] = state
		for _, o := range out {
			slices.SortFunc(o.set, cmp)
			next := labeled[joinStates(o.set)]
			if next == nil {
				next = explore(o.set)
			}
			state.next = append(state.next, MatchEdge{Type: o.term, Next: next})
		}
		return state
	}
	return explore(nullFrom(states, 0))
}

func checkForDeadEnds(match *ContentMatch, stream *tokenStream) errors.E {
	work := []*ContentMatch{match}
	for i := 0; i < len(work); i++ {
		state := work[i]
		dead := !state.ValidEnd
		var nodes []string
		for j := 0; j < len(state.next); j++ { //nolint:intrange
			typ, next := state.next[j].Type, state.next[j].Next
			nodes = append(nodes, typ.Name)
			if dead && !(typ.IsText || typ.HasRequiredAttrs()) { //nolint:staticcheck
				dead = false
			}
			if !slices.Contains(work, next) {
				work = append(work, next)
			}
		}
		if dead {
			return stream.err("only non-generatable nodes in a required position, see https://prosemirror.net/docs/guide/#generatable", "nodes", nodes)
		}
	}
	return nil
}
