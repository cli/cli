package cliutil

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// IsolatedEnvironment creates private profile directories without inheriting process variables.
// Prefix controls directory naming, for example "." for hidden rendering scratch directories.
func IsolatedEnvironment(root, prefix string) (map[string]string, error) {
	directories := map[string]string{}
	for _, name := range []string{"home", "config", "cache", "tmp"} {
		path := filepath.Join(root, prefix+name)
		if !Inside(root, path) {
			return nil, fmt.Errorf("profile directory leaves its workspace")
		}
		if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("profile directories must not be symbolic links")
		}
		directories[name] = path
	}
	return map[string]string{
		"HOME": directories["home"], "USERPROFILE": directories["home"],
		"XDG_CONFIG_HOME": directories["config"], "APPDATA": directories["config"],
		"XDG_CACHE_HOME": directories["cache"], "LOCALAPPDATA": directories["cache"],
		"TMPDIR": directories["tmp"], "TMP": directories["tmp"], "TEMP": directories["tmp"],
	}, nil
}

// EnvironmentList converts explicit environment values into deterministic subprocess arguments.
func EnvironmentList(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	slices.Sort(result)
	return result
}
