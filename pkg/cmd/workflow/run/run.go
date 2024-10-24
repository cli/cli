package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type RunOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

	Selector  string
	Ref       string
	JSONInput string
	JSON      bool

	MagicFields []string
	RawFields   []string

	Prompt bool
}

type iprompter interface {
	Input(string, string) (string, error)
	Select(string, string, []string) (int, error)
}

func NewCmdRun(f *cmdutil.Factory, runF func(*RunOptions) error) *cobra.Command {
	opts := &RunOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "run [<workflow-id> | <workflow-name>]",
		Short: "Run a workflow by creating a workflow_dispatch event",
		Long: heredoc.Docf(`
			Create a %[1]sworkflow_dispatch%[1]s event for a given workflow.

			This command will trigger GitHub Actions to run a given workflow file. The given workflow file must
			support an %[1]son.workflow_dispatch%[1]s trigger in order to be run in this way.

			If the workflow file supports inputs, they can be specified in a few ways:

			- Interactively
			- Via %[1]s-f/--raw-field%[1]s or %[1]s-F/--field%[1]s flags
			- As JSON, via standard input
		`, "`"),
		Example: heredoc.Doc(`
			# Have gh prompt you for what workflow you'd like to run and interactively collect inputs
			$ gh workflow run

			# Run the workflow file 'triage.yml' at the remote's default branch
			$ gh workflow run triage.yml

			# Run the workflow file 'triage.yml' at a specified ref
			$ gh workflow run triage.yml --ref my-branch

			# Run the workflow file 'triage.yml' with command line inputs
			$ gh workflow run triage.yml -f name=scully -f greeting=hello

			# Run the workflow file 'triage.yml' with JSON via standard input
			$ echo '{"name":"scully", "greeting":"hello"}' | gh workflow run triage.yml --json
		`),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(opts.MagicFields)+len(opts.RawFields) > 0 && len(args) == 0 {
				return cmdutil.FlagErrorf("workflow argument required when passing -f or -F")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			inputFieldsPassed := len(opts.MagicFields)+len(opts.RawFields) > 0

			if len(args) > 0 {
				opts.Selector = args[0]
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("workflow ID, name, or filename required when not running interactively")
			} else {
				opts.Prompt = true
			}

			if opts.JSON && !opts.IO.IsStdinTTY() {
				jsonIn, err := io.ReadAll(opts.IO.In)
				if err != nil {
					return errors.New("failed to read from STDIN")
				}
				opts.JSONInput = string(jsonIn)
			} else if opts.JSON {
				return cmdutil.FlagErrorf("--json specified but nothing on STDIN")
			}

			if opts.Selector == "" {
				if opts.JSONInput != "" {
					return cmdutil.FlagErrorf("workflow argument required when passing JSON")
				}
			} else {
				if opts.JSON && inputFieldsPassed {
					return cmdutil.FlagErrorf("only one of STDIN or -f/-F can be passed")
				}
			}

			if runF != nil {
				return runF(opts)
			}

			return runRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "The branch or tag name which contains the version of the workflow file you'd like to run")
	cmd.Flags().StringArrayVarP(&opts.MagicFields, "field", "F", nil, "Add a string parameter in `key=value` format, respecting @ syntax (see \"gh help api\").")
	cmd.Flags().StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in `key=value` format")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Read workflow inputs as JSON via STDIN")

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "ref")

	return cmd
}

// TODO copypasta from gh api
func parseField(f string) (string, string, error) {
	idx := strings.IndexRune(f, '=')
	if idx == -1 {
		return f, "", fmt.Errorf("field %q requires a value separated by an '=' sign", f)
	}
	return f[0:idx], f[idx+1:], nil
}

// TODO MODIFIED copypasta from gh api
func magicFieldValue(v string, opts RunOptions) (string, error) {
	if strings.HasPrefix(v, "@") {
		fileBytes, err := opts.IO.ReadUserFile(v[1:])
		if err != nil {
			return "", err
		}
		return string(fileBytes), nil
	}

	return v, nil
}

