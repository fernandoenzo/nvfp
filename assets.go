// Package assets embeds the games manifest that must stay at the repository
// root: internal/update's GamesURL serves it to already-published clients, and
// //go:embed cannot read files above its own package directory.
package assets

import _ "embed"

// Games is the bundled fallback copy of games.json, used when the remote fetch
// and the local cache are both unavailable.
//
//go:embed games.json
var Games []byte
