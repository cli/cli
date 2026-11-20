package shared

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"gopkg.in/yaml.v3"
)

// ErrCategoryTemplateNotFound indicates that a repository does not define a
// Discussion Category Form for the requested category slug. This is the
// common case (most categories have no form), so callers should treat it as
// a signal to fall back to a plain-text body rather than as a hard failure.
var ErrCategoryTemplateNotFound = errors.New("no Discussion Category Form found")

// CategoryTemplate is a parsed Discussion Category Form, as defined by a
// .github/DISCUSSION_TEMPLATE/<category-slug>.yml file. See:
// https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/syntax-for-githubs-form-schema
type CategoryTemplate struct {
	Body []TemplateElement `yaml:"body"`
}

// TemplateElement is a single element of a Discussion Category Form: either a
// static markdown block, or one input field (input, textarea, dropdown, or
// checkboxes).
type TemplateElement struct {
	Type        string
	Label       string
	Description string
	Value       string // markdown text (type: markdown), or a default value (type: input/textarea)
	Multiple    bool   // type: dropdown only
	Required    bool
	Options     []TemplateOption
}

// TemplateOption is a single choice within a dropdown or checkboxes element.
type TemplateOption struct {
	Label    string
	Required bool // type: checkboxes only; dropdown options cannot be individually required
}

// UnmarshalYAML decodes a single body element, dispatching on its "type" to
// pick out the fields that type supports. Unlike issue/PR templates (plain
// markdown with YAML front-matter), Discussion Category Forms have no fixed
// shape: "attributes" means something different for every element type.
func (e *TemplateElement) UnmarshalYAML(value *yaml.Node) error {
	*e = TemplateElement{}

	var shape struct {
		Type        string    `yaml:"type"`
		Attributes  yaml.Node `yaml:"attributes"`
		Validations struct {
			Required bool `yaml:"required"`
		} `yaml:"validations"`
	}
	if err := value.Decode(&shape); err != nil {
		return err
	}

	e.Type = shape.Type
	e.Required = shape.Validations.Required

	switch shape.Type {
	case "markdown":
		var attrs struct {
			Value string `yaml:"value"`
		}
		if err := shape.Attributes.Decode(&attrs); err != nil {
			return err
		}
		e.Value = attrs.Value

	case "input", "textarea":
		var attrs struct {
			Label       string `yaml:"label"`
			Description string `yaml:"description"`
			Value       string `yaml:"value"`
		}
		if err := shape.Attributes.Decode(&attrs); err != nil {
			return err
		}
		e.Label, e.Description, e.Value = attrs.Label, attrs.Description, attrs.Value

	case "dropdown":
		var attrs struct {
			Label       string   `yaml:"label"`
			Description string   `yaml:"description"`
			Multiple    bool     `yaml:"multiple"`
			Options     []string `yaml:"options"`
		}
		if err := shape.Attributes.Decode(&attrs); err != nil {
			return err
		}
		e.Label, e.Description, e.Multiple = attrs.Label, attrs.Description, attrs.Multiple
		for _, o := range attrs.Options {
			e.Options = append(e.Options, TemplateOption{Label: o})
		}

	case "checkboxes":
		var attrs struct {
			Label       string `yaml:"label"`
			Description string `yaml:"description"`
			Options     []struct {
				Label    string `yaml:"label"`
				Required bool   `yaml:"required"`
			} `yaml:"options"`
		}
		if err := shape.Attributes.Decode(&attrs); err != nil {
			return err
		}
		e.Label, e.Description = attrs.Label, attrs.Description
		for _, o := range attrs.Options {
			e.Options = append(e.Options, TemplateOption{Label: o.Label, Required: o.Required})
		}

	default:
		return fmt.Errorf("unsupported Discussion Category Form element type: %q", shape.Type)
	}

	return nil
}

// FetchCategoryTemplate fetches and parses the Discussion Category Form for
// the given category slug, from the repository's default branch. It returns
// ErrCategoryTemplateNotFound if the repository has no form defined for that
// category, which is the common case and not itself an error condition.
func FetchCategoryTemplate(httpClient *http.Client, repo ghrepo.Interface, slug string) (*CategoryTemplate, error) {
	filePath := fmt.Sprintf(".github/DISCUSSION_TEMPLATE/%s.yml", slug)
	apiPath := fmt.Sprintf("%srepos/%s/%s/contents/.github/DISCUSSION_TEMPLATE/%s.yml",
		ghinstance.RESTPrefix(repo.RepoHost()), repo.RepoOwner(), repo.RepoName(), url.PathEscape(slug))

	req, err := http.NewRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		err := api.HandleHTTPError(resp)
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, ErrCategoryTemplateNotFound
		}
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tpl CategoryTemplate
	if err := yaml.Unmarshal(body, &tpl); err != nil {
		return nil, fmt.Errorf("failed to parse Discussion Category Form %q: %w", filePath, err)
	}

	return &tpl, nil
}

