package shared

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCategoryTemplateUnmarshalYAML(t *testing.T) {
	src := `
body:
  - type: markdown
    attributes:
      value: 'Please fill this in.'
  - type: input
    attributes:
      label: 'Summary'
      description: 'One line'
      value: 'default summary'
    validations:
      required: true
  - type: textarea
    attributes:
      label: 'Details'
  - type: dropdown
    attributes:
      label: 'Platform'
      multiple: true
      options:
        - 'iOS'
        - 'Android'
    validations:
      required: true
  - type: checkboxes
    attributes:
      label: 'Code of Conduct'
      options:
        - label: 'I agree'
          required: true
        - label: 'Optional extra'
`
	var tpl CategoryTemplate
	require.NoError(t, yaml.Unmarshal([]byte(src), &tpl))
	require.Len(t, tpl.Body, 5)

	assert.Equal(t, TemplateElement{Type: "markdown", Value: "Please fill this in."}, tpl.Body[0])

	assert.Equal(t, TemplateElement{
		Type:        "input",
		Label:       "Summary",
		Description: "One line",
		Value:       "default summary",
		Required:    true,
	}, tpl.Body[1])

	assert.Equal(t, TemplateElement{Type: "textarea", Label: "Details"}, tpl.Body[2])

	assert.Equal(t, TemplateElement{
		Type:     "dropdown",
		Label:    "Platform",
		Multiple: true,
		Required: true,
		Options: []TemplateOption{
			{Label: "iOS"},
			{Label: "Android"},
		},
	}, tpl.Body[3])

	assert.Equal(t, TemplateElement{
		Type:  "checkboxes",
		Label: "Code of Conduct",
		Options: []TemplateOption{
			{Label: "I agree", Required: true},
			{Label: "Optional extra"},
		},
	}, tpl.Body[4])
}

func TestCategoryTemplateUnmarshalYAMLUnknownType(t *testing.T) {
	src := `
body:
  - type: carousel
    attributes:
      label: 'Nope'
`
	var tpl CategoryTemplate
	err := yaml.Unmarshal([]byte(src), &tpl)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported Discussion Category Form element type: "carousel"`)
}

func TestFetchCategoryTemplate(t *testing.T) {
	tests := []struct {
		name      string
		httpStubs func(*httpmock.Registry)
		wantErr   error
		wantErrIs error
		wantLen   int
	}{
		{
			name: "found",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/DISCUSSION_TEMPLATE/general.yml"),
					httpmock.StringResponse("body:\n  - type: input\n    attributes:\n      label: 'X'\n"),
				)
			},
			wantLen: 1,
		},
		{
			name: "not found",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/DISCUSSION_TEMPLATE/general.yml"),
					httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
				)
			},
			wantErrIs: ErrCategoryTemplateNotFound,
		},
		{
			name: "server error",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/DISCUSSION_TEMPLATE/general.yml"),
					httpmock.StatusStringResponse(500, `{"message":"boom"}`),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.httpStubs(reg)

			httpClient := &http.Client{Transport: reg}
			repo := ghrepo.New("OWNER", "REPO")

			tpl, err := FetchCategoryTemplate(httpClient, repo, "general")
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				return
			}
			if tt.name == "server error" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "HTTP 500")
				return
			}
			require.NoError(t, err)
			assert.Len(t, tpl.Body, tt.wantLen)
		})
	}
}

func TestPromptCategoryTemplate(t *testing.T) {
	tests := []struct {
		name     string
		tpl      *CategoryTemplate
		prompter *prompter.PrompterMock
		wantBody string
		wantErr  string
	}{
		{
			name: "input, textarea, single dropdown",
			tpl: &CategoryTemplate{Body: []TemplateElement{
				{Type: "markdown", Value: "Read this first."},
				{Type: "input", Label: "Summary", Required: true},
				{Type: "textarea", Label: "Details"},
				{Type: "dropdown", Label: "Platform", Required: true, Options: []TemplateOption{
					{Label: "iOS"}, {Label: "Android"},
				}},
			}},
			prompter: &prompter.PrompterMock{
				InputFunc: func(prompt, defaultValue string) (string, error) {
					assert.Equal(t, "Summary", prompt)
					return "A summary", nil
				},
				MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
					assert.Equal(t, "Details", prompt)
					assert.True(t, blankAllowed)
					return "", nil
				},
				SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
					assert.Equal(t, "Platform", prompt)
					assert.Equal(t, []string{"iOS", "Android"}, options)
					return 1, nil
				},
			},
			wantBody: "### Summary\n\nA summary\n\n### Details\n\n_No response_\n\n### Platform\n\nAndroid\n",
		},
		{
			name: "required input left blank errors",
			tpl: &CategoryTemplate{Body: []TemplateElement{
				{Type: "input", Label: "Summary", Required: true},
			}},
			prompter: &prompter.PrompterMock{
				InputFunc: func(prompt, defaultValue string) (string, error) {
					return "   ", nil
				},
			},
			wantErr: `"Summary" is required`,
		},
		{
			name: "optional dropdown can be skipped",
			tpl: &CategoryTemplate{Body: []TemplateElement{
				{Type: "dropdown", Label: "Platform", Options: []TemplateOption{{Label: "iOS"}, {Label: "Android"}}},
			}},
			prompter: &prompter.PrompterMock{
				SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
					assert.Equal(t, []string{"iOS", "Android", skipOption}, options)
					return 2, nil
				},
			},
			wantBody: "### Platform\n\n_No response_\n",
		},
		{
			name: "multi-select dropdown joins with comma",
			tpl: &CategoryTemplate{Body: []TemplateElement{
				{Type: "dropdown", Label: "Platform", Multiple: true, Options: []TemplateOption{{Label: "iOS"}, {Label: "Android"}, {Label: "Web"}}},
			}},
			prompter: &prompter.PrompterMock{
				MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
					return []int{0, 2}, nil
				},
			},
			wantBody: "### Platform\n\niOS, Web\n",
		},
		{
			name: "checkboxes render as a task list",
			tpl: &CategoryTemplate{Body: []TemplateElement{
				{Type: "checkboxes", Label: "Checks", Options: []TemplateOption{
					{Label: "First", Required: true},
					{Label: "Second"},
				}},
			}},
			prompter: &prompter.PrompterMock{
				MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
					assert.Equal(t, []string{"First", "Second"}, options)
					return []int{0}, nil
				},
			},
			wantBody: "### Checks\n\n- [x] First\n- [ ] Second\n",
		},
		{
			name: "missing required checkbox errors",
			tpl: &CategoryTemplate{Body: []TemplateElement{
				{Type: "checkboxes", Label: "Checks", Options: []TemplateOption{
					{Label: "I agree", Required: true},
				}},
			}},
			prompter: &prompter.PrompterMock{
				MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
					return nil, nil
				},
			},
			wantErr: `"I agree" is required`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()

			body, err := PromptCategoryTemplate(tt.prompter, ios, tt.tpl)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBody, body)
			_ = stdout
		})
	}
}
