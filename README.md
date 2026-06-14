# Go port of ProseMirror

[![pkg.go.dev](https://pkg.go.dev/badge/gitlab.com/tozd/go/prosemirror)](https://pkg.go.dev/gitlab.com/tozd/go/prosemirror)
[![NPM](https://img.shields.io/npm/v/@tozd/prosemirror.svg)](https://www.npmjs.com/package/@tozd/prosemirror)
[![Go Report Card](https://goreportcard.com/badge/gitlab.com/tozd/go/prosemirror)](https://goreportcard.com/report/gitlab.com/tozd/go/prosemirror)
[![pipeline status](https://gitlab.com/tozd/go/prosemirror/badges/main/pipeline.svg?ignore_skipped=true)](https://gitlab.com/tozd/go/prosemirror/-/pipelines)
[![coverage report](https://gitlab.com/tozd/go/prosemirror/badges/main/coverage.svg)](https://gitlab.com/tozd/go/prosemirror/-/graphs/main/charts)

A Go port of [ProseMirror](https://prosemirror.net/) for server-side parsing, validation, and canonical HTML
serialization of documents, with a TypeScript companion (the `@tozd/prosemirror` NPM package) that builds the same
schema in the browser from the same shareable JSON.

The components currently available:

- `model` package, ported from [prosemirror-model](https://github.com/ProseMirror/prosemirror-model) v1.25.8 – the
  document model: schemas, nodes, marks, content expressions, HTML fragment parsing, and canonical HTML serialization.

Each ported component mirrors the structure of its ProseMirror
TypeScript source so the two can be kept in sync. See [PORTING.md](PORTING.md)
for the porting contract.

It is used by [PeerDB](https://gitlab.com/peerdb/peerdb), a collaborative database.

## Installation

### Go installation

You can add it to your project using `go get`:

```sh
go get gitlab.com/tozd/go/prosemirror/model
```

It requires Go 1.25 or newer.

### TypeScript/JavaScript installation

You can add frontend utilities for Go port of ProseMirror to your project using `npm`:

```sh
npm install --save @tozd/prosemirror prosemirror-model
```

It requires node 22 or newer.

## Usage

### Go usage

```go
schema, errE := model.NewSchema(schemaJSON, map[string]model.AttrValidator{
  "linkURL": validateLinkURL,
})
if errE != nil {
  // Handle the error.
}

doc, errE := model.ParseHTML(schema, `<p>Hello, <b>world</b>!</p>`, model.ParseOptions{})
if errE != nil {
  // Handle the error.
}

canonical := model.SerializeHTML(doc)

ok, errE := model.IsCanonicalHTML(schema, canonical, model.ParseOptions{})
// ok is true: serializing a parsed document always yields the canonical form.
```

See full package documentation with examples on [pkg.go.dev](https://pkg.go.dev/gitlab.com/tozd/go/prosemirror#section-documentation).

### TypeScript/JavaScript usage

The same schema JSON can be built into a ProseMirror schema in the browser:

```ts
import { buildSchema } from "@tozd/prosemirror"

const schema = buildSchema(schemaJSON, { linkURL: validateLinkURL })
```

`buildSchema` takes a parsed schema JSON object and a registry of named attribute validators and returns a
`prosemirror-model` `Schema`, with `parseDOM` and `toDOM` derived from the dialect, the browser-usable counterpart of
`model.NewSchema`. `prosemirror-model` is a peer dependency. The frontend and the Go backend build the same schema from
one shared JSON definition.

## Schema JSON dialect

Schemas are described by a single JSON document that can be shared with a ProseMirror TypeScript implementation.
Functions cannot live in JSON, so attribute validators are referenced by name (resolved against a registry passed to
`model.NewSchema`) and the HTML mapping is declarative (`toHTML` and `parseHTML` keys on nodes and marks).
`model.NewSchema` accepts any spec in this dialect, not only the example schema shipped as a fixture: `toHTML` covers
the declarative subset of ProseMirror's `DOMOutputSpec` (including nested elements such as a code block rendered as
`pre` wrapping `code`), and `parseHTML` accepts a full CSS selector in `tag` (matched with
[cascadia](https://github.com/andybalholm/cascadia)), style rules keyed on an inline CSS declaration
(`{"style": "font-weight=bold"}`), and a `namespace` constraint, alongside `context` and `priority`. See
[PORTING.md](PORTING.md) for the full specification of the dialect.

The one documented fidelity boundary is style matching: a style rule matches only an element's own inline longhand
declarations and does not perform CSSOM shorthand expansion or value normalization. So `style="font-weight: bold"`
matches a `font-weight` rule, but `style="font: bold 12px serif"` (which exposes `font-weight` only after shorthand
expansion in a browser) does not. Everything else parses identically to the browser. Editor-only behaviors and the
transform and collaboration layers are out of scope.

## Related projects

- [prosemirror-go](https://github.com/cozy/prosemirror-go) – a similar port which is somewhat stale and is
  licensed under AGPL.

## GitHub mirror

There is also a [read-only GitHub mirror available](https://github.com/tozd/go-prosemirror),
if you need to fork the project there.

## Disclaimer

Port has been fully generated and maintained by AI.

## License

Licensed under the Apache License, Version 2.0 (see [LICENSE](LICENSE)). The ported packages are derivative works of the
corresponding [ProseMirror](https://prosemirror.net/) packages, used under the MIT license (see [NOTICE](NOTICE)).
