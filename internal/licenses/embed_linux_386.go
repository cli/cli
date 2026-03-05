package licenses

import "embed"

const rootDir = "embed/linux-386"

//go:embed all:embed/linux-386
var embedFS embed.FS
