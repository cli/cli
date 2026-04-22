package preview

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/skills/discovery"
	"github.com/cli/cli/v2/internal/skills/frontmatter"
	"github.com/cli/cli/v2/internal/skills/source"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
)

type PreviewOptions struct {
	IO             *iostreams.IOStreams
	Telemetry      ghtelemetry.EventRecorder
	HttpClient     func() (*http.Client, error)
	Prompter       prompter.Prompter
	ExecutablePath string
	RenderFile     func(string, string) string

	RepoArg   string
	SkillName string
	Version   string // resolved from @suffix on SkillName

	repo ghrepo.Interface
}

// NewCmdPreview creates the "skills preview" command.
func NewCmdPreview(f *cmdutil.Factory, telemetry ghtelemetry.CommandRecorder, runF func(*PreviewOptions) error) *cobra.Command {
	opts := &PreviewOptions{
		IO:             f.IOStreams,
		Telemetry:      telemetry,
		HttpClient:     f.HttpClient,
		Prompter:       f.Prompter,
		ExecutablePath: f.ExecutablePath,
	}
	opts.RenderFile = func(filePath, content string) string {
		return renderMarkdownPreview(opts.IO, filePath, content)
	}

	cmd := &cobra.Command{
		Use:   "preview <repository> [<skill>]",
		Short: "Preview a skill from a GitHub repository (preview)",
		Long: heredoc.Docf(`
			Render a skill's %[1]sSKILL.md%[1]s content in the terminal. This fetches the
			skill file from the repository and displays it using the configured
			pager, without installing anything.

			A file tree is shown first, followed by the rendered %[1]sSKILL.md%[1]s content.
			When running interactively and the skill contains additional files
			(scripts, references, etc.), a file picker lets you browse them
			individually.

			When run with only a repository argument, lists available skills and
			prompts for selection.

			To preview a specific version of the skill, append %[1]s@VERSION%[1]s to the
			skill name. The version is resolved as a git tag, branch, or commit SHA.
		`, "`"),
		Example: heredoc.Doc(`
			# Preview a specific skill
			$ gh skill preview github/awesome-copilot documentation-writer

			# Preview a skill at a specific version
			$ gh skill preview github/awesome-copilot documentation-writer@v1.2.0

			# Preview a skill at a specific commit SHA
			$ gh skill preview github/awesome-copilot documentation-writer@abc123def456

			# Browse and preview interactively
			$ gh skill preview github/awesome-copilot
		`),
		Aliases: []string{"show"},
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			opts.RepoArg = args[0]
			if len(args) == 2 {
				opts.SkillName = args[1]
			}

			if i := strings.LastIndex(opts.SkillName, "@"); i > 0 {
				opts.Version = opts.SkillName[i+1:]
				opts.SkillName = opts.SkillName[:i]
			}

			repo, err := ghrepo.FromFullName(opts.RepoArg)
			if err != nil {
				return err
			}
			opts.repo = repo

			if runF != nil {
				return runF(opts)
			}
			return previewRun(opts)
		},
	}

	return cmd
}

func previewRun(opts *PreviewOptions) error {
	cs := opts.IO.ColorScheme()

	repo := opts.repo
	owner := repo.RepoOwner()
	repoName := repo.RepoName()
	hostname := repo.RepoHost()
	if err := source.ValidateSupportedHost(hostname); err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	// Kick off the visibility fetch in parallel with the preview work so
	// the extra API roundtrip doesn't add latency on the critical path.
	// The result is consumed when the telemetry event is emitted below.
	type visResult struct {
		vis discovery.RepoVisibility
		err error
	}
	visCh := make(chan visResult, 1)
	go func() {
		vis, err := discovery.FetchRepoVisibility(apiClient, hostname, owner, repoName)
		visCh <- visResult{vis: vis, err: err}
	}()

	opts.IO.StartProgressIndicatorWithLabel(fmt.Sprintf("Resolving %s/%s", owner, repoName))
	resolved, err := discovery.ResolveRef(apiClient, hostname, owner, repoName, opts.Version)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("could not resolve version: %w", err)
	}

	opts.IO.StartProgressIndicatorWithLabel("Discovering skills")
	skills, err := discovery.DiscoverSkills(apiClient, hostname, owner, repoName, resolved.SHA)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].DisplayName() < skills[j].DisplayName()
	})

	skill, err := selectSkill(opts, skills)
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicatorWithLabel("Fetching skill content")
	var files []discovery.SkillFile
	if skill.TreeSHA != "" {
		files, err = discovery.ListSkillFiles(apiClient, hostname, owner, repoName, skill.TreeSHA)
		if err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "warning: could not list skill files: %v\n", err)
			files = nil
		}
	}
	content, err := discovery.FetchBlob(apiClient, hostname, owner, repoName, skill.BlobSHA)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	rendered := opts.renderFile("SKILL.md", content)

	// Collect extra files (everything that isn't SKILL.md)
	var extraFiles []discovery.SkillFile
	for _, f := range files {
		if f.Path != "SKILL.md" {
			extraFiles = append(extraFiles, f)
		}
	}

	canPrompt := opts.IO.CanPrompt()

	// Non-interactive or skill has only SKILL.md: dump through pager
	if !canPrompt || len(extraFiles) == 0 {
		renderAllFiles(opts, cs, skill, files, rendered, extraFiles, apiClient, hostname, owner, repoName)
	} else {
		// Interactive with multiple files: show tree, then file picker
		renderInteractive(opts, cs, skill, files, rendered, extraFiles, apiClient, hostname, owner, repoName)
	}

	dims := map[string]string{
		"skill_host_type": ghinstance.CategorizeHost(opts.repo.RepoHost()),
	}
	select {
	case r := <-visCh:
		if r.err == nil {
			dims["repo_visibility"] = string(r.vis)
			if r.vis == discovery.RepoVisibilityPublic {
				dims["skill_owner"] = opts.repo.RepoOwner()
				dims["skill_repo"] = opts.repo.RepoName()
				dims["skill_name"] = skill.DisplayName()
			}
		} else {
			dims["repo_visibility"] = "unknown"
		}
	case <-time.After(visibilityWaitTimeout):
		dims["repo_visibility"] = "unknown"
	}
	opts.Telemetry.Record(ghtelemetry.Event{
		Type:       "skill_preview",
		Dimensions: dims,
	})

	return nil
}

