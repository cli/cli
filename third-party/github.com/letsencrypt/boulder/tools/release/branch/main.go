/*
Branch Release creates a new Boulder hotfix release branch and pushes it to
GitHub. It ensures that the release branch has a standard name, and starts at
a previously-tagged mainline release.

The expectation is that this branch will then be the target of one or more PRs
copying (cherry-picking) commits from main to the release branch, and then a
hotfix release will be tagged on the branch using the related Tag Release tool.

Usage:

	go run github.com/letsencrypt/boulder/tools/release/tag@main [-push] tagname

The provided tagname must be a pre-existing release tag which is reachable from
the "main" branch.

If the -push flag is not provided, it will simply print the details of the new
branch and then exit. If it is provided, it will initiate a push to the remote.

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
	err := branch(os.Args[1:])
	if err != nil {
		var cmdErr cmdError
		if errors.As(err, &cmdErr) {
			show(cmdErr.output)
		}
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func branch(args []string) error {
	fs := flag.NewFlagSet("branch", flag.ContinueOnError)
	var push bool
	fs.BoolVar(&push, "push", false, "If set, push the resulting hotfix release branch to GitHub.")
	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("invalid flags: %w", err)
	}

	if len(fs.Args()) != 1 {
		return fmt.Errorf("must supply exactly one argument, got %d: %#v", len(fs.Args()), fs.Args())
	}

	tag := fs.Arg(0)

	// Confirm the reasonableness of the given tag name by inspecting each of its
	// components.
	parts := strings.SplitN(tag, ".", 3)
	if len(parts) != 3 {
		return fmt.Errorf("failed to parse patch version from release tag %q", tag)
	}

	major := parts[0]
	if major != "v0" {
		return fmt.Errorf("expected major portion of release tag to be 'v0', got %q", major)
	}

	minor := parts[1]
	t, err := time.Parse("20060102", minor)
	if err != nil {
		return fmt.Errorf("expected minor portion of release tag to be a ")
	}
	if t.Year() < 2015 {
		return fmt.Errorf("minor portion of release tag appears to be an unrealistic date: %q", t.String())
	}

	patch := parts[2]
	if patch != "0" {
		return fmt.Errorf("expected patch portion of release tag to be '0', got %q", patch)
	}

	// Fetch all of the latest refs from origin, so that we can get the most
	// complete view of this tag and its relationship to main.
	_, err = git("fetch", "origin")
	if err != nil {
		return err
	}

	_, err = git("merge-base", "--is-ancestor", tag, "origin/main")
	if err != nil {
		return fmt.Errorf("tag %q is not reachable from origin/main, may not have been created properly: %w", tag, err)
	}

	// Create the branch. We could skip this and instead push the tag directly
	// to the desired ref name on the remote, but that wouldn't give the operator
	// a chance to inspect it locally.
	branch := fmt.Sprintf("release-branch-%s.%s", major, minor)
	_, err = git("branch", branch, tag)
	if err != nil {
		return err
	}

	// Show the HEAD of the new branch, not including its diff.
	out, err := git("show", "-s", branch)
	if err != nil {
		return err
	}
	show(out)

	refspec := fmt.Sprintf("%s:%s", branch, branch)

	if push {
		_, err = git("push", "origin", refspec)
		if err != nil {
			return err
		}
	} else {
		fmt.Println()
		fmt.Println("Please inspect the branch above, then run:")
		fmt.Printf("    git push origin %s\n", refspec)
	}
	return nil
}
