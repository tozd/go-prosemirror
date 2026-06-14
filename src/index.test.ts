// @vitest-environment jsdom

import type { Node, Schema } from "prosemirror-model"

import type { SchemaJSON, Validator } from "./index.ts"

import { DOMParser, DOMSerializer } from "prosemirror-model"
import { assert, expect, test } from "vitest"

import { buildSchema, docToHtml, escapeHTML, htmlToDoc, isCanonicalHTML, serializeDOM } from "./index.ts"

const basicSchemaJSON: SchemaJSON = {
  nodes: {
    doc: { content: "block+" },
    paragraph: { group: "block", content: "inline*", toHTML: { tag: "p" }, parseHTML: [{ tag: "p" }] },
    heading: {
      group: "block",
      content: "inline*",
      attrs: { level: { default: 1 } },
      toHTML: { tag: "h{level}" },
      parseHTML: [
        { tag: "h1", attrs: { level: 1 } },
        { tag: "h2", attrs: { level: 2 } },
      ],
    },
    text: { group: "inline" },
  },
  marks: {
    em: { toHTML: { tag: "em" }, parseHTML: [{ tag: "em" }, { tag: "i" }, { style: "font-style=italic" }] },
    link: { attrs: { href: {} }, toHTML: { tag: "a", attrs: ["href"] }, parseHTML: [{ tag: "a[href]", attrs: { href: "href" } }] },
  },
}

function parse(schema: Schema, html: string): Node {
  const container = document.createElement("div")
  container.innerHTML = html
  return DOMParser.fromSchema(schema).parse(container)
}

function serialize(schema: Schema, doc: Node): string {
  const fragment = DOMSerializer.fromSchema(schema).serializeFragment(doc.content)
  const div = document.createElement("div")
  div.appendChild(fragment)
  return div.innerHTML
}

test("buildSchema constructs the declared node and mark types", () => {
  const schema = buildSchema(basicSchemaJSON)
  assert.isDefined(schema.nodes.paragraph)
  assert.isDefined(schema.nodes.heading)
  assert.isDefined(schema.marks.em)
  assert.isDefined(schema.marks.link)
  assert.equal(schema.topNodeType.name, "doc")
  assert.isTrue(schema.nodes.paragraph.isInGroup("block"))
  // The heading "level" attribute keeps its dialect default and drops validate/onInvalid.
  assert.equal(schema.nodes.heading.create().attrs.level, 1)
})

test("parses and serializes through the dialect parseDOM/toDOM", () => {
  const schema = buildSchema(basicSchemaJSON)
  const doc = parse(schema, "<p>hello <em>there</em></p>")
  assert.equal(doc.toString(), 'doc(paragraph("hello ", em("there")))')
  const html = serialize(schema, doc)
  expect(html).toBe("<p>hello <em>there</em></p>")
  // Round trips: parsing the serialization yields an equal document.
  assert.isTrue(doc.eq(parse(schema, html)))
})

test("resolves {attr} tag placeholders both ways", () => {
  const schema = buildSchema(basicSchemaJSON)
  const doc = parse(schema, "<h2>title</h2>")
  assert.equal(doc.firstChild!.type.name, "heading")
  assert.equal(doc.firstChild!.attrs.level, 2)
  expect(serialize(schema, doc)).toBe("<h2>title</h2>")
})

test("extracts attributes from elements and emits them", () => {
  const schema = buildSchema(basicSchemaJSON)
  const doc = parse(schema, '<p><a href="/page">x</a></p>')
  const text = doc.firstChild!.firstChild!
  assert.equal(text.marks.length, 1)
  assert.equal(text.marks[0].attrs.href, "/page")
  expect(serialize(schema, doc)).toBe('<p><a href="/page">x</a></p>')
})