// visibilityWaitTimeout is how long to wait at telemetry-emit time for
// the in-flight repo visibility fetch before giving up and emitting
// repo_visibility="unknown". By this point the command has already done
// several serial API calls and rendering work, so the fetch has almost
// always completed; this budget is a short safety net for the case
// where that single REST call has stalled.
const visibilityWaitTimeout = 200 * time.Millisecond

// renderAllFiles dumps the tree, SKILL.md, and all extra files through the pager.
func renderAllFiles(opts *PreviewOptions, cs *iostreams.ColorScheme, skill discovery.Skill,
	files []discovery.SkillFile, rendered string, extraFiles []discovery.SkillFile,
	apiClient *api.Client, hostname, owner, repo string) {

	opts.IO.DetectTerminalTheme()
	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	}
	defer opts.IO.StopPager()

	out := opts.IO.Out

	if len(files) > 0 {
		fmt.Fprintf(out, "%s\n", cs.Bold(skill.DisplayName()+"/"))
		renderFileTree(out, cs, files)
		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, "%s\n\n", cs.Bold("── SKILL.md ──"))
	fmt.Fprint(out, rendered)

	const maxFiles = 20
	const maxTotalBytes = 512 * 1024
	fetched := 0
	totalBytes := 0
	for _, f := range extraFiles {
		if fetched >= maxFiles {
			fmt.Fprintf(out, "\n%s\n", cs.Muted(fmt.Sprintf("(skipped remaining files, showing first %d)", maxFiles)))
			break
		}
		if totalBytes+f.Size > maxTotalBytes {
			fmt.Fprintf(out, "\n%s\n", cs.Muted("(skipped remaining files, size limit reached)"))
			break
		}
		fileContent, fetchErr := discovery.FetchBlob(apiClient, hostname, owner, repo, f.SHA)
		if fetchErr != nil {
			fmt.Fprintf(out, "\n%s\n\n%s\n", cs.Bold("── "+f.Path+" ──"), cs.Muted("(could not fetch file)"))
			continue
		}
		fetched++
		totalBytes += len(fileContent)
		fmt.Fprintf(out, "\n%s\n\n", cs.Bold("── "+f.Path+" ──"))
		fmt.Fprint(out, fileContent)
		if !strings.HasSuffix(fileContent, "\n") {
			fmt.Fprintln(out)
		}
	}
}

// renderInteractive shows the file tree, then a picker to browse individual files.
func renderInteractive(opts *PreviewOptions, cs *iostreams.ColorScheme, skill discovery.Skill,
	files []discovery.SkillFile, renderedSkillMD string, extraFiles []discovery.SkillFile,
	apiClient *api.Client, hostname, owner, repo string) {

	// Show the file tree to stderr so it persists above the prompt
	fmt.Fprintf(opts.IO.ErrOut, "\n%s\n", cs.Bold(skill.DisplayName()+"/"))
	renderFileTree(opts.IO.ErrOut, cs, files)
	fmt.Fprintln(opts.IO.ErrOut)

	// Build choices: SKILL.md first, then extra files
	choices := make([]string, 0, len(extraFiles)+1)
	choices = append(choices, "SKILL.md")
	for _, f := range extraFiles {
		choices = append(choices, f.Path)
	}

	// Save original stdout. StopPager closes IO.Out, so we need to
	// restore a working writer before each StartPager call.
	originalOut := opts.IO.Out

	for {
		// Restore original Out before each pager cycle. StartPager replaces
		// IO.Out with a pipe; StopPager closes that pipe but does not
		// restore the original. The original writer remains valid.
		opts.IO.Out = originalOut

		idx, err := opts.Prompter.Select("View a file (Esc to exit):", "", choices)
		if err != nil {
			return // Prompter returns error on Esc/Ctrl-C; treat as graceful exit
		}

		var content string

		if idx == 0 {
			content = renderedSkillMD
		} else {
			selectedFile := extraFiles[idx-1]

			// Fetch on demand; don't hold blob data in memory
			fileContent, fetchErr := discovery.FetchBlob(apiClient, hostname, owner, repo, selectedFile.SHA)
			if fetchErr != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s could not fetch %s: %v\n", cs.Red("!"), selectedFile.Path, fetchErr)
				continue
			}
			content = renderSelectedFilePreview(opts, selectedFile.Path, fileContent)
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
		}

		if err := opts.IO.StartPager(); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
		}
		fmt.Fprint(opts.IO.Out, content)
		opts.IO.StopPager()
	}
}

