package githubtemplate

import (
	"io/ioutil"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Find returns the list of template file paths
func Find(rootDir string, name string) []string {
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

		// detect a single template file
		for _, file := range files {
			if strings.EqualFold(file.Name(), name+".md") {
				results = append(results, path.Join(dir, file.Name()))
				break
			}
		}
		if len(results) > 0 {
			break
		}
	}

	sort.Strings(results)
	return results
}

// ExtractName returns the name of the template from YAML front-matter
func ExtractName(filePath string) string {
	contents, err := ioutil.ReadFile(filePath)
	if err == nil && detectFrontmatter(contents)[0] == 0 {
		templateData := struct {
			Name string
		}{}
		if err := yaml.Unmarshal(contents, &templateData); err == nil && templateData.Name != "" {
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
