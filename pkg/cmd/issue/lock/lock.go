// Package lock locks and unlocks conversations on both GitHub issues and pull
// requests.
//
// Every pull request is an issue, but not every issue is a pull request.
// Therefore, we should be able to use this in the package cmd/pr as well.
//
// A note on nomenclature for "comments", "conversations", and "discussions":
// The GitHub documentation refers to a set of comments on an issue or pull
// request as a conversation.  A GitHub discussion refers to the "message board"
// for a project where announcements, questions, and answers can be posted.
package lock

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

// the reason for not just declaring a map is so that I can put the keys in
// alphabetical order
var reasons = []string{"off_topic", "resolved", "spam", "too_heated"}
var reasonsString = strings.Join(reasons, ", ")

var reasonsApi = []githubv4.LockReason{
	githubv4.LockReasonOffTopic,
	githubv4.LockReasonResolved,
	githubv4.LockReasonSpam,
	githubv4.LockReasonTooHeated,
}
var reasonsMap map[string]*githubv4.LockReason

func init() {
	reasonsMap = make(map[string]*githubv4.LockReason)
	for i, reason := range reasons {
		reasonsMap[reason] = &reasonsApi[i]
	}

	// If no reason given, set lock_reason to null in graphql
	reasonsMap[""] = nil
}

type command struct {
	Name     string // actual command name
	FullName string // complete name for the command
	Typename string // return value from issue.Typename
}

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
	Unlock string = "Unlock"
	Lock   string = "Lock"
)

type LockOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Fields      []string
	ParentCmd   string
	Reason      string
	SelectorArg string
}

func (opts *LockOptions) setCommonOptions(f *cmdutil.Factory, cmd *cobra.Command, args []string) {

	opts.IO = f.IOStreams
	opts.HttpClient = f.HttpClient
	opts.Config = f.Config

	// support `-R, --repo` override
	opts.BaseRepo = f.BaseRepo

	opts.SelectorArg = args[0]

	opts.Fields = []string{
		"id", "number", "title", "url", "locked", "activeLockReason"}

}

func NewCmdLock(f *cmdutil.Factory, parentName string) *cobra.Command {

	opts := &LockOptions{ParentCmd: parentName}

	c := alias[opts.ParentCmd]
	short := fmt.Sprintf("Lock %s conversation", strings.ToLower(c.FullName))

	cmd := &cobra.Command{
		Use:   "lock {<number> | <url>}",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			opts.setCommonOptions(f, cmd, args)

			reasonProvided := cmd.Flags().Changed("message")
			if reasonProvided {
				_, ok := reasonsMap[opts.Reason]
				if !ok {
					cs := opts.IO.ColorScheme()

					return cmdutil.FlagErrorf("%s Invalid reason: %v.\n Aborting lock.  See help for options.",
						cs.FailureIconWithColor(cs.Red), opts.Reason)
				}
			}

			return padlock(Lock, opts)
		},
	}

	msg := fmt.Sprintf("Optional reason for locking conversation.  Must be one of the following: %v.", reasonsString)

	cmd.Flags().StringVarP(&opts.Reason, "message", "m", "", msg)
	return cmd
}

func NewCmdUnlock(f *cmdutil.Factory, parentName string) *cobra.Command {

	opts := &LockOptions{ParentCmd: parentName}

	c := alias[opts.ParentCmd]
	short := fmt.Sprintf("Unlock %s conversation", strings.ToLower(c.FullName))

	cmd := &cobra.Command{
		Use:   "unlock {<number> | <url>}",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			opts.setCommonOptions(f, cmd, args)

			return padlock(Unlock, opts)
		},
	}

	return cmd
}

func reason(reason string) string {
	result := ""
	if reason != "" {
		result = fmt.Sprintf(" as %s", strings.ToUpper(reason))
	}
	return result
}

func status(state string, lockable *api.Issue, opts *LockOptions) string {

	return fmt.Sprintf("%sed%s: %s #%d (%s)",
		state, reason(opts.Reason), alias[opts.ParentCmd].FullName, lockable.Number, lockable.Title)
}

// padlock will lock or unlock a conversation.
func padlock(state string, opts *LockOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	issuePr, baseRepo, err := issueShared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.SelectorArg, opts.Fields)

	parent := alias[opts.ParentCmd]

	if err != nil {
		return err
	} else if parent.Typename != issuePr.Typename {
		currentType := alias[parent.Typename]
		correctType := alias[issuePr.Typename]

		return fmt.Errorf("%s #%d not found, but found %s #%d.  Use `gh %s %s %d` instead",
			currentType.FullName, issuePr.Number,
			correctType.FullName, issuePr.Number,
			correctType.Name, strings.ToLower(state), issuePr.Number)
	}

	successMsg := fmt.Sprintf("%s %s\n",
		cs.SuccessIconWithColor(cs.Green), status(state, issuePr, opts))

	switch {
	case state == Lock && !issuePr.Locked:
		err = lockLockable(httpClient, baseRepo, issuePr, opts)

	case state == Lock && issuePr.Locked:
		relocked, errRelock := relockLockable(httpClient, baseRepo, issuePr, opts)
		err = errRelock
		if !relocked {
			successMsg = fmt.Sprintf("%s #%d already locked%s.  Nothing changed.\n",
				parent.FullName, issuePr.Number, reason(issuePr.ActiveLockReason))
		}

	case state == Unlock && issuePr.Locked:
		err = unlockLockable(httpClient, baseRepo, issuePr, opts)

	case state == Unlock && !issuePr.Locked:
		successMsg = fmt.Sprintf("%s #%d already unlocked.  Nothing changed.\n",
			parent.FullName, issuePr.Number)
	}

	if err != nil {
		return err
	}

	fmt.Fprint(opts.IO.ErrOut, successMsg)

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

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)

	return gql.MutateNamed(context.Background(), "LockLockable", &mutation, variables)
}

// unlockLockable will lock an issue or pull request
func unlockLockable(httpClient *http.Client, repo ghrepo.Interface, lockable *api.Issue, opts *LockOptions) error {

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

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)

	return gql.MutateNamed(context.Background(), "UnlockLockable", &mutation, variables)
}

// relockLockable will unlock then lock an issue or pull request.  A common use
// case would be to change the reason for locking.
func relockLockable(httpClient *http.Client, repo ghrepo.Interface, lockable *api.Issue, opts *LockOptions) (bool, error) {

	var relocked bool
	shouldRelock := &survey.Confirm{
		Message: fmt.Sprintf("%s #%d locked%s.  Unlock and lock again%s?",
			alias[opts.ParentCmd].FullName, lockable.Number, reason(lockable.ActiveLockReason), reason(opts.Reason)),
		Default: true,
	}

	err := survey.AskOne(shouldRelock, &relocked)
	if err != nil {
		return relocked, err
	} else if !relocked {
		return relocked, nil
	}

	err = unlockLockable(httpClient, repo, lockable, opts)
	if err != nil {
		return relocked, err
	}

	err = lockLockable(httpClient, repo, lockable, opts)
	if err != nil {
		return relocked, err
	}

	return relocked, nil
}
