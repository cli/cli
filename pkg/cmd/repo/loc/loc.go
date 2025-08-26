package loc

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type LinesOfCodeOptions struct {
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	GitClient  *git.Client
	Exporter   cmdutil.Exporter

	ExtensionFilter string
	ExcludeFiles    []string
	IncludeFiles    []string
	ShowByFileType  bool
	ShowDetailed    bool
}

type FileStats struct {
	Path       string
	Lines      int
	CodeLines  int
	BlankLines int
	Extension  string
}

type TypeStats struct {
	Extension  string
	FileCount  int
	TotalLines int
	CodeLines  int
	BlankLines int
}

func NewCmdLinesOfCode(f *cmdutil.Factory, runF func(*LinesOfCodeOptions) error) *cobra.Command {
	opts := &LinesOfCodeOptions{
		IO:        f.IOStreams,
		BaseRepo:  f.BaseRepo,
		GitClient: f.GitClient,
	}

	cmd := &cobra.Command{
		Use:   "loc",
		Short: "Count lines of code in repository",
		Long: heredoc.Doc(`
			Count lines of code in a Git repository using Git's file tracking.
			
			This command uses 'git ls-files' to get tracked files, which means it respects
			.gitignore files and only counts files that are actually tracked by Git.
			This is more accurate than using find or other filesystem-based tools.
		`),
		Example: heredoc.Doc(`
			# Count all lines of code in current repository
			$ gh repo loc
			
			# Count only TypeScript files
			$ gh repo loc --extension ts
			
			# Show breakdown by file type
			$ gh repo loc --by-type
			
			# Show detailed file-by-file breakdown
			$ gh repo loc --detailed
			
			# Exclude specific files
			$ gh repo loc --exclude "*.test.js" --exclude "*.spec.ts"
			
			# Include only specific files
			$ gh repo loc --include "src/**/*.go"
		`),
		RunE: func(c *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return linesOfCodeRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.ExtensionFilter, "extension", "e", "", "Filter by file extension (e.g., go, js, ts)")
	cmd.Flags().StringArrayVar(&opts.ExcludeFiles, "exclude", []string{}, "Exclude files matching pattern")
	cmd.Flags().StringArrayVar(&opts.IncludeFiles, "include", []string{}, "Include only files matching pattern")
	cmd.Flags().BoolVarP(&opts.ShowByFileType, "by-type", "t", false, "Show breakdown by file type")
	cmd.Flags().BoolVarP(&opts.ShowDetailed, "detailed", "d", false, "Show detailed file-by-file breakdown")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, []string{"path", "lines", "codeLines", "blankLines", "extension"})

	return cmd
}

func linesOfCodeRun(opts *LinesOfCodeOptions) error {
	ctx := context.Background()

	// Check if we're in a git repository
	isRepo, err := opts.GitClient.IsLocalGitRepo(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if directory is a git repository: %w", err)
	}
	if !isRepo {
		return fmt.Errorf("not in a git repository")
	}

	// Get list of tracked files using git ls-files
	files, err := getTrackedFiles(ctx, opts.GitClient)
	if err != nil {
		return fmt.Errorf("failed to get tracked files: %w", err)
	}

	// Filter files based on options
	filteredFiles := filterFiles(files, opts)

	if len(filteredFiles) == 0 {
		fmt.Fprintln(opts.IO.Out, "No files match the specified criteria")
		return nil
	}

	// Count lines for each file
	fileStats, err := countLines(filteredFiles)
	if err != nil {
		return fmt.Errorf("failed to count lines: %w", err)
	}

	// Handle JSON export
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, fileStats)
	}

	// Display results
	return displayResults(opts, fileStats)
}

