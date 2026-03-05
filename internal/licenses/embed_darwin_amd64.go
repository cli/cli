package licenses

import "embed"

const rootDir = "embed/darwin-amd64"

//go:embed all:embed/darwin-amd64
var embedFS embed.FS
