//go:build darwin

package browser

// Prefer browsers that are commonly used as a Chrome alternative, then Safari
// which is always present on macOS. Chrome is last because the failure mode we
// are recovering from is a broken Launch Services mapping left behind by Chrome.
var fallbackApps = []string{
	"Microsoft Edge",
	"Safari",
	"Firefox",
	"Brave Browser",
	"Arc",
	"Chromium",
	"Google Chrome",
}

