package build

import (
	"os"
	"runtime/debug"
)

// Version is dynamically set by the toolchain or overridden by the Makefile.
var Version = "DEV"

// Date is dynamically set at build time in the Makefile.
var Date = "" // YYYY-MM-DD

func init() {
	if Version == "DEV" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}

	// https://github.com/gdamore/tcell/commit/2f889d79bd61b1fd2f43372529975a65b792a7ae
	_ = os.Setenv("TCELL_MINIMIZE", "1")
}
