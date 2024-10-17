package shared

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/surveyext"
)

type Action int

const (
	SubmitAction Action = iota
	PreviewAction
	CancelAction
	MetadataAction
	EditCommitMessageAction
	EditCommitSubjectAction
	SubmitDraftAction

	noMilestone = "(none)"

	submitLabel      = "Submit"
	submitDraftLabel = "Submit as draft"
	previewLabel     = "Continue in browser"
	metadataLabel    = "Add metadata"
	cancelLabel      = "Cancel"
)

type Prompt interface {
	Input(string, string) (string, error)
	Select(string, string, []string) (int, error)
	MarkdownEditor(string, string, bool) (string, error)
	Confirm(string, bool) (bool, error)
	MultiSelect(string, []string, []string) ([]int, error)
}

func ConfirmIssueSubmission(p Prompt, allowPreview bool, allowMetadata bool) (Action, error) {
	return confirmSubmission(p, allowPreview, allowMetadata, false, false)
}

func ConfirmPRSubmission(p Prompt, allowPreview, allowMetadata, isDraft bool) (Action, error) {
	return confirmSubmission(p, allowPreview, allowMetadata, true, isDraft)
}

func confirmSubmission(p Prompt, allowPreview, allowMetadata, allowDraft, isDraft bool) (Action, error) {
	var options []string
	if !isDraft {
		options = append(options, submitLabel)
	}
	if allowDraft {
		options = append(options, submitDraftLabel)
	}
	if allowPreview {
		options = append(options, previewLabel)
	}
	if allowMetadata {
		options = append(options, metadataLabel)
	}
	options = append(options, cancelLabel)

	result, err := p.Select("What's next?", "", options)
	if err != nil {
		return -1, fmt.Errorf("could not prompt: %w", err)
	}

	switch options[result] {
	case submitLabel:
		return SubmitAction, nil
	case submitDraftLabel:
		return SubmitDraftAction, nil
	case previewLabel:
		return PreviewAction, nil
	case metadataLabel:
		return MetadataAction, nil
	case cancelLabel:
		return CancelAction, nil
	default:
		return -1, fmt.Errorf("invalid index: %d", result)
	}
}

func BodySurvey(p Prompt, state *IssueMetadataState, templateContent string) error {
	if templateContent != "" {
		if state.Body != "" {
			// prevent excessive newlines between default body and template
			state.Body = strings.TrimRight(state.Body, "\n")
			state.Body += "\n\n"
		}
		state.Body += templateContent
	}

	result, err := p.MarkdownEditor("Body", state.Body, true)
	if err != nil {
		return err
	}

	if state.Body != result {
		state.MarkDirty()
	}

	state.Body = result

	return nil
}

func TitleSurvey(p Prompt, io *iostreams.IOStreams, state *IssueMetadataState) error {
	var err error
	result := ""
	for result == "" {
		result, err = p.Input("Title (required)", state.Title)
		if err != nil {
			return err
		}
		if result == "" {
			fmt.Fprintf(io.ErrOut, "%s Title cannot be blank\n", io.ColorScheme().FailureIcon())
		}
	}

	if result != state.Title {
		state.MarkDirty()
	}

	state.Title = result

	return nil
}

type MetadataFetcher struct {
	IO        *iostreams.IOStreams
	APIClient *api.Client
	Repo      ghrepo.Interface
	State     *IssueMetadataState
}

func (mf *MetadataFetcher) RepoMetadataFetch(input api.RepoMetadataInput) (*api.RepoMetadataResult, error) {
	mf.IO.StartProgressIndicator()
	metadataResult, err := api.RepoMetadata(mf.APIClient, mf.Repo, input)
	mf.IO.StopProgressIndicator()
	mf.State.MetadataResult = metadataResult
	return metadataResult, err
}

type RepoMetadataFetcher interface {
	RepoMetadataFetch(api.RepoMetadataInput) (*api.RepoMetadataResult, error)
}

