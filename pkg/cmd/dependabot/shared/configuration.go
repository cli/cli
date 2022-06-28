package shared

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/utils"

	"github.com/invopop/yaml"
	"github.com/xeipuuv/gojsonschema"
)

var possibleFilenames = [...]string{
	".github/dependabot.yml",
	".github/dependabot.yaml",
}

func FetchDependabotConfiguration(client *api.Client, repo ghrepo.Interface, ref string) ([]byte, error) {
	var response struct {
		Content string `json:"content"`
	}

	for _, filename := range possibleFilenames {
		url := GenerateDependabotConfigurationURL(repo, ref, filename, true)
		if err := client.REST(repo.RepoHost(), "GET", url, nil, &response); err == nil {
			break
		}
	}

	if response.Content == "" {
		return nil, fmt.Errorf("failed to fetch configuration")
	}

	decoded, err := base64.StdEncoding.DecodeString(response.Content)
	if err != nil {
		return nil, fmt.Errorf("error decoding content: %w", err)
	}

	return decoded, nil
}

func GenerateDependabotConfigurationURL(repo ghrepo.Interface, ref string, filename string, raw bool) string {
	if raw {
		path := fmt.Sprintf("repos/%s/contents/%s", ghrepo.FullName(repo), filename)
		if ref != "" {
			path = path + fmt.Sprintf("?ref=%s", url.QueryEscape(ref))
		}
		return ghinstance.RESTPrefix(repo.RepoHost()) + path
	} else {
		return ghrepo.GenerateRepoURL(repo, "blob/%s/%s", url.QueryEscape(ref), filename)
	}
}

func GenerateDependabotConfigurationTemplateURL(repo ghrepo.Interface, ref string) string {
	return ghrepo.GenerateRepoURL(repo, "new/%s?dependabot_template=1&filename=dependabot.yml", ref)
}

//go:embed schema.json
var schema string

func ValidateDependabotConfiguration(data []byte) error {
	data, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}

	schemaLoader := gojsonschema.NewStringLoader(schema)
	documentLoader := gojsonschema.NewBytesLoader(data)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return err
	} else if !result.Valid() {
		num := len(result.Errors())
		err := fmt.Errorf("invalid Dependabot configuration, %s found", utils.Pluralize(num, "problem"))
		for _, e := range result.Errors() {
			err = fmt.Errorf("%w\n%s", err, e.Description())
		}
		return err
	} else {
		return nil
	}
}
