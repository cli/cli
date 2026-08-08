package licenses

import "embed"

const rootDir = "embed/linux-arm64"

//go:embed all:embed/linux-arm64
var embedFS embed.FS
