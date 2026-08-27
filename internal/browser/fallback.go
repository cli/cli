package browser

import (
	"fmt"
	"os"
	"path/filepath"
)

func openFirstInstalledApp(url string, apps []string, dirs []string, run execFunc, stat statFunc) error {
	var lastErr error
	for _, app := range apps {
		if !appInstalled(app, dirs, stat) {
			continue
		}
		if err := run("open", "-a", app, url); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no installed browser found")
}

func appInstalled(app string, dirs []string, stat statFunc) bool {
	for _, dir := range dirs {
		if _, err := stat(filepath.Join(dir, app+".app")); err == nil {
			return true
		}
	}
	return false
}

func applicationDirs() []string {
	dirs := []string{
		"/Applications",
		"/System/Applications",
		"/System/Cryptexes/App/System/Applications",
		"/System/Volumes/Preboot/Cryptexes/App/System/Applications",
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}
