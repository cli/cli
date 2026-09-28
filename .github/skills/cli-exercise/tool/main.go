package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/evidence"
	"github.com/cli/cli/v2/cli-exercise/internal/execution"
	"github.com/cli/cli/v2/cli-exercise/internal/preflight"
)

type options struct {
	skillRoot string
	command   string
	args      []string
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("cli-exercise", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var opts options
	flags.StringVar(&opts.skillRoot, "skill-root", "", "Path to the installed cli-exercise skill")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	remaining := flags.Args()
	if len(remaining) == 0 {
		return opts, fmt.Errorf("choose preflight, run, client, or evidence")
	}
	opts.command, opts.args = remaining[0], remaining[1:]
	switch opts.command {
	case "preflight", "run", "evidence":
		if opts.skillRoot == "" {
			return opts, fmt.Errorf("--skill-root is required for %s", opts.command)
		}
	case "client", "probe-fixture":
	default:
		return opts, fmt.Errorf("unknown helper command %q", opts.command)
	}
	return opts, nil
}

func run(ctx context.Context, args []string, streams cliutil.Streams) int {
	opts, err := parseOptions(args)
	if err != nil {
		if err == flag.ErrHelp {
			if _, writeErr := fmt.Fprintln(streams.Out,
				"Usage: cli-exercise [--skill-root PATH] {preflight|run|client|evidence} [arguments]"); writeErr != nil {
				return 1
			}
			return 0
		}
		if writeErr := cliutil.Error(streams.ErrOut, "usage", err.Error()); writeErr != nil {
			return 1
		}
		return 2
	}
	switch opts.command {
	case "preflight":
		return preflight.Main(ctx, opts.skillRoot, opts.args, streams)
	case "run":
		return execution.RunMain(ctx, opts.skillRoot, opts.args, streams)
	case "client":
		return execution.ClientMain(ctx, opts.args, streams)
	case "probe-fixture":
		return preflight.ProbeMain(ctx, opts.args, streams)
	default:
		return evidence.Main(ctx, opts.skillRoot, opts.args, streams)
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], cliutil.Streams{
		In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr,
	})
	stop()
	os.Exit(code)
}
