package licenses

import "embed"

const rootDir = "embed/windows-386"

//go:embed all:embed/windows-386
var embedFS embed.FS
