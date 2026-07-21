//go:build android && arm64

package licenses

import "embed"

const rootDir = "embed/android-arm64"

//go:embed all:embed/android-arm64
var embedFS embed.FS
