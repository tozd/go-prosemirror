// Generates the conformance fixtures in model/testdata/fixtures from the reference prosemirror-model implementation. The schema JSON
// dialect is built into a ProseMirror Schema by the shared src/index.ts (the published @tozd/prosemirror module), and parsing and
// serialization use the npm prosemirror-model package, pinned to match the vendored git submodule by scripts/check-pm-version.ts (the
// submodule remains the source the Go port and its tests are derived from). For every named input HTML string from scripts/cases.ts, and
// for every file in model/testdata/corpus, the input is parsed through a jsdom div innerHTML and the ProseMirror DOMParser, and the
// resulting document JSON together with its canonical HTML serialization is recorded. Run with "npm run generate-fixtures".

import type { Node as PMNode, Schema } from "prosemirror-model"

import type { SchemaJSON, Validator } from "../src/index.ts"
import type { CaseCategory } from "./cases.ts"

import { JSDOM } from "jsdom"
import * as fs from "node:fs"
import * as path from "node:path"
import { fileURLToPath } from "node:url"

import { buildSchema, docToHtml, htmlToDoc } from "../src/index.ts"
import { caseCategories } from "./cases.ts"
import { assertProseMirrorModelVersionMatch } from "./check-pm-version.ts"

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const testdataDir = path.join(scriptDir, "..", "model", "testdata")
const fixturesDir = path.join(testdataDir, "fixtures")
const corpusDir = path.join(testdataDir, "corpus")

// Named attribute validators referenced by the example schema: linkURL allows a same-origin path, an absolute http or https URL, or a
// mailto URL; resourceURL is the same minus mailto. Non-string values are invalid. These are example-specific and are supplied to
// buildSchema; the published module is validator agnostic.
const exampleValidators: Record<string, Validator> = {
  linkURL: (value) => typeof value === "string" && /^(?:\/(?:[^/]|$)|https?:\/\/[^/]|mailto:[^/])/i.test(value),
  resourceURL: (value) => typeof value === "string" && /^(?:\/(?:[^/]|$)|https?:\/\/[^/])/i.test(value),
}

// JSON.parse preserves the declaration order of (non-numeric) object keys, which determines mark rank, parse rule precedence, and group
// expansion order, matching the strict decoding on the Go side.
function loadSchema(fileName: string): Schema {
  const json = JSON.parse(fs.readFileSync(path.join(testdataDir, fileName), "utf8")) as SchemaJSON
  return buildSchema(json, exampleValidators)
}

const dom = new JSDOM("")
const document = dom.window.document

const compiledSchemas = new Map<string, Schema>()

function schemaFor(fileName: string): Schema {
  let schema = compiledSchemas.get(fileName)
  if (schema === undefined) {
    schema = loadSchema(fileName)
    compiledSchemas.set(fileName, schema)
  }
  return schema
}

// The corpus category replays the HTML sanitization corpus (model/testdata/corpus/t*.html.input) against the example schema. Files are
// ordered by the numeric part of the file name and one trailing newline is stripped from each file content.
function corpusCategory(): CaseCategory {
  const pattern = /^t(\d+)([a-z]*)\.html\.input$/
  const files = fs
    .readdirSync(corpusDir)
    .filter((name) => pattern.test(name))
    .sort((a, b) => {
      const ma = pattern.exec(a)!
      const mb = pattern.exec(b)!
      return Number(ma[1]) - Number(mb[1]) || ma[2].localeCompare(mb[2])
    })
  if (files.length === 0) {
    throw new Error(`no corpus files found in ${corpusDir}`)
  }
  const cases = files.map((name) => {
    let input = fs.readFileSync(path.join(corpusDir, name), "utf8")
    if (input.endsWith("\n")) {
      input = input.slice(0, -1)
    }
    return { name: name.slice(0, -".html.input".length), input }
  })
  return { fixture: "corpus", schema: "example-schema.json", cases }
}

function generateFixture(category: CaseCategory): void {
  const schema = schemaFor(category.schema)
  const cases = category.cases.map(({ name, input }) => {
    let doc: PMNode
    let canonical: string
    let recanonical: string
    try {
      doc = htmlToDoc(schema, input, {}, document)
      doc.check()
      canonical = docToHtml(schema, doc, document)
      // Canonicalization is not always idempotent: a document may serialize to HTML which parses back to a different document (for example
      // a paragraph whose text keeps a leading space because of the block structure it was parsed out of). When that happens the case
      // records the re-canonicalization so both implementations can assert the same non-fixed-point behavior.
      recanonical = docToHtml(schema, htmlToDoc(schema, canonical, {}, document), document)
    } catch (error) {
      throw new Error(`fixture ${category.fixture}, case "${name}": ${String(error)}`, { cause: error })
    }
    if (recanonical === canonical) {
      // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
      return { name, input, doc: doc.toJSON(), canonical }
    }
    // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
    return { name, input, doc: doc.toJSON(), canonical, recanonical }
  })
  const fixturePath = path.join(fixturesDir, `${category.fixture}.json`)
  fs.writeFileSync(fixturePath, JSON.stringify({ schema: category.schema, cases }, null, 2) + "\n")
  console.log(`${path.relative(process.cwd(), fixturePath)}: ${cases.length} cases (${category.schema})`)
}

assertProseMirrorModelVersionMatch()

fs.mkdirSync(fixturesDir, { recursive: true })
for (const category of [...caseCategories, corpusCategory()]) {
  generateFixture(category)
}
