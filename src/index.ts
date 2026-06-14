// ProseMirror frontend utilities for Go port of ProseMirror. It provides:
//
// A builder for ProseMirror Schema from the schema JSON dialect described in the "Schema JSON dialect" section of PORTING.md, the same dialect the Go port parses.
// Given an already-parsed schema JSON object and a registry of named attribute validators, it produces a prosemirror-model Schema (with parseDOM and toDOM derived
// from the dialect) that a frontend can use directly.
//
// A canonical HTML serializer (serializeDOM, with escapeHTML) that stringifies the DOM produced by prosemirror-model's DOMSerializer into the same canonical HTML
// the Go SerializeHTML emits (the same five-character escaping, lowercased tags, double-quoted attributes, void-element handling, and pre newline rule), so a
// frontend can compute the same canonical HTML as the backend.
//
// HTML conversion helpers (htmlToDoc, docToHtml, isCanonicalHTML) that parse an HTML string into a document of a given schema, serialize a document back to its
// canonical HTML, and report whether HTML is already canonical (its parse/serialize round trip is the identity), matching the Go ParseHTML/SerializeHTML and the
// claim-validity check IsCanonicalHTML.

import type { Attrs, DOMOutputSpec, Mark, MarkSpec, NodeSpec, ParseOptions, ParseRule, Node as PMNode, StyleParseRule, TagParseRule } from "prosemirror-model"

import { DOMParser, DOMSerializer, Schema } from "prosemirror-model"

// Shapes of the schema JSON dialect.

export interface AttributeSpecJSON {
  default?: unknown
  validate?: string
  onInvalid?: "rejectRule" | "drop"
}

export interface ToHTMLJSON {
  tag: string
  attrs?: string[]
  content?: ToHTMLJSON
}

export interface ParseRuleJSON {
  tag?: string
  style?: string
  namespace?: string
  context?: string
  consuming?: boolean
  ignore?: boolean
  skip?: boolean
  closeParent?: boolean
  priority?: number
  preserveWhitespace?: boolean | "full"
  attrs?: Record<string, unknown>
}

export interface NodeSpecJSON {
  content?: string
  marks?: string
  group?: string
  inline?: boolean
  atom?: boolean
  code?: boolean
  whitespace?: "pre" | "normal"
  linebreakReplacement?: boolean
  defining?: boolean
  selectable?: boolean
  attrs?: Record<string, AttributeSpecJSON>
  toHTML?: ToHTMLJSON
  parseHTML?: ParseRuleJSON[]
}

export interface MarkSpecJSON {
  group?: string
  code?: boolean
  excludes?: string
  spanning?: boolean
  inclusive?: boolean
  attrs?: Record<string, AttributeSpecJSON>
  toHTML?: ToHTMLJSON
  parseHTML?: ParseRuleJSON[]
}

export interface SchemaJSON {
  topNode?: string
  nodes: Record<string, NodeSpecJSON>
  marks?: Record<string, MarkSpecJSON>
}

// A named attribute validator returns true when the value is acceptable. The registry passed to buildSchema maps validator names used in the schema JSON
// to these functions; a schema referencing a name that is not in the registry is an error.
export type Validator = (value: unknown) => boolean

