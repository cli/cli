package licenses

import "embed"

const rootDir = "embed/linux-arm"

//go:embed all:embed/linux-arm
var embedFS embed.FS
