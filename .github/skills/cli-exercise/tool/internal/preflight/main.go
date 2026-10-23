package preflight

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
)

type cliOptions struct {
	output     string
	selections options
}

type formats []string

func (value *formats) String() string { return strings.Join(*value, ",") }
func (value *formats) Set(format string) error {
	if format != "gif" && format != "mp4" {
		return fmt.Errorf("format must be gif or mp4")
	}
	*value = append(*value, format)
	return nil
}

func parseOptions(args []string) (cliOptions, error) {
	var result cliOptions
	if len(args) == 0 || args[0] != "check" {
		return result, fmt.Errorf("preflight supports check only; setup is performed by the agent after caller approval")
	}
	flags := flag.NewFlagSet("preflight check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&result.output, "output", "", "Save the full report")
	flags.StringVar(&result.selections.ModuleRoot, "module-root", "", "Existing node_modules root")
	flags.StringVar(&result.selections.Node, "node", "", "Selected Node executable")
	flags.StringVar(&result.selections.FFmpeg, "ffmpeg", "", "Selected FFmpeg executable")
	flags.StringVar(&result.selections.FFprobe, "ffprobe", "", "Selected ffprobe executable")
	flags.Float64Var(&result.selections.ProbeTimeout, "probe-timeout", 20, "Per-process timeout in seconds")
	var formats formats
	flags.Var(&formats, "format", "Requested gif or mp4 output; repeat as needed")
	if err := flags.Parse(args[1:]); err != nil {
		return result, err
	}
	var emptySelection string
	flags.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "module-root", "node", "ffmpeg", "ffprobe":
			if strings.TrimSpace(option.Value.String()) == "" {
				emptySelection = option.Name
			}
		}
	})
	if emptySelection != "" {
		return result, fmt.Errorf("--%s requires a nonempty selection", emptySelection)
	}
	result.selections.Formats = formats
	if len(formats) == 0 {
		result.selections.Formats = []string{"gif", "mp4"}
	}
	if result.output == "" || len(flags.Args()) > 0 {
		return result, fmt.Errorf("check requires --output and no positional arguments")
	}
	if math.IsNaN(result.selections.ProbeTimeout) || math.IsInf(result.selections.ProbeTimeout, 0) {
		return result, fmt.Errorf("prerequisite timeout must be finite")
	}
	return result, nil
}

func emit(value report, output string, streams cliutil.Streams) (int, error) {
	status := value.Status
	code := 2
	if status == "ready" {
		code = 0
	} else if status == "needs-install" {
		code = 1
	}
	if output == "" {
		return code, cliutil.JSON(streams.Out, value)
	}
	if info, err := os.Lstat(output); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return 2, fmt.Errorf("report output must not replace a symbolic link")
	} else if err != nil && !os.IsNotExist(err) {
		return 2, err
	}
	if err := cliutil.WriteJSON(output, value); err != nil {
		return 2, err
	}
	path, err := filepath.Abs(output)
	if err != nil {
		return 2, err
	}
	result := map[string]any{"status": status, "reportPath": path}
	switch status {
	case "ready":
		result["next"] = "Use the saved receipt with the Go run helper."
	case "needs-install":
		result["next"] = "Propose setup commands using the pinned requirements, ask for approval, then rerun check."
	default:
		result["next"] = "Resolve the listed blockers; full diagnostics are in the saved report."
	}
	if len(value.Issues) > 0 {
		result["issues"] = value.Issues
	}
	return code, cliutil.JSON(streams.Out, result)
}

func run(ctx context.Context, root string, options cliOptions, streams cliutil.Streams, factory func(string, options) (*service, error)) (int, error) {
	service, err := factory(root, options.selections)
	if err != nil {
		return emit(blocked(err.Error()), options.output, streams)
	}
	value, err := service.check(ctx)
	if err != nil {
		value = blocked(err.Error())
	}
	return emit(value, options.output, streams)
}

func blocked(message string) report {
	return report{SchemaVersion: 1, Status: "blocked", Issues: []issue{{Item: "preflight", Detail: message}}}
}

// Main checks selected prerequisites without installing or modifying dependencies.
func Main(ctx context.Context, skillRoot string, args []string, streams cliutil.Streams) int {
	options, err := parseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		_, err = fmt.Fprintln(streams.Out, "Usage: cli-exercise --skill-root PATH preflight check --output FILE [options]")
		if err == nil {
			return 0
		}
	}
	code := 2
	if err == nil {
		code, err = run(ctx, skillRoot, options, streams, newService)
	}
	if err != nil {
		if writeErr := cliutil.Error(streams.ErrOut, "preflight_blocked", err.Error()); writeErr != nil {
			return 2
		}
	}
	return code
}
