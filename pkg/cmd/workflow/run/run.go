package run

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/workflow/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

type RunOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Selector string
	Ref      string
	JSON     string

	InputArgs []string

	Prompt bool
}

func NewCmdRun(f *cmdutil.Factory, runF func(*RunOptions) error) *cobra.Command {
	opts := &RunOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "run [<workflow ID> | <workflow name>]",
		Short: "Create a dispatch event for a workflow, starting a run",
		Long: heredoc.Doc(`
			Create a workflow_dispatch event for a given workflow.

			This command will trigger GitHub Actions to run a given workflow file.  The given workflow file must
			support a workflow_dispatch 'on' trigger in order to be run in this way.

			If the workflow file supports inputs, they can be specified in a few ways:

			- Interactively
			- As command line arguments specified after --
			- As JSON, via STDIN
			- As JSON, via --json
    `),
		Example: heredoc.Doc(`
			# Have gh prompt you for what workflow you'd like to run
			$ gh workflow run

			# Run the workflow file 'triage.yml' at the remote's default branch, interactively providing inputs
			$ gh workflow run triage.yml

			# Run the workflow file 'triage.yml' at a specified ref, interactively providing inputs
			$ gh workflow run triage.yml --ref=myBranch

			# Run the workflow file 'triage.yml' with command line inputs
			$ gh workflow run triage.yml -- --name=scully --greeting=hello

			# Run the workflow file 'triage.yml' with JSON via STDIN
			$ echo '{"name":"scully", "greeting":"hello"}' | gh workflow run triage.yml

			# Run the workflow file 'triage.yml' with JSON via --json
			$ gh workflow run triage.yml --json='{"name":"scully", "greeting":"hello"}'
		`),
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() == 0 && len(args[1:]) > 0 {
				return cmdutil.FlagError{Err: fmt.Errorf("workflow argument required when passing input flags")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.Selector = args[0]
				opts.InputArgs = args[1:]
			} else if !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("workflow ID, name, or filename required when not running interactively")}
			} else {
				opts.Prompt = true
			}

			if !opts.IO.IsStdinTTY() {
				if opts.JSON != "" {
					return errors.New("JSON can only be passed on one of STDIN or --json at a time")
				}
				jsonIn, err := ioutil.ReadAll(opts.IO.In)
				if err != nil {
					return errors.New("failed to read from STDIN")
				}
				opts.JSON = string(jsonIn)
			}

			if opts.Selector == "" {
				if opts.JSON != "" {
					return &cmdutil.FlagError{Err: errors.New("workflow argument required when passing JSON")}
				}
			} else {
				if opts.JSON != "" && len(opts.InputArgs) > 0 {
					return &cmdutil.FlagError{Err: errors.New("only one of JSON or input arguments can be passed at a time")}
				}
			}

			if runF != nil {
				return runF(opts)
			}
			return runRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "The branch or tag name which contains the version of the workflow file you'd like to run")
	cmd.Flags().StringVar(&opts.JSON, "json", "", "Provide workflow inputs as JSON")

	return cmd
}

type InputAnswer struct {
	providedInputs map[string]string
}

func (ia *InputAnswer) WriteAnswer(name string, value interface{}) error {
	if s, ok := value.(string); ok {
		ia.providedInputs[name] = s
		return nil
	}

	// TODO i hate this; this is to make tests work:
	if rv, ok := value.(reflect.Value); ok {
		ia.providedInputs[name] = rv.String()
		return nil
	}

	return fmt.Errorf("unexpected value type: %v", value)
}

