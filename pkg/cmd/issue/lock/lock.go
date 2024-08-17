// Package lock locks and unlocks conversations on both GitHub issues and pull
// requests.
//
// Every pull request is an issue, but not every issue is a pull request.
// Therefore, this package is used in `cmd/pr` as well.
//
// A note on nomenclature for "comments", "conversations", and "discussions":
// The GitHub documentation refers to a set of comments on an issue or pull
// request as a conversation.  A GitHub discussion refers to the "message board"
// for a project where announcements, questions, and answers can be posted.
package lock

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type iprompter interface {
	Confirm(string, bool) (bool, error)
	Select(string, string, []string) (int, error)
}

// reasons contains all possible lock reasons allowed by GitHub.
//
// We don't directly construct a map so that we can maintain the reasons in
// alphabetical order.
var reasons = []string{"off_topic", "resolved", "spam", "too_heated"}

var reasonsString = strings.Join(reasons, ", ")

var reasonsApi = []githubv4.LockReason{
	githubv4.LockReasonOffTopic,
	githubv4.LockReasonResolved,
	githubv4.LockReasonSpam,
	githubv4.LockReasonTooHeated,
}

// If no reason is given (an empty string), reasonsMap will return the nil
// value, since it is not contained in the map.  This, in turn, sets lock_reason
// to null in GraphQL.
var reasonsMap map[string]*githubv4.LockReason

func init() {
	reasonsMap = make(map[string]*githubv4.LockReason)
	for i, reason := range reasons {
		reasonsMap[reason] = &reasonsApi[i]
	}
}

type command struct {
	Name     string // actual command name
	FullName string // complete name for the command
	Typename string // return value from issue.Typename
}

// The `FullName` should be capitalized as if starting a sentence since it is
// used in print and error statements.  It's easier to manually capitalize and
// call `ToLower`, when needed, than the other way around.
var aliasIssue = command{"issue", "Issue", api.TypeIssue}
var aliasPr = command{"pr", "Pull request", api.TypePullRequest}

var alias map[string]*command = map[string]*command{
	"issue":             &aliasIssue,
	"pr":                &aliasPr,
	api.TypeIssue:       &aliasIssue,
	api.TypePullRequest: &aliasPr,
}

// Acceptable lock states for conversations.  These are used in print
// statements, hence the use of strings instead of booleans.
const (
	Lock   = "Lock"
	Unlock = "Unlock"
)

func fields() []string {
	return []string{
		"activeLockReason", "id", "locked", "number", "title", "url",
	}
}

type LockOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

	ParentCmd   string
	Reason      string
	SelectorArg string
	Interactive bool
}

func (opts *LockOptions) setCommonOptions(f *cmdutil.Factory, args []string) {
	opts.IO = f.IOStreams
	opts.HttpClient = f.HttpClient
	opts.Config = f.Config

	// support `-R, --repo` override
	opts.BaseRepo = f.BaseRepo

	opts.SelectorArg = args[0]

}

func NewCmdLock(f *cmdutil.Factory, parentName string, runF func(string, *LockOptions) error) *cobra.Command {
	opts := &LockOptions{
		ParentCmd: parentName,
		Prompter:  f.Prompter,
	}

	c := alias[opts.ParentCmd]
	short := fmt.Sprintf("Lock %s conversation", strings.ToLower(c.FullName))

	cmd := &cobra.Command{
		Use:   "lock {<number> | <url>}",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.setCommonOptions(f, args)

			reasonProvided := cmd.Flags().Changed("reason")
			if reasonProvided {
				_, ok := reasonsMap[opts.Reason]
				if !ok {
					if opts.IO.IsStdoutTTY() {
						cs := opts.IO.ColorScheme()

						return cmdutil.FlagErrorf("%s Invalid reason: %v\n",
							cs.FailureIconWithColor(cs.Red), opts.Reason)
					} else {
						return fmt.Errorf("invalid reason %s", opts.Reason)
					}
				}
			} else if opts.IO.CanPrompt() {
				opts.Interactive = true
			}

			if runF != nil {
				return runF(Lock, opts)
			}
			return lockRun(Lock, opts)
		},
	}

	msg := fmt.Sprintf("Optional reason for locking conversation (%v).", reasonsString)

	cmd.Flags().StringVarP(&opts.Reason, "reason", "r", "", msg)
	return cmd
}

func NewCmdUnlock(f *cmdutil.Factory, parentName string, runF func(string, *LockOptions) error) *cobra.Command {
	opts := &LockOptions{ParentCmd: parentName}

	c := alias[opts.ParentCmd]
	short := fmt.Sprintf("Unlock %s conversation", strings.ToLower(c.FullName))

	cmd := &cobra.Command{
		Use:   "unlock {<number> | <url>}",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.setCommonOptions(f, args)

			if runF != nil {
				return runF(Unlock, opts)
			}
			return lockRun(Unlock, opts)
		},
	}

	return cmd
}

// reason creates a sentence fragment so that the lock reason can be used in a
// sentence.
//
// e.g. "resolved" -> " as RESOLVED"
func reason(reason string) string {
	result := ""
	if reason != "" {
		result = fmt.Sprintf(" as %s", strings.ToUpper(reason))
	}
	return result
}

