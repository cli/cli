package command

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	sterm "github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"

	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/cli/cli/utils"
)

func init() {
	prCmd.AddCommand(prReviewCmd)

	prReviewCmd.Flags().BoolP("approve", "a", false, "Approve pull request")
	prReviewCmd.Flags().BoolP("request-changes", "r", false, "Request changes on a pull request")
	prReviewCmd.Flags().BoolP("comment", "c", false, "Comment on a pull request")
	prReviewCmd.Flags().StringP("body", "b", "", "Specify the body of a review")

	prReviewCmd.Flags().BoolP("patch", "p", false, "Review interactively in patch mode")
}

var prReviewCmd = &cobra.Command{
	Use:   "review [{<number> | <url> | <branch>]",
	Short: "Add a review to a pull request.",
	Args:  cobra.MaximumNArgs(1),
	Long: `Add a review to either a specified pull request or the pull request associated with the current branch.

Examples:

	gh pr review                                  # add a review for the current branch's pull request
	gh pr review 123                              # add a review for pull request 123
	gh pr review -a                               # mark the current branch's pull request as approved
	gh pr review -c -b "interesting"              # comment on the current branch's pull request
	gh pr review 123 -r -b "needs more ascii art" # request changes on pull request 123
	`,
	RunE: prReview,
}

func processReviewOpt(cmd *cobra.Command) (*api.PullRequestReviewInput, error) {
	found := 0
	flag := ""
	var state api.PullRequestReviewState

	if cmd.Flags().Changed("approve") {
		found++
		flag = "approve"
		state = api.ReviewApprove
	}
	if cmd.Flags().Changed("request-changes") {
		found++
		flag = "request-changes"
		state = api.ReviewRequestChanges
	}
	if cmd.Flags().Changed("comment") {
		found++
		flag = "comment"
		state = api.ReviewComment
	}

	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return nil, err
	}

	if found == 0 && body == "" {
		return nil, nil // signal interactive mode
	} else if found == 0 && body != "" {
		return nil, errors.New("--body unsupported without --approve, --request-changes, or --comment")
	} else if found > 1 {
		return nil, errors.New("need exactly one of --approve, --request-changes, or --comment")
	}

	if (flag == "request-changes" || flag == "comment") && body == "" {
		return nil, fmt.Errorf("body cannot be blank for %s review", flag)
	}

	return &api.PullRequestReviewInput{
		Body:  body,
		State: state,
	}, nil
}

func prReview(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	var prNum int
	branchWithOwner := ""

	if len(args) == 0 {
		prNum, branchWithOwner, err = prSelectorForCurrentBranch(ctx, baseRepo)
		if err != nil {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}
	} else {
		prArg, repo := prFromURL(args[0])
		if repo != nil {
			baseRepo = repo
		} else {
			prArg = strings.TrimPrefix(args[0], "#")
		}
		prNum, err = strconv.Atoi(prArg)
		if err != nil {
			return errors.New("could not parse pull request argument")
		}
	}

	reviewData, err := processReviewOpt(cmd)
	if err != nil {
		return fmt.Errorf("did not understand desired review action: %w", err)
	}

	var pr *api.PullRequest
	if prNum > 0 {
		pr, err = api.PullRequestByNumber(apiClient, baseRepo, prNum)
		if err != nil {
			return fmt.Errorf("could not find pull request: %w", err)
		}
	} else {
		pr, err = api.PullRequestForBranch(apiClient, baseRepo, "", branchWithOwner)
		if err != nil {
			return fmt.Errorf("could not find pull request: %w", err)
		}
		prNum = pr.Number
	}

	out := colorableOut(cmd)
	patchMode, err := cmd.Flags().GetBool("patch")
	if err != nil {
		return err
	}

	if patchMode {
		reviewData, err = patchReview(cmd)
		if err != nil {
			return err
		}
		// for now just return
		return nil
	}

	if reviewData == nil {
		reviewData, err = reviewSurvey(cmd)

		if err != nil {
			return err
		}
		if reviewData == nil && err == nil {
			fmt.Fprint(out, "Discarding.\n")
			return nil
		}
	}

	err = api.AddReview(apiClient, pr, reviewData)
	if err != nil {
		return fmt.Errorf("failed to create review: %w", err)
	}

	switch reviewData.State {
	case api.ReviewComment:
		fmt.Fprintf(out, "%s Reviewed pull request #%d\n", utils.Gray("-"), prNum)
	case api.ReviewApprove:
		fmt.Fprintf(out, "%s Approved pull request #%d\n", utils.Green("✓"), prNum)
	case api.ReviewRequestChanges:
		fmt.Fprintf(out, "%s Requested changes to pull request #%d\n", utils.Red("+"), prNum)
	}

	return nil
}

