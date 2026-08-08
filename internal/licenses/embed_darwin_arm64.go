package licenses

import "embed"

const rootDir = "embed/darwin-arm64"

//go:embed all:embed/darwin-arm64
var embedFS embed.FS