func MetadataSurvey(p Prompt, io *iostreams.IOStreams, baseRepo ghrepo.Interface, fetcher RepoMetadataFetcher, state *IssueMetadataState) error {
	isChosen := func(m string) bool {
		for _, c := range state.Metadata {
			if m == c {
				return true
			}
		}
		return false
	}

	allowReviewers := state.Type == PRMetadata

	extraFieldsOptions := []string{}
	if allowReviewers {
		extraFieldsOptions = append(extraFieldsOptions, "Reviewers")
	}
	extraFieldsOptions = append(extraFieldsOptions, "Assignees", "Labels", "Projects", "Milestone")

	selected, err := p.MultiSelect("What would you like to add?", nil, extraFieldsOptions)
	if err != nil {
		return err
	}
	for _, i := range selected {
		state.Metadata = append(state.Metadata, extraFieldsOptions[i])
	}

	metadataInput := api.RepoMetadataInput{
		Reviewers:  isChosen("Reviewers"),
		Assignees:  isChosen("Assignees"),
		Labels:     isChosen("Labels"),
		Projects:   isChosen("Projects"),
		Milestones: isChosen("Milestone"),
	}
	metadataResult, err := fetcher.RepoMetadataFetch(metadataInput)
	if err != nil {
		return fmt.Errorf("error fetching metadata options: %w", err)
	}

	var reviewers []string
	for _, u := range metadataResult.AssignableUsers {
		if u.Login != metadataResult.CurrentLogin {
			reviewers = append(reviewers, u.DisplayName())
		}
	}
	for _, t := range metadataResult.Teams {
		reviewers = append(reviewers, fmt.Sprintf("%s/%s", baseRepo.RepoOwner(), t.Slug))
	}
	var assignees []string
	for _, u := range metadataResult.AssignableUsers {
		assignees = append(assignees, u.DisplayName())
	}
	var labels []string
	for _, l := range metadataResult.Labels {
		labels = append(labels, l.Name)
	}
	var projects []string
	for _, p := range metadataResult.Projects {
		projects = append(projects, p.Name)
	}
	for _, p := range metadataResult.ProjectsV2 {
		projects = append(projects, p.Title)
	}
	milestones := []string{noMilestone}
	for _, m := range metadataResult.Milestones {
		milestones = append(milestones, m.Title)
	}

	values := struct {
		Reviewers []string
		Assignees []string
		Labels    []string
		Projects  []string
		Milestone string
	}{}

	if isChosen("Reviewers") {
		if len(reviewers) > 0 {
			selected, err := p.MultiSelect("Reviewers", state.Reviewers, reviewers)
			if err != nil {
				return err
			}
			for _, i := range selected {
				values.Reviewers = append(values.Reviewers, reviewers[i])
			}
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no available reviewers")
		}
	}
	if isChosen("Assignees") {
		if len(assignees) > 0 {
			selected, err := p.MultiSelect("Assignees", state.Assignees, assignees)
			if err != nil {
				return err
			}
			for _, i := range selected {
				values.Assignees = append(values.Assignees, assignees[i])
			}
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no assignable users")
		}
	}
	if isChosen("Labels") {
		if len(labels) > 0 {
			selected, err := p.MultiSelect("Labels", state.Labels, labels)
			if err != nil {
				return err
			}
			for _, i := range selected {
				values.Labels = append(values.Labels, labels[i])
			}
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no labels in the repository")
		}
	}
	if isChosen("Projects") {
		if len(projects) > 0 {
			selected, err := p.MultiSelect("Projects", state.Projects, projects)
			if err != nil {
				return err
			}
			for _, i := range selected {
				values.Projects = append(values.Projects, projects[i])
			}
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no projects to choose from")
		}
	}
	if isChosen("Milestone") {
		if len(milestones) > 1 {
			var milestoneDefault string
			if len(state.Milestones) > 0 {
				milestoneDefault = state.Milestones[0]
			} else {
				milestoneDefault = milestones[1]
			}
			selected, err := p.Select("Milestone", milestoneDefault, milestones)
			if err != nil {
				return err
			}
			values.Milestone = milestones[selected]
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no milestones in the repository")
		}
	}

	if isChosen("Reviewers") {
		var logins []string
		for _, r := range values.Reviewers {
			// Extract user login from display name
			logins = append(logins, (strings.Split(r, " "))[0])
		}
		state.Reviewers = logins
	}
	if isChosen("Assignees") {
		var logins []string
		for _, a := range values.Assignees {
			// Extract user login from display name
			logins = append(logins, (strings.Split(a, " "))[0])
		}
		state.Assignees = logins
	}
	if isChosen("Labels") {
		state.Labels = values.Labels
	}
	if isChosen("Projects") {
		state.Projects = values.Projects
	}
	if isChosen("Milestone") {
		if values.Milestone != "" && values.Milestone != noMilestone {
			state.Milestones = []string{values.Milestone}
		} else {
			state.Milestones = []string{}
		}
	}

	return nil
}

type Editor interface {
	Edit(filename, initialValue string) (string, error)
}

type UserEditor struct {
	IO     *iostreams.IOStreams
	Config func() (gh.Config, error)
}

func (e *UserEditor) Edit(filename, initialValue string) (string, error) {
	editorCommand, err := cmdutil.DetermineEditor(e.Config)
	if err != nil {
		return "", err
	}
	return surveyext.Edit(editorCommand, filename, initialValue, e.IO.In, e.IO.Out, e.IO.ErrOut)
}

const editorHintMarker = "------------------------ >8 ------------------------"
const editorHint = `
Please Enter the title on the first line and the body on subsequent lines.
Lines below dotted lines will be ignored, and an empty title aborts the creation process.`

func TitledEditSurvey(editor Editor) func(string, string) (string, string, error) {
	return func(initialTitle, initialBody string) (string, string, error) {
		initialValue := strings.Join([]string{initialTitle, initialBody, editorHintMarker, editorHint}, "\n")
		titleAndBody, err := editor.Edit("*.md", initialValue)
		if err != nil {
			return "", "", err
		}

		titleAndBody = strings.ReplaceAll(titleAndBody, "\r\n", "\n")
		titleAndBody, _, _ = strings.Cut(titleAndBody, editorHintMarker)
		title, body, _ := strings.Cut(titleAndBody, "\n")
		return title, strings.TrimSuffix(body, "\n"), nil
	}
}

func InitEditorMode(f *cmdutil.Factory, editorMode bool, webMode bool, canPrompt bool) (bool, error) {
	if err := cmdutil.MutuallyExclusive(
		"specify only one of `--editor` or `--web`",
		editorMode,
		webMode,
	); err != nil {
		return false, err
	}

	config, err := f.Config()
	if err != nil {
		return false, err
	}

	editorMode = !webMode && (editorMode || config.PreferEditorPrompt("").Value == "enabled")

	if editorMode && !canPrompt {
		return false, errors.New("--editor or enabled prefer_editor_prompt configuration are not supported in non-tty mode")
	}

	return editorMode, nil
}