func reviewSurvey(cmd *cobra.Command) (*api.PullRequestReviewInput, error) {
	editorCommand, err := determineEditor(cmd)
	if err != nil {
		return nil, err
	}

	typeAnswers := struct {
		ReviewType string
	}{}
	typeQs := []*survey.Question{
		{
			Name: "reviewType",
			Prompt: &survey.Select{
				Message: "What kind of review do you want to give?",
				Options: []string{
					"Comment",
					"Approve",
					"Request changes",
				},
			},
		},
	}

	err = SurveyAsk(typeQs, &typeAnswers)
	if err != nil {
		return nil, err
	}

	var reviewState api.PullRequestReviewState

	switch typeAnswers.ReviewType {
	case "Approve":
		reviewState = api.ReviewApprove
	case "Request changes":
		reviewState = api.ReviewRequestChanges
	case "Comment":
		reviewState = api.ReviewComment
	default:
		panic("unreachable state")
	}

	bodyAnswers := struct {
		Body string
	}{}

	blankAllowed := false
	if reviewState == api.ReviewApprove {
		blankAllowed = true
	}

	bodyQs := []*survey.Question{
		&survey.Question{
			Name: "body",
			Prompt: &surveyext.GhEditor{
				BlankAllowed:  blankAllowed,
				EditorCommand: editorCommand,
				Editor: &survey.Editor{
					Message:  "Review body",
					FileName: "*.md",
				},
			},
		},
	}

	err = SurveyAsk(bodyQs, &bodyAnswers)
	if err != nil {
		return nil, err
	}

	if bodyAnswers.Body == "" && (reviewState == api.ReviewComment || reviewState == api.ReviewRequestChanges) {
		return nil, errors.New("this type of review cannot be blank")
	}

	if len(bodyAnswers.Body) > 0 {
		out := colorableOut(cmd)
		renderedBody, err := utils.RenderMarkdown(bodyAnswers.Body)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(out, "Got:\n%s", renderedBody)
	}

	confirm := false
	confirmQs := []*survey.Question{
		{
			Name: "confirm",
			Prompt: &survey.Confirm{
				Message: "Submit?",
				Default: true,
			},
		},
	}

	err = SurveyAsk(confirmQs, &confirm)
	if err != nil {
		return nil, err
	}

	if !confirm {
		return nil, nil
	}

	return &api.PullRequestReviewInput{
		Body:  bodyAnswers.Body,
		State: reviewState,
	}, nil
}

