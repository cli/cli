package licenses

import "embed"

const rootDir = "embed/windows-arm64"

//go:embed all:embed/windows-arm64
var embedFS embed.FS
