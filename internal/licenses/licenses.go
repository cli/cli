package licenses

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed embed/report.txt
var report string

//go:embed all:embed/third-party
var thirdParty embed.FS

func Content() string {
	return content(report, thirdParty, "embed/third-party")
}

func content(report string, thirdPartyFS fs.ReadFileFS, root string) string {
	var b strings.Builder

	b.WriteString(report)
	b.WriteString("\n")

	// Walk the third-party directory and output each license/notice file
	// grouped by module path.
	type moduleFiles struct {
		path  string
		files []string
	}

	modules := map[string]*moduleFiles{}
	fs.WalkDir(thirdPartyFS, root, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", filePath, err)
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if name == "PLACEHOLDER" {
			return nil
		}

		// Module path is the directory relative to root
		dir := filepath.Dir(filepath.FromSlash(filePath))
		rel, _ := filepath.Rel(filepath.FromSlash(root), dir)
		if _, ok := modules[rel]; !ok {
			modules[rel] = &moduleFiles{path: rel}
		}
		modules[rel].files = append(modules[rel].files, filePath)
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
			data, err := thirdPartyFS.ReadFile(filePath)
			if err != nil {
				continue
			}
			b.Write(data)
			b.WriteString("\n\n")
		}
	}

	return b.String()
}