// status creates a string showing the result of a successful lock/unlock that
// is parameterized on a bunch of options.
//
// Example output: "Locked as RESOLVED: Issue #31 (Title of issue)"
func status(state string, lockable *api.Issue, baseRepo ghrepo.Interface, opts *LockOptions) string {
	return fmt.Sprintf("%sed%s: %s %s#%d (%s)",
		state, reason(opts.Reason), alias[opts.ParentCmd].FullName, ghrepo.FullName(baseRepo), lockable.Number, lockable.Title)
}

// lockRun will lock or unlock a conversation.
func lockRun(state string, opts *LockOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	issuePr, baseRepo, err := issueShared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.SelectorArg, fields())

	parent := alias[opts.ParentCmd]

	if err != nil {
		return err
	} else if parent.Typename != issuePr.Typename {
		currentType := alias[parent.Typename]
		correctType := alias[issuePr.Typename]

		return fmt.Errorf("%s %s %s#%d not found, but found %s %s#%d.  Use `gh %s %s %d` instead",
			cs.FailureIconWithColor(cs.Red),
			currentType.FullName, ghrepo.FullName(baseRepo), issuePr.Number,
			strings.ToLower(correctType.FullName), ghrepo.FullName(baseRepo), issuePr.Number,
			correctType.Name, strings.ToLower(state), issuePr.Number)
	}

	if opts.Interactive {
		options := []string{"None", "Off topic", "Resolved", "Spam", "Too heated"}
		selected, err := opts.Prompter.Select("Lock reason?", "", options)
		if err != nil {
			return err
		}
		if selected > 0 {
			opts.Reason = reasons[selected-1]
		}
	}

	successMsg := fmt.Sprintf("%s %s\n",
		cs.SuccessIconWithColor(cs.Green), status(state, issuePr, baseRepo, opts))

	switch state {
	case Lock:
		if !issuePr.Locked {
			err = lockLockable(httpClient, baseRepo, issuePr, opts)
		} else {
			var relocked bool
			relocked, err = relockLockable(httpClient, baseRepo, issuePr, opts)

			if !relocked {
				successMsg = fmt.Sprintf("%s %s#%d already locked%s.  Nothing changed.\n",
					parent.FullName, ghrepo.FullName(baseRepo), issuePr.Number, reason(issuePr.ActiveLockReason))
			}
		}

	case Unlock:
		if issuePr.Locked {
			err = unlockLockable(httpClient, baseRepo, issuePr)
		} else {
			successMsg = fmt.Sprintf("%s %s#%d already unlocked.  Nothing changed.\n",
				parent.FullName, ghrepo.FullName(baseRepo), issuePr.Number)
		}
	default:
		panic("bad state")
	}

	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprint(opts.IO.Out, successMsg)
	}

	return nil
}

// lockLockable will lock an issue or pull request
func lockLockable(httpClient *http.Client, repo ghrepo.Interface, lockable *api.Issue, opts *LockOptions) error {
	var mutation struct {
		LockLockable struct {
			LockedRecord struct {
				Locked bool
			}
		} `graphql:"lockLockable(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.LockLockableInput{
			LockableID: lockable.ID,
			LockReason: reasonsMap[opts.Reason],
		},
	}

	gql := api.NewClientFromHTTP(httpClient)
	return gql.Mutate(repo.RepoHost(), "LockLockable", &mutation, variables)
}

// unlockLockable will unlock an issue or pull request
func unlockLockable(httpClient *http.Client, repo ghrepo.Interface, lockable *api.Issue) error {

	var mutation struct {
		UnlockLockable struct {
			UnlockedRecord struct {
				Locked bool
			}
		} `graphql:"unlockLockable(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.UnlockLockableInput{
			LockableID: lockable.ID,
		},
	}

	gql := api.NewClientFromHTTP(httpClient)
	return gql.Mutate(repo.RepoHost(), "UnlockLockable", &mutation, variables)
}

// relockLockable will unlock then lock an issue or pull request.  A common use
// case would be to change the reason for locking.
//
// The current api doesn't allow you to send a single lock request to update a
// lockable item that is already locked; it will just ignore that request.  You
// need to first unlock then lock with a new reason.
func relockLockable(httpClient *http.Client, repo ghrepo.Interface, lockable *api.Issue, opts *LockOptions) (bool, error) {
	if !opts.IO.CanPrompt() {
		return false, errors.New("already locked")
	}

	prompt := fmt.Sprintf("%s %s#%d already locked%s. Unlock and lock again%s?",
		alias[opts.ParentCmd].FullName, ghrepo.FullName(repo), lockable.Number, reason(lockable.ActiveLockReason), reason(opts.Reason))

	relocked, err := opts.Prompter.Confirm(prompt, true)
	if err != nil {
		return false, err
	} else if !relocked {
		return relocked, nil
	}

	err = unlockLockable(httpClient, repo, lockable)
	if err != nil {
		return relocked, err
	}

	err = lockLockable(httpClient, repo, lockable, opts)
	if err != nil {
		return relocked, err
	}

	return relocked, nil
}
