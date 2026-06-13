// String level entrypoints tying together HTML parsing (from_dom.go) and canonical HTML serialization (to_html.go). This file has no
// prosemirror-model counterpart.

package model

import (
	"gitlab.com/tozd/go/errors"
)

// CanonicalizeHTML parses the input HTML into a document of the given schema, applying the given parse options, and serializes that document
// back to HTML, returning the canonical HTML form of the input. With default options canonical HTML is the fixed point of this function:
// parsing canonical HTML and serializing it again is the identity.
func CanonicalizeHTML(s *Schema, input string, options ParseOptions) (string, errors.E) {
	doc, errE := ParseHTML(s, input, options)
	if errE != nil {
		return "", errE
	}
	return SerializeHTML(doc), nil
}

// IsCanonicalHTML reports whether the input HTML is in the canonical form produced by SerializeHTML for the given schema: parsing it into
// the schema with the given parse options and serializing it back has to be the identity. Values produced by SerializeHTML satisfy this by
// construction when parsed with default options.
func IsCanonicalHTML(s *Schema, input string, options ParseOptions) (bool, errors.E) {
	canonical, errE := CanonicalizeHTML(s, input, options)
	if errE != nil {
		return false, errE
	}
	return canonical == input, nil
}
