package licenses

import "embed"

const rootDir = "embed/linux-amd64"

//go:embed all:embed/linux-amd64
var embedFS embed.FS