function validatedSchemaJSON(onInvalid: "rejectRule" | "drop"): SchemaJSON {
  return {
    nodes: {
      doc: { content: "block+" },
      paragraph: { group: "block", content: "inline*", toHTML: { tag: "p" }, parseHTML: [{ tag: "p" }] },
      text: { group: "inline" },
    },
    marks: {
      link: {
        attrs: { href: { default: null, validate: "path", onInvalid } },
        toHTML: { tag: "a", attrs: ["href"] },
        parseHTML: [{ tag: "a[href]", attrs: { href: "href" } }],
      },
    },
  }
}

const pathValidators: Record<string, Validator> = { path: (value) => typeof value === "string" && value.startsWith("/") }

test("applies named validators: rejectRule drops the whole rule on failure", () => {
  const schema = buildSchema(validatedSchemaJSON("rejectRule"), { validators: pathValidators })
  // A valid value keeps the link.
  assert.equal(parse(schema, '<p><a href="/ok">x</a></p>').firstChild!.firstChild!.marks.length, 1)
  // An invalid value rejects the parse rule, so no link mark is applied.
  assert.equal(parse(schema, '<p><a href="javascript:bad">x</a></p>').firstChild!.firstChild!.marks.length, 0)
})

test("applies named validators: drop replaces the invalid value with the default", () => {
  const schema = buildSchema(validatedSchemaJSON("drop"), { validators: pathValidators })
  const text = parse(schema, '<p><a href="javascript:bad">x</a></p>').firstChild!.firstChild!
  assert.equal(text.marks.length, 1)
  assert.isNull(text.marks[0].attrs.href)
})

test("throws on a schema that references a validator the registry does not supply", () => {
  const schema = buildSchema(validatedSchemaJSON("rejectRule")) // no validators passed
  assert.throws(() => parse(schema, '<p><a href="/ok">x</a></p>'), /unknown validator/)
})

test("matches inline style rules to marks", () => {
  const schema = buildSchema(basicSchemaJSON)
  const doc = parse(schema, '<p><span style="font-style: italic">x</span></p>')
  assert.equal(doc.firstChild!.firstChild!.marks[0].type.name, "em")
})

test("escapeHTML escapes the five canonical characters and leaves everything else raw", () => {
  expect(escapeHTML(`& ' < > "`)).toBe("&amp; &#39; &lt; &gt; &#34;")
  // Everything else, including U+00A0 and a tab, stays raw.
  expect(escapeHTML("a b\tc")).toBe("a b\tc")
  expect(escapeHTML("plain")).toBe("plain")
})

test("serializeDOM serializes a text node with the canonical escaping", () => {
  expect(serializeDOM(document.createTextNode("a & b <c>"))).toBe("a &amp; b &lt;c&gt;")
})

test("serializeDOM lowercases the tag and double-quotes escaped attributes", () => {
  const el = document.createElement("a")
  el.setAttribute("href", "/x?a=1&b=2")
  el.appendChild(document.createTextNode("link"))
  expect(serializeDOM(el)).toBe('<a href="/x?a=1&amp;b=2">link</a>')
})

test("serializeDOM emits void elements as an open tag only", () => {
  expect(serializeDOM(document.createElement("br"))).toBe("<br>")
  const img = document.createElement("img")
  img.setAttribute("src", "x.png")
  expect(serializeDOM(img)).toBe('<img src="x.png">')
})

test("serializeDOM doubles a leading newline inside pre but not otherwise", () => {
  const pre = document.createElement("pre")
  pre.appendChild(document.createTextNode("\nx"))
  expect(serializeDOM(pre)).toBe("<pre>\n\nx</pre>")
  const pre2 = document.createElement("pre")
  pre2.appendChild(document.createTextNode("x"))
  expect(serializeDOM(pre2)).toBe("<pre>x</pre>")
})

test("serializeDOM recurses into nested elements", () => {
  const p = document.createElement("p")
  const strong = document.createElement("strong")
  strong.appendChild(document.createTextNode("bold"))
  p.appendChild(document.createTextNode("a "))
  p.appendChild(strong)
  expect(serializeDOM(p)).toBe("<p>a <strong>bold</strong></p>")
})