// TODO copypasta from gh api
func parseFields(opts RunOptions) (map[string]string, error) {
	params := make(map[string]string)
	for _, f := range opts.RawFields {
		key, value, err := parseField(f)
		if err != nil {
			return params, err
		}
		params[key] = value
	}
	for _, f := range opts.MagicFields {
		key, strValue, err := parseField(f)
		if err != nil {
			return params, err
		}
		value, err := magicFieldValue(strValue, opts)
		if err != nil {
			return params, fmt.Errorf("error parsing %q value: %w", key, err)
		}
		params[key] = value
	}
	return params, nil
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

func collectInputs(p iprompter, yamlContent []byte) (map[string]string, error) {
	inputs, err := findInputs(yamlContent)
	if err != nil {
		return nil, err
	}

	providedInputs := map[string]string{}

	if len(inputs) == 0 {
		return providedInputs, nil
	}

	for _, input := range inputs {
		var answer string

		if input.Type == "choice" {
			name := input.Name
			if input.Required {
				name += " (required)"
			}
			selected, err := p.Select(name, input.Default, input.Options)
			if err != nil {
				return nil, err
			}
			answer = input.Options[selected]
		} else if input.Required {
			for answer == "" {
				answer, err = p.Input(input.Name+" (required)", input.Default)
				if err != nil {
					break
				}
			}
		} else {
			answer, err = p.Input(input.Name, input.Default)
		}

		if err != nil {
			return nil, err
		}

		providedInputs[input.Name] = answer
	}
	return providedInputs, nil
}

func runRun(opts *RunOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	ref := opts.Ref

	if ref == "" {
		ref, err = api.RepoDefaultBranch(client, repo)
		if err != nil {
			return fmt.Errorf("unable to determine default branch for %s: %w", ghrepo.FullName(repo), err)
		}
	}

	states := []shared.WorkflowState{shared.Active}
	workflow, err := shared.ResolveWorkflow(opts.Prompter,
		opts.IO, client, repo, opts.Prompt, opts.Selector, states)
	if err != nil {
		var fae shared.FilteredAllError
		if errors.As(err, &fae) {
			return errors.New("no workflows are enabled on this repository")
		}
		return err
	}

	defaultBranch, err := api.RepoDefaultBranch(client, repo)
	if err != nil {
		return fmt.Errorf("unable to determine default branch for %s: %w", ghrepo.FullName(repo), err)
	}

	_, err = shared.GetWorkflowContent(client, repo, *workflow, defaultBranch)
	if err != nil {
		if ref != defaultBranch {
			return fmt.Errorf("the workflow is %s not found on %s, and workflows are required to exist on the default branch to be triggered by workflow_dispatch", workflow.Base(), defaultBranch)
		}
		return fmt.Errorf("unable to fetch workflow file content: %w", err)
	}

	providedInputs := map[string]string{}

	if len(opts.MagicFields)+len(opts.RawFields) > 0 {
		providedInputs, err = parseFields(*opts)
		if err != nil {
			return err
		}
	} else if opts.JSONInput != "" {
		err := json.Unmarshal([]byte(opts.JSONInput), &providedInputs)
		if err != nil {
			return fmt.Errorf("could not parse provided JSON: %w", err)
		}
	} else if opts.Prompt {
		yamlContent, err := shared.GetWorkflowContent(client, repo, *workflow, ref)
		if err != nil {
			return fmt.Errorf("unable to fetch workflow file content: %w", err)
		}
		providedInputs, err = collectInputs(opts.Prompter, yamlContent)
		if err != nil {
			return err
		}
	}

	path := fmt.Sprintf("repos/%s/actions/workflows/%d/dispatches",
		ghrepo.FullName(repo), workflow.ID)

	requestByte, err := json.Marshal(map[string]interface{}{
		"ref":    ref,
		"inputs": providedInputs,
	})
	if err != nil {
		return fmt.Errorf("failed to serialize workflow inputs: %w", err)
	}

	body := bytes.NewReader(requestByte)

	err = client.REST(repo.RepoHost(), "POST", path, body, nil)
	if err != nil {
		return fmt.Errorf("could not create workflow dispatch event: %w", err)
	}

	if opts.IO.IsStdoutTTY() {
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
	Name        string
	Required    bool
	Default     string
	Description string
	Type        string
	Options     []string
}

func findInputs(yamlContent []byte) ([]WorkflowInput, error) {
	var rootNode yaml.Node
	err := yaml.Unmarshal(yamlContent, &rootNode)
	if err != nil {
		return nil, fmt.Errorf("unable to parse workflow YAML: %w", err)
	}

	if len(rootNode.Content) != 1 {
		return nil, errors.New("invalid YAML file")
	}

	var onKeyNode *yaml.Node
	var dispatchKeyNode *yaml.Node
	var inputsKeyNode *yaml.Node
	var inputsMapNode *yaml.Node

	// TODO this is pretty hideous
	for _, node := range rootNode.Content[0].Content {
		if onKeyNode != nil {
			if node.Kind == yaml.MappingNode {
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
			} else if node.Kind == yaml.SequenceNode {
				for _, node := range node.Content {
					if node.Value == "workflow_dispatch" {
						dispatchKeyNode = node
						break
					}
				}
			} else if node.Kind == yaml.ScalarNode {
				if node.Value == "workflow_dispatch" {
					dispatchKeyNode = node
					break
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

	out := []WorkflowInput{}

	m := map[string]WorkflowInput{}

	if inputsKeyNode == nil || inputsMapNode == nil {
		return out, nil
	}

	err = inputsMapNode.Decode(&m)
	if err != nil {
		return nil, fmt.Errorf("could not decode workflow inputs: %w", err)
	}

	for name, input := range m {
		if input.Type == "choice" && len(input.Options) == 0 {
			return nil, fmt.Errorf("workflow input %q is of type choice, but has no options", name)
		}
		out = append(out, WorkflowInput{
			Name:        name,
			Default:     input.Default,
			Description: input.Description,
			Required:    input.Required,
			Options:     input.Options,
			Type:        input.Type,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out, nil
}
