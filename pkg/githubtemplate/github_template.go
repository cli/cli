package githubtemplate

import (
	"fmt"
	"io/ioutil"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FindNonLegacy returns the list of template file paths from the template folder (according to the "upgraded multiple template builder")
func FindNonLegacy(rootDir string, name string) []string {
	results := []string{}

	// https://help.github.com/en/github/building-a-strong-community/creating-a-pull-request-template-for-your-repository
	candidateDirs := []string{
		path.Join(rootDir, ".github"),
		rootDir,
		path.Join(rootDir, "docs"),
	}

mainLoop:
	for _, dir := range candidateDirs {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			continue
		}

		// detect multiple templates in a subdirectory
		for _, file := range files {
			if strings.EqualFold(file.Name(), name) && file.IsDir() {
				templates, err := ioutil.ReadDir(path.Join(dir, file.Name()))
				if err != nil {
					break
				}
				for _, tf := range templates {
					if strings.HasSuffix(tf.Name(), ".md") {
						results = append(results, path.Join(dir, file.Name(), tf.Name()))
					}
				}
				if len(results) > 0 {
					break mainLoop
				}
				break
			}
		}
	}
	sort.Strings(results)
	return results
}

// FindLegacy returns the file path of the default(legacy) template
func FindLegacy(rootDir string, name string) string {
	namePattern := regexp.MustCompile(fmt.Sprintf(`(?i)^%s(\.|$)`, strings.ReplaceAll(name, "_", "[_-]")))

	// https://help.github.com/en/github/building-a-strong-community/creating-a-pull-request-template-for-your-repository
	candidateDirs := []string{
		path.Join(rootDir, ".github"),
		rootDir,
		path.Join(rootDir, "docs"),
	}
	for _, dir := range candidateDirs {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			continue
		}

		// detect a single template file
		for _, file := range files {
			if namePattern.MatchString(file.Name()) && !file.IsDir() {
				return path.Join(dir, file.Name())
			}
		}
	}
	return ""
}

// ExtractName returns the name of the template from YAML front-matter
func ExtractName(filePath string) string {
	contents, err := ioutil.ReadFile(filePath)
	frontmatterBoundaries := detectFrontmatter(contents)
	if err == nil && frontmatterBoundaries[0] == 0 {
		templateData := struct {
			Name string
		}{}
		if err := yaml.Unmarshal(contents[0:frontmatterBoundaries[1]], &templateData); err == nil && templateData.Name != "" {
			return templateData.Name
		}
	}
	return path.Base(filePath)
}

// ExtractContents returns the template contents without the YAML front-matter
func ExtractContents(filePath string) []byte {
	contents, err := ioutil.ReadFile(filePath)
	if err != nil {
		return []byte{}
	}
	if frontmatterBoundaries := detectFrontmatter(contents); frontmatterBoundaries[0] == 0 {
		return contents[frontmatterBoundaries[1]:]
	}
	return contents
}

var yamlPattern = regexp.MustCompile(`(?m)^---\r?\n(\s*\r?\n)?`)

func detectFrontmatter(c []byte) []int {
	if matches := yamlPattern.FindAllIndex(c, 2); len(matches) > 1 {
		return []int{matches[0][0], matches[1][1]}
	}
	return []int{-1, -1}
}
