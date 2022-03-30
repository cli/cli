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
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type state uint

// Acceptable lock states for conversations
const (
	Unlock state = iota
	Lock
)

var reasons = []string{"off-topic", "resolved", "spam", "too-heated"}
var reasonsString = strings.Join(reasons, ", ")
var reasonsJson = []string{"off-topic", "resolved", "spam", "too heated"}
var reasonsMap map[string]string

func init() {
	reasonsMap = make(map[string]string)
	for i, reason := range reasons {
		reasonsMap[reason] = reasonsJson[i]
	}
}

type LockOptions struct {
	HttpClient   func() (*http.Client, error)
	Config       func() (config.Config, error)
	IO           *iostreams.IOStreams
	BaseRepo     func() (ghrepo.Interface, error)
	PadlockState state
	Reason       string
	ParentCmd    string
	IssueNumber  string
}

func (opts *LockOptions) setCommonOptions(f *cmdutil.Factory, cmd *cobra.Command, args []string) {

	opts.IO = f.IOStreams
	opts.HttpClient = f.HttpClient
	opts.Config = f.Config

	// support `-R, --repo` override
	opts.BaseRepo = f.BaseRepo

	// Set what type of conversation
	opts.ParentCmd = cmd.Parent().Name()

	opts.IssueNumber = args[0]
}

func NewCmdLock(f *cmdutil.Factory, runF func(*LockOptions) error) *cobra.Command {

	opts := &LockOptions{
		PadlockState: Lock,
	}

	cmd := &cobra.Command{
		Use:   "lock {<number> | <url>}",
		Short: "Lock a conversation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			reasonProvided := cmd.Flags().Changed("message")
			if reasonProvided {
				jsonReason, ok := reasonsMap[opts.Reason]
				if !ok {
					return cmdutil.FlagErrorf("Invalid reason: %v.\nMust be one of the following: %v.\nAborting lock.",
						opts.Reason, reasonsString)
				}

				opts.Reason = jsonReason
			}

			opts.setCommonOptions(f, cmd, args)

			if runF != nil {
				return runF(opts)
			}
			return padlock(opts)
		},
	}

	msg := fmt.Sprintf("Optional reason for locking conversation.  Must be one of the following: %v.", reasonsString)

	cmd.Flags().StringVarP(&opts.Reason, "message", "m", "", msg)
	return cmd
}

func NewCmdUnlock(f *cmdutil.Factory, runF func(*LockOptions) error) *cobra.Command {

	opts := &LockOptions{
		PadlockState: Unlock,
	}

	cmd := &cobra.Command{
		Use:   "unlock {<number> | <url>}",
		Short: "Unlock a conversation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			opts.setCommonOptions(f, cmd, args)

			if runF != nil {
				return runF(opts)
			}
			return padlock(opts)
		},
	}

	return cmd
}

// padlock will lock or unlock a conversation.
func padlock(opts *LockOptions) error {
	switch opts.PadlockState {
	case Lock:
		fmt.Printf("Locking %v #%v\n", opts.ParentCmd, opts.IssueNumber)
	case Unlock:
		fmt.Printf("Unlocking %v #%v\n", opts.ParentCmd, opts.IssueNumber)
	}

	return nil
}