func getTrackedFiles(ctx context.Context, gitClient *git.Client) ([]string, error) {
	cmd, err := gitClient.Command(ctx, "ls-files")
	if err != nil {
		return nil, err
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func filterFiles(files []string, opts *LinesOfCodeOptions) []string {
	var filtered []string

	for _, file := range files {
		// Check extension filter
		if opts.ExtensionFilter != "" {
			ext := strings.TrimPrefix(filepath.Ext(file), ".")
			if ext != opts.ExtensionFilter {
				continue
			}
		}

		// Check exclude patterns
		excluded := false
		for _, pattern := range opts.ExcludeFiles {
			if matched, _ := filepath.Match(pattern, file); matched {
				excluded = true
				break
			}
			if matched, _ := filepath.Match(pattern, filepath.Base(file)); matched {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Check include patterns (if any specified)
		if len(opts.IncludeFiles) > 0 {
			included := false
			for _, pattern := range opts.IncludeFiles {
				if matched, _ := filepath.Match(pattern, file); matched {
					included = true
					break
				}
				if matched, _ := filepath.Match(pattern, filepath.Base(file)); matched {
					included = true
					break
				}
			}
			if !included {
				continue
			}
		}

		filtered = append(filtered, file)
	}

	return filtered
}

func countLines(files []string) ([]FileStats, error) {
	var stats []FileStats
	commentPatterns := getCommentPatterns()

	for _, file := range files {
		fileStats, err := countLinesInFile(file, commentPatterns)
		if err != nil {
			// Skip files that can't be read (binary files, etc.)
			continue
		}
		stats = append(stats, fileStats)
	}

	return stats, nil
}

func countLinesInFile(filePath string, commentPatterns map[string]*regexp.Regexp) (FileStats, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return FileStats{}, err
	}
	defer file.Close()

	extension := strings.TrimPrefix(filepath.Ext(filePath), ".")
	commentPattern := commentPatterns[extension]

	scanner := bufio.NewScanner(file)
	totalLines := 0
	blankLines := 0
	commentLines := 0

	for scanner.Scan() {
		totalLines++
		line := strings.TrimSpace(scanner.Text())
		
		if line == "" {
			blankLines++
		} else if commentPattern != nil && commentPattern.MatchString(line) {
			commentLines++
		}
	}

	if err := scanner.Err(); err != nil {
		return FileStats{}, err
	}

	codeLines := totalLines - blankLines - commentLines

	return FileStats{
		Path:       filePath,
		Lines:      totalLines,
		CodeLines:  codeLines,
		BlankLines: blankLines,
		Extension:  extension,
	}, nil
}

func getCommentPatterns() map[string]*regexp.Regexp {
	patterns := map[string]string{
		"go":   `^\s*//`,
		"js":   `^\s*(//|/\*|\*)`,
		"ts":   `^\s*(//|/\*|\*)`,
		"jsx":  `^\s*(//|/\*|\*)`,
		"tsx":  `^\s*(//|/\*|\*)`,
		"c":    `^\s*(//|/\*|\*)`,
		"cpp":  `^\s*(//|/\*|\*)`,
		"java": `^\s*(//|/\*|\*)`,
		"py":   `^\s*#`,
		"rb":   `^\s*#`,
		"sh":   `^\s*#`,
		"yaml": `^\s*#`,
		"yml":  `^\s*#`,
		"sql":  `^\s*--`,
		"html": `^\s*<!--`,
		"xml":  `^\s*<!--`,
		"css":  `^\s*/\*`,
	}

	compiled := make(map[string]*regexp.Regexp)
	for ext, pattern := range patterns {
		compiled[ext] = regexp.MustCompile(pattern)
	}
	return compiled
}

func displayResults(opts *LinesOfCodeOptions, fileStats []FileStats) error {
	cs := opts.IO.ColorScheme()

	if opts.ShowDetailed {
		return displayDetailedResults(opts, fileStats, cs)
	}

	if opts.ShowByFileType {
		return displayByTypeResults(opts, fileStats, cs)
	}

	return displaySummaryResults(opts, fileStats, cs)
}

func displayDetailedResults(opts *LinesOfCodeOptions, fileStats []FileStats, cs *iostreams.ColorScheme) error {
	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
		cs.Bold("File"),
		cs.Bold("Total"),
		cs.Bold("Code"),
		cs.Bold("Blank"),
		cs.Bold("Type"))

	// Sort by total lines descending
	sort.Slice(fileStats, func(i, j int) bool {
		return fileStats[i].Lines > fileStats[j].Lines
	})

	for _, stat := range fileStats {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n",
			stat.Path,
			stat.Lines,
			stat.CodeLines,
			stat.BlankLines,
			stat.Extension)
	}

	return displaySummary(opts, fileStats, cs, w)
}

func displayByTypeResults(opts *LinesOfCodeOptions, fileStats []FileStats, cs *iostreams.ColorScheme) error {
	typeStatsMap := make(map[string]*TypeStats)

	// Aggregate by file type
	for _, stat := range fileStats {
		ext := stat.Extension
		if ext == "" {
			ext = "no extension"
		}

		if typeStats, exists := typeStatsMap[ext]; exists {
			typeStats.FileCount++
			typeStats.TotalLines += stat.Lines
			typeStats.CodeLines += stat.CodeLines
			typeStats.BlankLines += stat.BlankLines
		} else {
			typeStatsMap[ext] = &TypeStats{
				Extension:  ext,
				FileCount:  1,
				TotalLines: stat.Lines,
				CodeLines:  stat.CodeLines,
				BlankLines: stat.BlankLines,
			}
		}
	}

	// Convert to slice and sort
	var typeStats []*TypeStats
	for _, stats := range typeStatsMap {
		typeStats = append(typeStats, stats)
	}
	sort.Slice(typeStats, func(i, j int) bool {
		return typeStats[i].TotalLines > typeStats[j].TotalLines
	})

	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
		cs.Bold("Extension"),
		cs.Bold("Files"),
		cs.Bold("Total"),
		cs.Bold("Code"),
		cs.Bold("Blank"))

	for _, stat := range typeStats {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\n",
			stat.Extension,
			stat.FileCount,
			stat.TotalLines,
			stat.CodeLines,
			stat.BlankLines)
	}

	return displaySummary(opts, fileStats, cs, w)
}

func displaySummaryResults(opts *LinesOfCodeOptions, fileStats []FileStats, cs *iostreams.ColorScheme) error {
	w := tabwriter.NewWriter(opts.IO.Out, 0, 0, 2, ' ', 0)
	defer w.Flush()

	return displaySummary(opts, fileStats, cs, w)
}

func displaySummary(opts *LinesOfCodeOptions, fileStats []FileStats, cs *iostreams.ColorScheme, w *tabwriter.Writer) error {
	totalFiles := len(fileStats)
	totalLines := 0
	totalCodeLines := 0
	totalBlankLines := 0

	for _, stat := range fileStats {
		totalLines += stat.Lines
		totalCodeLines += stat.CodeLines
		totalBlankLines += stat.BlankLines
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold("Summary:"))
	fmt.Fprintf(w, "%s:\t%s\n", "Files", cs.Green(strconv.Itoa(totalFiles)))
	fmt.Fprintf(w, "%s:\t%s\n", "Total lines", cs.Green(strconv.Itoa(totalLines)))
	fmt.Fprintf(w, "%s:\t%s\n", "Code lines", cs.Green(strconv.Itoa(totalCodeLines)))
	fmt.Fprintf(w, "%s:\t%s\n", "Blank lines", cs.Green(strconv.Itoa(totalBlankLines)))
	fmt.Fprintf(w, "%s:\t%s\n", "Comment lines", cs.Green(strconv.Itoa(totalLines-totalCodeLines-totalBlankLines)))

	return nil
}