func (opts *PreviewOptions) renderFile(filePath, content string) string {
	if opts.RenderFile != nil {
		return opts.RenderFile(filePath, content)
	}

	return renderMarkdownPreview(opts.IO, filePath, content)
}

func renderSelectedFilePreview(opts *PreviewOptions, filePath, content string) string {
	if !isMarkdownFile(filePath) {
		return content
	}

	return opts.renderFile(filePath, content)
}

func renderMarkdownPreview(io *iostreams.IOStreams, filePath, content string) string {
	if filePath == "SKILL.md" {
		parsed, err := frontmatter.Parse(content)
		if err == nil {
			content = parsed.Body
		}
	}

	rendered, err := markdown.Render(content,
		markdown.WithTheme(io.TerminalTheme()),
		markdown.WithWrap(io.TerminalWidth()),
		markdown.WithoutIndentation())
	if err != nil {
		return content
	}

	return rendered
}

func isMarkdownFile(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".markdown", ".mdown", ".mkd", ".mkdn":
		return true
	default:
		return false
	}
}

func selectSkill(opts *PreviewOptions, skills []discovery.Skill) (discovery.Skill, error) {
	if opts.SkillName != "" {
		for _, s := range skills {
			if s.DisplayName() == opts.SkillName || s.Name == opts.SkillName {
				return s, nil
			}
		}
		// Fall back to InstallName so that namespaced identifiers produced
		// by the post-install hint (e.g. "namespace/skill") are accepted.
		for _, s := range skills {
			if s.InstallName() == opts.SkillName {
				return s, nil
			}
		}
		return discovery.Skill{}, fmt.Errorf("skill %q not found in %s", opts.SkillName, ghrepo.FullName(opts.repo))
	}

	if !opts.IO.CanPrompt() {
		return discovery.Skill{}, fmt.Errorf("must specify a skill name when not running interactively")
	}

	choices := make([]string, len(skills))
	for i, s := range skills {
		choices[i] = s.DisplayName()
	}

	idx, err := opts.Prompter.Select("Select a skill to preview:", "", choices)
	if err != nil {
		return discovery.Skill{}, err
	}

	return skills[idx], nil
}

// treeNode represents a file or directory in the tree for rendering.
type treeNode struct {
	name     string
	children []*treeNode
	isDir    bool
}

// renderFileTree prints a tree of skill files using box-drawing characters.
func renderFileTree(w io.Writer, cs *iostreams.ColorScheme, files []discovery.SkillFile) {
	root := buildTree(files)
	printTree(w, cs, root.children, "")
}

// buildTree constructs a tree structure from flat file paths.
func buildTree(files []discovery.SkillFile) *treeNode {
	root := &treeNode{isDir: true}
	for _, f := range files {
		parts := strings.Split(f.Path, "/")
		current := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			found := false
			for _, child := range current.children {
				if child.name == part {
					current = child
					found = true
					break
				}
			}
			if !found {
				node := &treeNode{name: part, isDir: !isLast}
				current.children = append(current.children, node)
				current = node
			}
		}
	}
	sortTree(root)
	return root
}

func sortTree(node *treeNode) {
	sort.Slice(node.children, func(i, j int) bool {
		if node.children[i].isDir != node.children[j].isDir {
			return node.children[i].isDir
		}
		return node.children[i].name < node.children[j].name
	})
	for _, child := range node.children {
		if child.isDir {
			sortTree(child)
		}
	}
}

func printTree(w io.Writer, cs *iostreams.ColorScheme, nodes []*treeNode, indent string) {
	for i, node := range nodes {
		isLast := i == len(nodes)-1
		connector := "├── "
		childIndent := "│   "
		if isLast {
			connector = "└── "
			childIndent = "    "
		}
		if node.isDir {
			fmt.Fprintf(w, "%s%s%s\n", indent, cs.Muted(connector), cs.Bold(node.name+"/"))
			printTree(w, cs, node.children, indent+cs.Muted(childIndent))
		} else {
			fmt.Fprintf(w, "%s%s%s\n", indent, cs.Muted(connector), node.name)
		}
	}
}
