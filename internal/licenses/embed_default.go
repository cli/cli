// This file is necessary to allow building on platforms that we do not have
// official release builds for. Without this, `go build` or `go install` calls
// would fail due to undefined symbols that are expected to be included in the
// build.

//go:build !(darwin && (amd64 || arm64)) && !(linux && (386 || amd64 || arm || arm64)) && !(windows && (386 || amd64 || arm64))

package licenses

import "embed"

const rootDir = ""

// embedFS is left empty to indicate there's no embedded content.
var embedFS embed.FS