// noResponse is the placeholder GitHub's web UI substitutes for an optional
// field the user left blank, so a form-generated body matches what the web
// UI would have produced.
const noResponse = "_No response_"

// PromptCategoryTemplate walks the elements of a Discussion Category Form,
// prompting the user for each input field, and assembles the responses into
// body markdown in the same "### label" section format GitHub's web UI
// produces when a Discussion is created from a category form.
func PromptCategoryTemplate(p prompter.Prompter, io *iostreams.IOStreams, tpl *CategoryTemplate) (string, error) {
	var sb strings.Builder

	for _, el := range tpl.Body {
		if el.Type == "markdown" {
			rendered, err := markdown.Render(el.Value,
				markdown.WithTheme(io.TerminalTheme()),
				markdown.WithWrap(io.TerminalWidth()))
			if err != nil {
				return "", err
			}
			fmt.Fprintln(io.Out, rendered)
			continue
		}

		if el.Description != "" {
			fmt.Fprintln(io.Out, io.ColorScheme().Muted(el.Description))
		}

		answer, err := promptTemplateElement(p, el)
		if err != nil {
			return "", err
		}

		sb.WriteString("### ")
		sb.WriteString(el.Label)
		sb.WriteString("\n\n")
		if answer == "" {
			sb.WriteString(noResponse)
		} else {
			sb.WriteString(answer)
		}
		sb.WriteString("\n\n")
	}

	return strings.TrimRight(sb.String(), "\n") + "\n", nil
}

// skipOption is offered for optional dropdowns so the user can leave them
// unanswered, since Select (unlike Input) cannot return a blank result.
const skipOption = "Skip"

func promptTemplateElement(p prompter.Prompter, el TemplateElement) (string, error) {
	switch el.Type {
	case "input":
		answer, err := p.Input(el.Label, el.Value)
		if err != nil {
			return "", err
		}
		if el.Required && strings.TrimSpace(answer) == "" {
			return "", fmt.Errorf("%q is required", el.Label)
		}
		return answer, nil

	case "textarea":
		answer, err := p.MarkdownEditor(el.Label, el.Value, !el.Required)
		if err != nil {
			return "", err
		}
		if el.Required && strings.TrimSpace(answer) == "" {
			return "", fmt.Errorf("%q is required", el.Label)
		}
		return answer, nil

	case "dropdown":
		labels := optionLabels(el.Options)

		if el.Multiple {
			idxs, err := p.MultiSelect(el.Label, nil, labels)
			if err != nil {
				return "", err
			}
			if el.Required && len(idxs) == 0 {
				return "", fmt.Errorf("%q is required", el.Label)
			}
			selected := make([]string, len(idxs))
			for i, idx := range idxs {
				selected[i] = labels[idx]
			}
			return strings.Join(selected, ", "), nil
		}

		options := labels
		if !el.Required {
			options = append(append([]string{}, labels...), skipOption)
		}
		idx, err := p.Select(el.Label, "", options)
		if err != nil {
			return "", err
		}
		if options[idx] == skipOption {
			return "", nil
		}
		return options[idx], nil

	case "checkboxes":
		labels := optionLabels(el.Options)
		idxs, err := p.MultiSelect(el.Label, nil, labels)
		if err != nil {
			return "", err
		}

		checked := make(map[int]bool, len(idxs))
		for _, idx := range idxs {
			checked[idx] = true
		}

		var missing []string
		for i, opt := range el.Options {
			if opt.Required && !checked[i] {
				missing = append(missing, opt.Label)
			}
		}
		if len(missing) > 0 {
			return "", fmt.Errorf("%q is required", strings.Join(missing, ", "))
		}

		var sb strings.Builder
		for i, label := range labels {
			if i > 0 {
				sb.WriteString("\n")
			}
			if checked[i] {
				sb.WriteString("- [x] ")
			} else {
				sb.WriteString("- [ ] ")
			}
			sb.WriteString(label)
		}
		return sb.String(), nil

	default:
		return "", fmt.Errorf("unsupported Discussion Category Form element type: %q", el.Type)
	}
}

func optionLabels(options []TemplateOption) []string {
	labels := make([]string, len(options))
	for i, o := range options {
		labels[i] = o.Label
	}
	return labels
}
