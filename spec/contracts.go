// Package spec embeds the canonical portable contract for runtime validation.
package spec

import _ "embed"

//go:embed snapshot.schema.json
var SnapshotSchema []byte
