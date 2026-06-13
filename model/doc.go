// Package model is the document-model component of a Go port of ProseMirror (https://prosemirror.net/), ported from prosemirror-model
// (vendored under prosemirror/prosemirror-model): schemas, nodes, marks, and content expressions, plus HTML fragment parsing over
// golang.org/x/net/html and canonical HTML serialization. Documents are persistent immutable values: nodes, fragments, and marks must not
// be mutated after construction. See PORTING.md in the repository root for the mapping to the TypeScript sources and the list of intentional
// deviations.
package model
