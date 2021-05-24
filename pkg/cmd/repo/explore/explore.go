package explore

import (
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alecthomas/chroma/quick"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

type ExploreOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	RepoArg    string
	Branch     string
}

func NewCmdExplore(f *cmdutil.Factory, runF func(*ExploreOptions) error) *cobra.Command {
	opts := ExploreOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "explore [<repository>]",
		Short: "Explore a repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			if runF != nil {
				return runF(&opts)
			}
			return exploreRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Explore a specific branch of the repository")

	return cmd
}

func exploreRun(opts *ExploreOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var toExplore ghrepo.Interface
	cachedClient := api.NewCachedClient(httpClient, time.Hour*24)
	apiClient := api.NewClientFromHTTP(cachedClient)

	if opts.RepoArg == "" {
		var err error
		toExplore, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		var err error
		exploreURL := opts.RepoArg
		if !strings.Contains(exploreURL, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
			if err != nil {
				return err
			}
			exploreURL = currentUser + "/" + exploreURL
		}
		toExplore, err = ghrepo.FromFullName(exploreURL)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	}

	ref := opts.Branch
	if ref == "" {
		ref, err = api.RepoDefaultBranch(apiClient, toExplore)
		if err != nil {
			return err
		}
	}

	tree, err := repositoryTree(apiClient, toExplore, ref)
	if err != nil {
		return err
	}

	treeView := buildTreeView(toExplore, tree)
	treeView.SetBorder(true)

	fileView := tview.NewTextView()
	fileView.SetDynamicColors(true)
	fileView.SetBorder(true)

	selectedFunc := selectTreeNode(apiClient, toExplore, ref, fileView)
	treeView.SetSelectedFunc(selectedFunc)

	label := ghrepo.FullName(toExplore)
	searchView := tview.NewInputField()
	searchView.SetBorder(true)
	searchView.SetLabel(label)
	searchView.SetFieldBackgroundColor(0)
	searchView.SetLabelWidth(len(label) + 1)

	searchFunc := searchTree(toExplore, tree, treeView)
	searchView.SetChangedFunc(searchFunc)

	app := tview.NewApplication()

	topRow := tview.NewFlex().
		AddItem(treeView, 0, 1, false).
		AddItem(fileView, 0, 4, false)

	bottomRow := tview.NewFlex().
		AddItem(searchView, 0, 1, false)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topRow, 0, 16, false).
		AddItem(bottomRow, 0, 1, false)

	err = app.SetRoot(flex, true).EnableMouse(true).SetFocus(searchView).Run()
	return err
}

func buildTreeView(repo ghrepo.Interface, rt RepoTree) *tview.TreeView {
	rootDir := ghrepo.FullName(repo)
	root := tview.NewTreeNode(rootDir).SetColor(tcell.ColorRed)
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	searchTree(repo, rt, tree)("")
	return tree
}

func searchTree(repo ghrepo.Interface, rt RepoTree, treeView *tview.TreeView) func(string) {
	return func(query string) {
		root := treeView.GetRoot()
		root.ClearChildren()
		dirs := map[string]*tview.TreeNode{".": root}
		r := regexp.MustCompile(query)

		for _, n := range rt {
			if n.IsDir() || !r.MatchString(n.Path) {
				continue
			}
			node := tview.NewTreeNode(n.Name())
			node.SetReference(n)
			parentNode := makeParentNodes(dirs, n.Dir(), len(query) != 0)
			parentNode.AddChild(node)
		}
	}
}

func makeParentNodes(dirs map[string]*tview.TreeNode, dir string, expanded bool) *tview.TreeNode {
	parentNode := dirs[dir]
	if parentNode != nil {
		return parentNode
	}
	parentNode = makeParentNodes(dirs, filepath.Dir(dir), expanded)
	node := tview.NewTreeNode(dir)
	node.SetReference(RepoTreeNode{Path: dir, Type: "tree"})
	node.SetColor(tcell.ColorGreen)
	node.SetExpanded(expanded)
	parentNode.AddChild(node)
	dirs[dir] = node
	return node
}

func selectTreeNode(client *api.Client, repo ghrepo.Interface, branch string, fileView *tview.TextView) func(*tview.TreeNode) {
	return func(node *tview.TreeNode) {
		reference := node.GetReference()
		if reference == nil {
			return
		}
		rtn := reference.(RepoTreeNode)
		if rtn.IsDir() {
			node.SetExpanded(!node.IsExpanded())
			return
		}

		fileBytes, err := repositoryFileContent(client, repo, branch, rtn.Path)
		if err != nil {
			return
		}
		file := string(fileBytes)
		fileView.Clear()
		coloredFileView := tview.ANSIWriter(fileView)
		_ = quick.Highlight(coloredFileView, file, rtn.Ext(), "terminal256", "solarized-dark")
		fileView.ScrollToBeginning()
	}
}
