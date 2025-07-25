/*
Tag Release creates a new Boulder release tag and pushes it to GitHub. It
ensures that the release tag points to the correct commit, has standardized
formatting of both the tag itself and its message, and is GPG-signed.

It always produces Semantic Versioning tags of the form v0.YYYYMMDD.N, where:
  - the major version of 0 indicates that we are not committing to any
    backwards-compatibility guarantees;
  - the minor version of the current date provides a human-readable date for the
    release, and ensures that minor versions will be monotonically increasing;
    and
  - the patch version is always 0 for mainline releases, and a monotonically
    increasing number for hotfix releases.

Usage:

	go run github.com/letsencrypt/boulder/tools/release/tag@main [-push] [branchname]

If the "branchname" argument is not provided, it assumes "main". If it is
provided, it must be either "main" or a properly-formatted release branch name.

If the -push flag is not provided, it will simply print the details of the new
tag and then exit. If it is provided, it will initiate a push to the remote.

In all cases, it assumes that the upstream remote is named "origin".
*/
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type cmdError struct {
	error
	output string
}

func (e cmdError) Unwrap() error {
	return e.error
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	fmt.Println("Running:", cmd.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), cmdError{
			error:  fmt.Errorf("running %q: %w", cmd.String(), err),
			output: string(out),
		}
	}
	return string(out), nil
}

func show(output string) {
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		fmt.Println("  ", line)
	}
}

func main() {
	err := tag(os.Args[1:])
	if err != nil {
		var cmdErr cmdError
		if errors.As(err, &cmdErr) {
			show(cmdErr.output)
		}
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func tag(args []string) error {
	fs := flag.NewFlagSet("tag", flag.ContinueOnError)
	var push bool
	fs.BoolVar(&push, "push", false, "If set, push the resulting release tag to GitHub.")
	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("invalid flags: %w", err)
	}

	if len(fs.Args()) > 1 {
		return fmt.Errorf("too many args: %#v", fs.Args())
	}

	branch := "main"
	if len(fs.Args()) == 1 {
		branch = fs.Arg(0)
	}

	switch {
	case branch == "main":
		break
	case strings.HasPrefix(branch, "release-branch-"):
		return fmt.Errorf("sorry, tagging hotfix release branches is not yet supported")
	default:
		return fmt.Errorf("branch must be 'main' or 'release-branch-...', got %q", branch)
	}

	// Fetch all of the latest commits on this ref from origin, so that we can
	// ensure we're tagging the tip of the upstream branch.
	_, err = git("fetch", "origin", branch)
	if err != nil {
		return err
	}

	// We use semver's vMajor.Minor.Patch format, where the Major version is
	// always 0 (no backwards compatibility guarantees), the Minor version is
	// the date of the release, and the Patch number is zero for normal releases
	// and only non-zero for hotfix releases.
	minor := time.Now().Format("20060102")
	version := fmt.Sprintf("v0.%s.0", minor)
	message := fmt.Sprintf("Release %s", version)

	// Produce the tag, using -s to PGP sign it. This will fail if a tag with
	// that name already exists.
	_, err = git("tag", "-s", "-m", message, version, "origin/"+branch)
	if err != nil {
		return err
	}

	// Show the result of the tagging operation, including the tag message and
	// signature, and the commit hash and message, but not the diff.
	out, err := git("show", "-s", version)
	if err != nil {
		return err
	}
	show(out)

	if push {
		_, err = git("push", "origin", version)
		if err != nil {
			return err
		}
	} else {
		fmt.Println()
		fmt.Println("Please inspect the tag above, then run:")
		fmt.Printf("    git push origin %s\n", version)
	}
	return nil
}
