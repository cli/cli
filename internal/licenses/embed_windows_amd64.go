package licenses

import "embed"

const rootDir = "embed/windows-amd64"

//go:embed all:embed/windows-amd64
var embedFS embed.FS