test("serializeDOM ignores nodes that are neither elements nor text", () => {
  expect(serializeDOM(document.createComment("c"))).toBe("")
})

test("htmlToDoc parses an HTML string into a document of the schema", () => {
  const schema = buildSchema(basicSchemaJSON)
  const doc = htmlToDoc(schema, "<p>hello <em>there</em></p>")
  assert.equal(doc.toString(), 'doc(paragraph("hello ", em("there")))')
})

test("docToHtml serializes a document to canonical HTML", () => {
  const schema = buildSchema(basicSchemaJSON)
  const doc = htmlToDoc(schema, "<p>hello <em>there</em></p>")
  expect(docToHtml(schema, doc)).toBe("<p>hello <em>there</em></p>")
})

test("htmlToDoc honors the preserveWhitespace parse option", () => {
  const schema = buildSchema(basicSchemaJSON)
  // The default collapses the double space; preserveWhitespace keeps it.
  expect(docToHtml(schema, htmlToDoc(schema, "<p>a  b</p>"))).toBe("<p>a b</p>")
  expect(docToHtml(schema, htmlToDoc(schema, "<p>a  b</p>", { preserveWhitespace: true }))).toBe("<p>a  b</p>")
})

test("isCanonicalHTML reports whether HTML is its own parse and serialize round trip", () => {
  const schema = buildSchema(basicSchemaJSON)
  // Canonical input is unchanged by the round trip.
  assert.isTrue(isCanonicalHTML(schema, "<p>hello <em>there</em></p>"))
  // Non-canonical input (an uppercase tag, or an em written as i) is not.
  assert.isFalse(isCanonicalHTML(schema, "<P>x</P>"))
  assert.isFalse(isCanonicalHTML(schema, "<p><i>x</i></p>"))
})

test("buildParseRule honors the consuming, ignore, skip, and closeParent flags", () => {
  const flagsSchemaJSON: SchemaJSON = {
    nodes: {
      doc: { content: "block+" },
      paragraph: {
        group: "block",
        content: "inline*",
        toHTML: { tag: "p" },
        parseHTML: [{ tag: "p" }, { tag: "br", closeParent: true }, { tag: "cite", skip: true }, { tag: "del", ignore: true }],
      },
      text: { group: "inline" },
    },
    marks: {
      em: { toHTML: { tag: "em" }, parseHTML: [{ tag: "em" }, { tag: "i" }, { style: "font-weight", consuming: false }] },
      strong: { toHTML: { tag: "strong" }, parseHTML: [{ tag: "strong" }, { tag: "b" }, { style: "font-weight=800" }] },
      underline: { toHTML: { tag: "u" }, parseHTML: [{ tag: "u" }, { style: "font-style=oblique", ignore: true }] },
    },
  }
  const schema = buildSchema(flagsSchemaJSON)
  // closeParent: a br closes the paragraph; skip: the cite is dropped but its content kept; ignore: the del and its content are dropped.
  expect(docToHtml(schema, htmlToDoc(schema, "<p>one<br>two</p>"))).toBe("<p>one</p><p>two</p>")
  expect(docToHtml(schema, htmlToDoc(schema, "<p>a<cite>b</cite>c</p>"))).toBe("<p>abc</p>")
  expect(docToHtml(schema, htmlToDoc(schema, "<p>a<del>b</del>c</p>"))).toBe("<p>ac</p>")
  // consuming false on the font-weight style rule lets the font-weight=800 rule also match, applying both marks.
  expect(docToHtml(schema, htmlToDoc(schema, '<p><span style="font-weight: 800">one</span></p>'))).toBe("<p><em><strong>one</strong></em></p>")
  // ignore on a style rule drops the element carrying it together with its content.
  expect(docToHtml(schema, htmlToDoc(schema, '<p>x<span style="font-style: oblique">y</span>z</p>'))).toBe("<p>xz</p>")
})