// Builds a ProseMirror parse rule from a dialect parseHTML rule. Constant attribute entries (number/boolean/null) become static rule attrs;
// string entries are attribute extraction and become a getAttrs function which reads the named HTML attribute, falls back to the attribute spec
// default when the attribute is absent (rejecting the rule when the attribute is required), and applies the named validator when one is
// configured: on failure, onInvalid "rejectRule" (the default) rejects the rule and "drop" replaces the value with the default.
export function buildParseRule(ruleJSON: ParseRuleJSON, attrSpecs: Record<string, AttributeSpecJSON> | undefined, validators: Record<string, Validator>): ParseRule {
  // Style rules produce marks and match an inline CSS declaration; their attrs are always constants (the declarative dialect does not extract from the
  // style value). Both consuming and ignore are honored for style rules (skip and closeParent are tag-rule behaviors). Tag rules match a CSS selector,
  // optionally restricted by namespace, and may extract attributes from the element.
  if (ruleJSON.style !== undefined) {
    const rule: StyleParseRule = { style: ruleJSON.style }
    if (ruleJSON.context !== undefined) rule.context = ruleJSON.context
    if (ruleJSON.consuming !== undefined) rule.consuming = ruleJSON.consuming
    if (ruleJSON.ignore !== undefined) rule.ignore = ruleJSON.ignore
    if (ruleJSON.priority !== undefined) rule.priority = ruleJSON.priority
    if (ruleJSON.attrs !== undefined) rule.attrs = ruleJSON.attrs
    return rule
  }
  const rule: TagParseRule = { tag: ruleJSON.tag! }
  if (ruleJSON.namespace !== undefined) rule.namespace = ruleJSON.namespace
  if (ruleJSON.context !== undefined) rule.context = ruleJSON.context
  if (ruleJSON.consuming !== undefined) rule.consuming = ruleJSON.consuming
  if (ruleJSON.ignore !== undefined) rule.ignore = ruleJSON.ignore
  if (ruleJSON.skip !== undefined) rule.skip = ruleJSON.skip
  if (ruleJSON.closeParent !== undefined) rule.closeParent = ruleJSON.closeParent
  if (ruleJSON.priority !== undefined) rule.priority = ruleJSON.priority
  if (ruleJSON.preserveWhitespace !== undefined) rule.preserveWhitespace = ruleJSON.preserveWhitespace
  if (ruleJSON.attrs !== undefined) {
    const constants: Record<string, unknown> = {}
    const extracted: Record<string, string> = {}
    for (const [attrName, value] of Object.entries(ruleJSON.attrs)) {
      if (typeof value === "string") {
        extracted[attrName] = value
      } else {
        constants[attrName] = value
      }
    }
    if (Object.keys(extracted).length === 0) {
      rule.attrs = constants
    } else {
      rule.getAttrs = (dom: HTMLElement): Attrs | false => {
        const attrs: Record<string, unknown> = { ...constants }
        for (const [attrName, htmlAttr] of Object.entries(extracted)) {
          const attrSpec = attrSpecs?.[attrName]
          if (attrSpec === undefined) {
            throw new Error(`parse rule for tag "${ruleJSON.tag}" extracts undeclared attribute "${attrName}"`)
          }
          const hasDefault = Object.prototype.hasOwnProperty.call(attrSpec, "default")
          const value = dom.getAttribute(htmlAttr)
          if (value === null) {
            if (!hasDefault) {
              return false
            }
            attrs[attrName] = attrSpec.default
            continue
          }
          if (attrSpec.validate !== undefined) {
            const validator = validators[attrSpec.validate]
            if (validator === undefined) {
              throw new Error(`unknown validator "${attrSpec.validate}" for attribute "${attrName}"`)
            }
            if (!validator(value)) {
              if ((attrSpec.onInvalid ?? "rejectRule") === "drop") {
                attrs[attrName] = attrSpec.default
                continue
              }
              return false
            }
          }
          attrs[attrName] = value
        }
        return attrs
      }
    }
  }
  return rule
}

// Builds a DOMOutputSpec from a (possibly nested) dialect toHTML spec against concrete attribute values: "{attr}" placeholders in the tag are substituted
// with String(value) and the attribute object is built from the listed attribute names, skipping null/undefined values. When the spec has a content field
// the element wraps the nested spec, so the content hole (when hole is true, for nodes with content and for marks) sits at the innermost element.
export function buildOutputSpec(toHTML: ToHTMLJSON, attrs: Attrs, hole: boolean): DOMOutputSpec {
  const tag = toHTML.tag.replace(/\{([^{}]+)\}/g, (_match, name: string) => String(attrs[name]))
  const attrsObj: Record<string, unknown> = {}
  for (const name of toHTML.attrs ?? []) {
    // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
    const value = attrs[name]
    if (value === null || value === undefined) {
      continue
    }
    attrsObj[name] = value
  }
  if (toHTML.content !== undefined) {
    return [tag, attrsObj, buildOutputSpec(toHTML.content, attrs, hole)] as DOMOutputSpec
  }
  return (hole ? [tag, attrsObj, 0] : [tag, attrsObj]) as DOMOutputSpec
}