func runRun(opts *RunOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	states := []shared.WorkflowState{shared.Active}
	workflow, err := shared.ResolveWorkflow(
		opts.IO, client, repo, opts.Prompt, opts.Selector, states)
	if err != nil {
		var fae shared.FilteredAllError
		if errors.As(err, &fae) {
			return errors.New("no workflows are enabled on this repository")
		}
		return err
	}

	ref := opts.Ref

	if ref == "" {
		ref, err = api.RepoDefaultBranch(client, repo)
		if err != nil {
			return fmt.Errorf("unable to determine default branch for %s: %w", ghrepo.FullName(repo), err)
		}
	}

	yamlContent, err := getWorkflowContent(client, repo, workflow, ref)
	if err != nil {
		return fmt.Errorf("unable to fetch workflow file content: %w", err)
	}

	inputs, err := findInputs(yamlContent)
	if err != nil {
		return err
	}

	providedInputs := map[string]string{}

	if opts.Prompt {
		qs := []*survey.Question{}
		for inputName, input := range inputs {
			q := &survey.Question{
				Name: inputName,
				Prompt: &survey.Input{
					Message: inputName,
					Default: input.Default,
				},
			}
			if input.Required {
				q.Validate = survey.Required
			}
			qs = append(qs, q)
		}

		sort.Slice(qs, func(i, j int) bool {
			return qs[i].Name < qs[j].Name
		})

		inputAnswer := InputAnswer{
			providedInputs: providedInputs,
		}
		err = prompt.SurveyAsk(qs, &inputAnswer)
		if err != nil {
			return err
		}
	} else {
		if opts.JSON != "" {
			err := json.Unmarshal([]byte(opts.JSON), &providedInputs)
			if err != nil {
				return fmt.Errorf("could not parse provided JSON: %w", err)
			}
		}

		if len(opts.InputArgs) > 0 {
			fs := pflag.FlagSet{}
			for inputName, input := range inputs {
				fs.String(inputName, input.Default, input.Description)
			}
			err = fs.Parse(opts.InputArgs)
			if err != nil {
				return fmt.Errorf("could not parse input args: %w", err)
			}
			for inputName := range inputs {
				providedValue, _ := fs.GetString(inputName)
				providedInputs[inputName] = providedValue
			}
		}
	}

	for inputName, inputValue := range inputs {
		if inputValue.Required && providedInputs[inputName] == "" {
			return fmt.Errorf("missing required input '%s'", inputName)
		}
	}

	path := fmt.Sprintf("repos/%s/actions/workflows/%d/dispatches",
		ghrepo.FullName(repo), workflow.ID)

	requestByte, err := json.Marshal(struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}{
		Ref:    ref,
		Inputs: providedInputs,
	})
	if err != nil {
		return fmt.Errorf("failed to serialize workflow inputs: %w", err)
	}

	body := bytes.NewReader(requestByte)

	err = client.REST(repo.RepoHost(), "POST", path, body, nil)
	if err != nil {
		return fmt.Errorf("could not create workflow dispatch event: %w", err)
	}

	if opts.IO.CanPrompt() {
		out := opts.IO.Out
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(out, "%s Created workflow_dispatch event for %s at %s\n",
			cs.SuccessIcon(), cs.Cyan(workflow.Base()), cs.Bold(ref))

		fmt.Fprintln(out)

		fmt.Fprintf(out, "To see runs for this workflow, try: %s\n",
			cs.Boldf("gh run list --workflow=%s", workflow.Base()))
	}

	return nil
}

type WorkflowInput struct {
	Required    bool
	Default     string
	Description string
}

func findInputs(yamlContent []byte) (map[string]WorkflowInput, error) {
	var rootNode yaml.Node
	err := yaml.Unmarshal(yamlContent, &rootNode)
	if err != nil {
		return nil, fmt.Errorf("unable to parse workflow YAML: %w", err)
	}

	if len(rootNode.Content) != 1 {
		return nil, errors.New("invalid yaml file")
	}

	var onKeyNode *yaml.Node
	var dispatchKeyNode *yaml.Node
	var inputsKeyNode *yaml.Node
	var inputsMapNode *yaml.Node

	// TODO this is pretty hideous
	for _, node := range rootNode.Content[0].Content {
		if onKeyNode != nil {
			for _, node := range node.Content {
				if dispatchKeyNode != nil {
					for _, node := range node.Content {
						if inputsKeyNode != nil {
							inputsMapNode = node
							break
						}
						if node.Value == "inputs" {
							inputsKeyNode = node
						}
					}
					break
				}
				if node.Value == "workflow_dispatch" {
					dispatchKeyNode = node
				}
			}
			break
		}
		if strings.EqualFold(node.Value, "on") {
			onKeyNode = node
		}
	}

	if onKeyNode == nil {
		return nil, errors.New("invalid workflow: no 'on' key")
	}

	if dispatchKeyNode == nil {
		return nil, errors.New("unable to manually run a workflow without a workflow_dispatch event")
	}

	out := map[string]WorkflowInput{}

	if inputsKeyNode == nil || inputsMapNode == nil {
		return out, nil
	}

	err = inputsMapNode.Decode(&out)
	if err != nil {
		return nil, fmt.Errorf("could not decode workflow inputs: %w", err)
	}

	return out, nil
}

func getWorkflowContent(client *api.Client, repo ghrepo.Interface, workflow *shared.Workflow, ref string) ([]byte, error) {
	path := fmt.Sprintf("repos/%s/contents/%s?ref=%s", ghrepo.FullName(repo), workflow.Path, url.QueryEscape(ref))

	type Result struct {
		Content string
	}

	var result Result
	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(result.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode workflow file: %w", err)
	}

	return decoded, nil
}
