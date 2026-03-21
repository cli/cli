package batchclone

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/repo/clone"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type BatchCloneOptions struct {
	IO       *iostreams.IOStreams
	Prompter prompter.Prompter

	FilePath           string
	TargetDir          string
	SkipExisting       bool
	ContinueOnError    bool
	NoConfirm          bool
	NoUpstream         bool
	UpstreamRemoteName string
	LogFile            string
	GitArgs            []string
}

type RepoEntry struct {
	RawInput    string
	Repository  string
	Destination string
}