// Translates the attrs object of a node/mark spec, keeping only the "default" key (validate/onInvalid govern HTML parse behavior and are handled
// by buildParseRule; passing them to ProseMirror would change node creation semantics).
export function buildAttrs(attrsJSON: Record<string, AttributeSpecJSON>): Record<string, { default?: unknown }> {
  const attrs: Record<string, { default?: unknown }> = {}
  for (const [name, attrJSON] of Object.entries(attrsJSON)) {
    attrs[name] = Object.prototype.hasOwnProperty.call(attrJSON, "default") ? { default: attrJSON.default } : {}
  }
  return attrs
}

export function translateNodeSpec(json: NodeSpecJSON, validators: Record<string, Validator>): NodeSpec {
  const spec: NodeSpec = {}
  if (json.content !== undefined) spec.content = json.content
  if (json.marks !== undefined) spec.marks = json.marks
  if (json.group !== undefined) spec.group = json.group
  if (json.inline !== undefined) spec.inline = json.inline
  if (json.atom !== undefined) spec.atom = json.atom
  if (json.code !== undefined) spec.code = json.code
  if (json.whitespace !== undefined) spec.whitespace = json.whitespace
  if (json.linebreakReplacement !== undefined) spec.linebreakReplacement = json.linebreakReplacement
  if (json.defining !== undefined) spec.defining = json.defining
  if (json.selectable !== undefined) spec.selectable = json.selectable
  if (json.attrs !== undefined) spec.attrs = buildAttrs(json.attrs)
  // A node's parse rules are tag rules; style rules produce marks and the dialect (like the Go side) only allows them on mark types.
  if (json.parseHTML !== undefined) spec.parseDOM = json.parseHTML.map((rule) => buildParseRule(rule, json.attrs, validators)) as TagParseRule[]
  if (json.toHTML !== undefined) {
    const toHTML = json.toHTML
    const hasContent = Boolean(json.content)
    spec.toDOM = (node: PMNode): DOMOutputSpec => buildOutputSpec(toHTML, node.attrs, hasContent)
  }
  return spec
}

export function translateMarkSpec(json: MarkSpecJSON, validators: Record<string, Validator>): MarkSpec {
  const spec: MarkSpec = {}
  if (json.group !== undefined) spec.group = json.group
  if (json.code !== undefined) spec.code = json.code
  if (json.excludes !== undefined) spec.excludes = json.excludes
  if (json.spanning !== undefined) spec.spanning = json.spanning
  if (json.inclusive !== undefined) spec.inclusive = json.inclusive
  if (json.attrs !== undefined) spec.attrs = buildAttrs(json.attrs)
  if (json.parseHTML !== undefined) spec.parseDOM = json.parseHTML.map((rule) => buildParseRule(rule, json.attrs, validators))
  if (json.toHTML !== undefined) {
    const toHTML = json.toHTML
    spec.toDOM = (mark: Mark): DOMOutputSpec => buildOutputSpec(toHTML, mark.attrs, true)
  }
  return spec
}

// Builds a prosemirror-model Schema from a parsed schema JSON object. The validators registry resolves the validate names referenced by attribute specs;
// it defaults to empty, so a schema that references a validator must supply it (an unknown validator throws during HTML parsing).
//
// JSON.parse preserves the declaration order of (non-numeric) object keys, which determines mark rank, parse rule precedence, and group expansion order,
// matching the strict decoding on the Go side; pass the object from JSON.parse to preserve that order.
export function buildSchema(spec: SchemaJSON, validators: Record<string, Validator> = {}): Schema {
  const nodes: Record<string, NodeSpec> = {}
  for (const [name, nodeSpec] of Object.entries(spec.nodes)) {
    nodes[name] = translateNodeSpec(nodeSpec, validators)
  }
  const marks: Record<string, MarkSpec> = {}
  for (const [name, markSpec] of Object.entries(spec.marks ?? {})) {
    marks[name] = translateMarkSpec(markSpec, validators)
  }
  return new Schema({ topNode: spec.topNode, nodes, marks })
}