func patchReview(cmd *cobra.Command) (*api.PullRequestReviewInput, error) {
	hunks := []*Hunk{
		{"diff --git a/command/pr_review.go b/command/pr_review.go",
			`@@ -11,7 +11,8 @@ import (
 )
 
 func init() {
-	prCmd.AddCommand(prReviewCmd)
+	// TODO re-register post release
+	// prCmd.AddCommand(prReviewCmd)
 
 	prReviewCmd.Flags().BoolP("approve", "a", false, "Approve pull request")
 	prReviewCmd.Flags().BoolP("request-changes", "r", false, "Request changes on a pull request")
		`, ""},
		{"diff --git a/command/pr_review_test.go b/command/pr_review_test.go",
			`@@ -8,6 +8,7 @@ import (
 )
 
 func TestPRReview_validation(t *testing.T) {
+	t.Skip("skipping until release is done")
 	initBlankContext("", "OWNER/REPO", "master")
 	http := initFakeHTTP()
 	for _, cmd := range []string{
		`, ""},
		{"diff --git a/command/pr_review_test.go b/command/pr_review_test.go",
			`@@ -22,6 +23,7 @@ func TestPRReview_validation(t *testing.T) {
 }
 
 func TestPRReview_url_arg(t *testing.T) {
+	t.Skip("skipping until release is done")
 	initBlankContext("", "OWNER/REPO", "master")
 	http := initFakeHTTP()
 	http.StubRepoResponse("OWNER", "REPO")
		`, ""},
		{"diff --git a/command/pr_review_test.go b/command/pr_review_test.go",
			`@@ -67,6 +69,7 @@ func TestPRReview_url_arg(t *testing.T) {
 }
 
 func TestPRReview_number_arg(t *testing.T) {
+	t.Skip("skipping until release is done")
 	initBlankContext("", "OWNER/REPO", "master")
 	http := initFakeHTTP()
 	http.StubRepoResponse("OWNER", "REPO")
		`, ""},
		{"diff --git a/command/pr_review_test.go b/command/pr_review_test.go",
			`@@ -112,6 +115,7 @@ func TestPRReview_number_arg(t *testing.T) {
 }
 
 func TestPRReview_no_arg(t *testing.T) {
+	t.Skip("skipping until release is done")
 	initBlankContext("", "OWNER/REPO", "feature")
 	http := initFakeHTTP()
 	http.StubRepoResponse("OWNER", "REPO")
		`, ""},
		{"diff --git a/command/pr_review_test.go b/command/pr_review_test.go",
			`@@ -147,6 +151,7 @@ func TestPRReview_no_arg(t *testing.T) {
 }
 
 func TestPRReview_blank_comment(t *testing.T) {
+	t.Skip("skipping until release is done")
 	initBlankContext("", "OWNER/REPO", "master")
 	http := initFakeHTTP()
 	http.StubRepoResponse("OWNER", "REPO")`,
			""},

		{"diff --git a/command/pr_review_test.go b/command/pr_review_test.go",
			`@@ -156,6 +161,7 @@ func TestPRReview_blank_comment(t *testing.T) {
 }
 
 func TestPRReview_blank_request_changes(t *testing.T) {
+	t.Skip("skipping until release is done")
 	initBlankContext("", "OWNER/REPO", "master")
 	http := initFakeHTTP()
 	http.StubRepoResponse("OWNER", "REPO")
 }

		`, ""},

		{"diff --git a/command/pr_review_test.go b/command/pr_review_test.go",
			`@@ -165,6 +171,7 @@ func TestPRReview_blank_request_changes(t *testing.T) {
			func TestPRReview(t *testing.T) {
+	t.Skip("skipping until release is done")
 	type c struct {
 		Cmd           string
 		ExpectedEvent string
		`, ""},

		{"diff --git a/command/root.go b/command/root.go",
			`@@ -271,7 +271,7 @@ func rootHelpFunc(command *cobra.Command, s []string) {
 		s := "  " + rpad(c.Name()+":", c.NamePadding()) + c.Short
 		if includes(coreCommandNames, c.Name()) {
 			coreCommands = append(coreCommands, s)
-		} else {
+		} else if c != creditsCmd {
 			additionalCommands = append(additionalCommands, s)
 		}
 	}`, ""},
	}
	out := colorableOut(cmd)

	fmt.Fprintln(out, "- starting review ~/.config/gh/reviews/0001.json")
	fmt.Fprintf(out, "You are going to review %s across %s.\n",
		utils.Bold(utils.Pluralize(9, "change")),
		utils.Bold(utils.Pluralize(3, "file")))

	cont := false
	continueQs := []*survey.Question{
		{
			Name: "continue",
			Prompt: &survey.Confirm{
				Message: "Continue?",
				Default: true,
			},
		},
	}

	err := SurveyAsk(continueQs, &cont)
	if err != nil {
		return nil, err
	}

	if !cont {
		return nil, nil
	}

	commentsMade := 0
	skipFile := false
	var skipping string
	for _, hunk := range hunks {
		if skipFile {
			if skipping == hunk.File {
				continue
			} else {
				skipFile = false
			}
		}
		fmt.Fprintf(out, "\n%s\n", utils.Bold(hunk.File))
		md := fmt.Sprintf("```diff\n%s\n```", hunk.Diff)
		rendered, err := utils.RenderMarkdown(md)
		if err != nil {
			return nil, err
		}

		fmt.Fprintln(out, rendered)

		fmt.Fprint(out, "s: skip, f: skip file, v: view diff, c: comment, q: quit ")

		action, err := patchSurvey()
		if err != nil {
			return nil, err
		}

		if action == HunkActionSkipFile {
			skipFile = true
			skipping = hunk.File
		}

		if action == HunkActionComment {
			editorCommand, err := determineEditor(cmd)
			if err != nil {
				return nil, err
			}
			bodyAnswers := struct {
				Body string
			}{}

			preseed := fmt.Sprintf(
				"\n\n======= everything below this line will be discarded =======\n%s\n%s",
				hunk.File,
				fmt.Sprintf("```\n%s\n```", hunk.Diff))

			bodyQs := []*survey.Question{
				&survey.Question{
					Name: "body",
					Prompt: &surveyext.GhEditor{
						BlankAllowed:  false,
						EditorCommand: editorCommand,
						Editor: &survey.Editor{
							Message:       "Comment",
							FileName:      "*.md",
							Default:       preseed,
							HideDefault:   true,
							AppendDefault: true,
						},
					},
				},
			}
			err = SurveyAsk(bodyQs, &bodyAnswers)
			if err != nil {
				return nil, err
			}
			body := trimBody(bodyAnswers.Body)
			if !isBlank(body) {
				hunk.Comment = body
				commentsMade++
			}
		}

		if action == HunkActionQuit {
			fmt.Fprintln(out, "Quitting.")
			return nil, nil
		}
	}

	fmt.Fprintf(out, "\n\nWrapping up a review with %s.\n",
		utils.Bold(utils.Pluralize(commentsMade, "comment")))

	reviewBody := `
	
======= everything below this line will be discarded =======
	
`

	for _, h := range hunks {
		if isBlank(h.Comment) {
			continue
		}
		reviewBody += "-----------------------------------\n"
		reviewBody += h.Comment
		reviewBody += "\n```\n"
		reviewBody += h.Diff
		reviewBody += "\n```\n"
	}

	reviewData, err := reviewSurveyPreBody(cmd, reviewBody)
	reviewData.Body = trimBody(reviewData.Body)

	if err != nil {
		return nil, err
	}
	if reviewData == nil && err == nil {
		fmt.Fprint(out, "Discarding.\n")
		return nil, nil
	}

	fmt.Fprintln(out, "would actually submit, here~")

	return nil, nil
}

