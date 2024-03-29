package schema

import "github.com/open-policy-agent/gatekeeper/v3/pkg/mutation/path/parser"

// Unknown represents a path element we do not know the type of.
// Elements of type unknown do not conflict with path elements of known types.
const Unknown = parser.NodeType("Unknown")

// Set represents a list populated by unique values.
const Set = parser.NodeType("Set")

// String represents a string element.
const String = parser.NodeType("String")