// The five characters escaped in text and attribute values; everything else, including U+00A0, is emitted raw.
const GO_HTML_ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "'": "&#39;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&#34;",
}

// Escapes the five characters the Go canonical serializer escapes in text and attribute values, leaving everything else (including U+00A0) raw.
export function escapeHTML(text: string): string {
  return text.replace(/[&'<>"]/g, (c) => GO_HTML_ESCAPES[c])
}

// The full HTML spec void element set, emitted as an open tag only; this keeps any schema well formed.
const VOID_ELEMENTS = new Set(["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr"])

const ELEMENT_NODE = 1
const TEXT_NODE = 3

// Recursive string serialization of the DOM produced by DOMSerializer: lowercase tags, attributes always double-quoted in DOM order, void elements emitted as the
// open tag only, and one extra "\n" after a pre open tag whose first child is a text node starting with "\n" (the HTML parser drops one newline right after the pre
// open tag, so the doubled form keeps the parse round trip lossless). It produces the same canonical HTML as the Go SerializeHTML.
export function serializeDOM(node: Node): string {
  if (node.nodeType === TEXT_NODE) {
    return escapeHTML((node as Text).data)
  }
  if (node.nodeType !== ELEMENT_NODE) {
    // DOMSerializer produces only elements and text nodes.
    return ""
  }
  const element = node as Element
  const tag = element.tagName.toLowerCase()
  let out = "<" + tag
  for (let i = 0; i < element.attributes.length; i++) {
    const attr = element.attributes[i]
    out += " " + attr.name + '="' + escapeHTML(attr.value) + '"'
  }
  out += ">"
  if (VOID_ELEMENTS.has(tag)) {
    return out
  }
  if (tag === "pre" && element.firstChild !== null && element.firstChild.nodeType === TEXT_NODE && (element.firstChild as Text).data.startsWith("\n")) {
    out += "\n"
  }
  for (const child of element.childNodes) {
    out += serializeDOM(child)
  }
  return out + "</" + tag + ">"
}

// htmlToDoc parses an HTML string into a document of the given schema: it assigns the HTML to the innerHTML of a detached div (the HTML fragment parsing the browser
// uses) and runs the schema's DOMParser over it. options are the prosemirror-model parse options, notably preserveWhitespace. domDocument supplies the DOM
// implementation used to create the container; it defaults to the global document, so a browser caller can omit it, while a non-browser caller (for example one using
// jsdom) passes its document.
export function htmlToDoc(schema: Schema, html: string, options: ParseOptions = {}, domDocument: Document = document): PMNode {
  const container = domDocument.createElement("div")
  container.innerHTML = html
  return DOMParser.fromSchema(schema).parse(container, options)
}

// docToHtml serializes a document to its canonical HTML form: it runs the schema's DOMSerializer to build a DOM fragment and stringifies it with serializeDOM,
// producing the same canonical HTML the Go SerializeHTML emits. The output is a pure function of the document (the same on every browser engine), so collaborating
// clients can compare HTML against their own editor state directly. domDocument supplies the DOM implementation used to build the fragment; it defaults to the global
// document, so a browser caller can omit it, while a non-browser caller passes its document.
export function docToHtml(schema: Schema, doc: PMNode, domDocument: Document = document): string {
  const fragment = DOMSerializer.fromSchema(schema).serializeFragment(doc.content, { document: domDocument })
  let html = ""
  for (const child of fragment.childNodes) {
    html += serializeDOM(child)
  }
  return html
}

// isCanonicalHTML reports whether html is already in the canonical form docToHtml produces: parsing it into the schema and serializing it back is the identity. The
// parse options have to match those used to produce the canonical HTML for the answer to be meaningful. It mirrors the Go IsCanonicalHTML and is the HTML claim
// validity check on the frontend.
export function isCanonicalHTML(schema: Schema, html: string, options: ParseOptions = {}, domDocument: Document = document): boolean {
  return docToHtml(schema, htmlToDoc(schema, html, options, domDocument), domDocument) === html
}