type Hunk struct {
	File    string
	Diff    string
	Comment string
}

type HunkAction int

const (
	HunkActionSkip = iota
	HunkActionSkipFile
	HunkActionComment
	HunkActionQuit
)

func patchSurvey() (HunkAction, error) {
	std := sterm.Stdio{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
	rr := sterm.NewRuneReader(std)
	_ = rr.SetTermMode()
	defer func() { _ = rr.RestoreTermMode() }()

	//cursor := e.NewCursor()
	//cursor.Hide()
	//defer cursor.Show()

	for {
		r, _, err := rr.ReadRune()
		if err != nil {
			return -1, err
		}
		switch {
		case r == 's':
			return HunkActionSkip, nil
		case r == 'f':
			return HunkActionSkipFile, nil
		case r == 'c':
			return HunkActionComment, nil
		case r == 'q':
			return HunkActionQuit, nil
		case r == sterm.KeyInterrupt:
			return -1, sterm.InterruptErr
		}
	}

	return -1, nil
}

func trimBody(body string) string {
	r := regexp.MustCompile(`(?s)======= everything below.*$`)
	return r.ReplaceAllString(body, "")
}

func isBlank(body string) bool {
	r := regexp.MustCompile(`(?s)^\s*$`)
	return r.MatchString(body)
}

func reviewSurveyPreBody(cmd *cobra.Command, body string) (*api.PullRequestReviewInput, error) {
	editorCommand, err := determineEditor(cmd)
	if err != nil {
		return nil, err
	}

	typeAnswers := struct {
		ReviewType string
	}{}
	typeQs := []*survey.Question{
		{
			Name: "reviewType",
			Prompt: &survey.Select{
				Message: "What kind of review do you want to give?",
				Options: []string{
					"Comment",
					"Approve",
					"Request changes",
				},
			},
		},
	}

	err = SurveyAsk(typeQs, &typeAnswers)
	if err != nil {
		return nil, err
	}

	var reviewState api.PullRequestReviewState

	switch typeAnswers.ReviewType {
	case "Approve":
		reviewState = api.ReviewApprove
	case "Request changes":
		reviewState = api.ReviewRequestChanges
	case "Comment":
		reviewState = api.ReviewComment
	default:
		panic("unreachable state")
	}

	bodyAnswers := struct {
		Body string
	}{}

	blankAllowed := false
	if reviewState == api.ReviewApprove {
		blankAllowed = true
	}

	bodyQs := []*survey.Question{
		&survey.Question{
			Name: "body",
			Prompt: &surveyext.GhEditor{
				BlankAllowed:  blankAllowed,
				EditorCommand: editorCommand,
				Editor: &survey.Editor{
					Message:       "Review body",
					FileName:      "*.md",
					AppendDefault: true,
					Default:       body,
					HideDefault:   true,
				},
			},
		},
	}

	err = SurveyAsk(bodyQs, &bodyAnswers)
	if err != nil {
		return nil, err
	}

	if bodyAnswers.Body == "" && (reviewState == api.ReviewComment || reviewState == api.ReviewRequestChanges) {
		return nil, errors.New("this type of review cannot be blank")
	}

	confirm := false
	confirmQs := []*survey.Question{
		{
			Name: "confirm",
			Prompt: &survey.Confirm{
				Message: "Submit?",
				Default: true,
			},
		},
	}

	err = SurveyAsk(confirmQs, &confirm)
	if err != nil {
		return nil, err
	}

	if !confirm {
		return nil, nil
	}

	return &api.PullRequestReviewInput{
		Body:  bodyAnswers.Body,
		State: reviewState,
	}, nil
}
