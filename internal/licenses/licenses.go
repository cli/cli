package licenses

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Content returns the full license report, including the main report and all
// third-party licenses.
func Content() string {
	return content(embedFS, rootDir)
}

func content(embedFS fs.ReadFileFS, rootDir string) string {
	var b strings.Builder

	reportPath := path.Join(rootDir, "report.txt")
	thirdPartyPath := path.Join(rootDir, "third-party")

	report, err := fs.ReadFile(embedFS, reportPath)
	if err != nil {
		return "License information is only available in official release builds.\n"
	}

	b.Write(report)
	b.WriteString("\n")

	// Walk the third-party directory and output each license/notice file
	// grouped by module path.
	type moduleFiles struct {
		path  string
		files []string
	}

	thirdPartyFS, err := fs.Sub(embedFS, thirdPartyPath)
	if err != nil {
		return b.String()
	}

	modules := map[string]*moduleFiles{}
	fs.WalkDir(thirdPartyFS, ".", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", filePath, err)
		}

		if d.IsDir() {
			return nil
		}

		dir := path.Dir(filePath)
		if _, ok := modules[dir]; !ok {
			modules[dir] = &moduleFiles{path: dir}
		}
		modules[dir].files = append(modules[dir].files, filePath)
		return nil
	})

	// Sort modules by path for deterministic output
	sorted := make([]string, 0, len(modules))
	for k := range modules {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, modPath := range sorted {
		mod := modules[modPath]
		b.WriteString("================================================================================\n")
		fmt.Fprintf(&b, "%s\n", mod.path)
		b.WriteString("================================================================================\n\n")

		for _, filePath := range mod.files {
			data, err := fs.ReadFile(thirdPartyFS, filePath)
			if err != nil {
				continue
			}
			b.Write(data)
			b.WriteString("\n\n")
		}
	}

	return b.String()
}
