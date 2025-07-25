// Package main provides the "admin" tool, which can perform various
// administrative actions (such as revoking certificates) against a Boulder
// deployment.
//
// Run "admin -h" for a list of flags and subcommands.
//
// Note that the admin tool runs in "dry-run" mode *by default*. All commands
// which mutate the database (either directly or via gRPC requests) will refuse
// to do so, and instead print log lines representing the work they would do,
// unless the "-dry-run=false" flag is passed.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/features"
)

type Config struct {
	Admin struct {
		// DB controls the admin tool's direct connection to the database.
		DB cmd.DBConfig
		// TLS controls the TLS client the admin tool uses for gRPC connections.
		TLS cmd.TLSConfig

		RAService *cmd.GRPCClientConfig
		SAService *cmd.GRPCClientConfig

		Features features.Config
	}

	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig
}

// subcommand specifies the set of methods that a struct must implement to be
// usable as an admin subcommand.
type subcommand interface {
	// Desc should return a short (one-sentence) description of the subcommand for
	// use in help/usage strings.
	Desc() string
	// Flags should register command line flags on the provided flagset. These
	// should use the "TypeVar" methods on the provided flagset, targeting fields
	// on the subcommand struct, so that the results of command line parsing can
	// be used by other methods on the struct.
	Flags(*flag.FlagSet)
	// Run should do all of the subcommand's heavy lifting, with behavior gated on
	// the subcommand struct's member fields which have been populated from the
	// command line. The provided admin object can be used for access to external
	// services like the RA, SA, and configured logger.
	Run(context.Context, *admin) error
}

// main is the entry-point for the admin tool. We do not include admin in the
// suite of tools which are subcommands of the "boulder" binary, since it
// should be small and portable and standalone.
func main() {
	// Do setup as similarly as possible to all other boulder services, including
	// config parsing and stats and logging setup. However, the one downside of
	// not being bundled with the boulder binary is that we don't get config
	// validation for free.
	defer cmd.AuditPanic()

	// This is the registry of all subcommands that the admin tool can run.
	subcommands := map[string]subcommand{
		"revoke-cert":      &subcommandRevokeCert{},
		"block-key":        &subcommandBlockKey{},
		"pause-identifier": &subcommandPauseIdentifier{},
		"unpause-account":  &subcommandUnpauseAccount{},
	}

	defaultUsage := flag.Usage
	flag.Usage = func() {
		defaultUsage()
		fmt.Printf("\nSubcommands:\n")
		for name, command := range subcommands {
			fmt.Printf("  %s\n", name)
			fmt.Printf("\t%s\n", command.Desc())
		}
		fmt.Print("\nYou can run \"admin <subcommand> -help\" to get usage for that subcommand.\n")
	}

	// Start by parsing just the global flags before we get to the subcommand, if
	// they're present.
	configFile := flag.String("config", "", "Path to the configuration file for this service (required)")
	dryRun := flag.Bool("dry-run", true, "Print actions instead of mutating the database")
	flag.Parse()

	// Figure out which subcommand they want us to run.
	unparsedArgs := flag.Args()
	if len(unparsedArgs) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	subcommand, ok := subcommands[unparsedArgs[0]]
	if !ok {
		flag.Usage()
		os.Exit(1)
	}

	// Then parse the rest of the args according to the selected subcommand's
	// flags, and allow the global flags to be placed after the subcommand name.
	subflags := flag.NewFlagSet(unparsedArgs[0], flag.ExitOnError)
	subcommand.Flags(subflags)
	flag.VisitAll(func(f *flag.Flag) {
		// For each flag registered at the global/package level, also register it on
		// the subflags FlagSet. The `f.Value` here is a pointer to the same var
		// that the original global flag would populate, so the same variable can
		// be set either way.
		subflags.Var(f.Value, f.Name, f.Usage)
	})
	_ = subflags.Parse(unparsedArgs[1:])

	// With the flags all parsed, now we can parse our config and set up our admin
	// object.
	if *configFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	a, err := newAdmin(*configFile, *dryRun)
	cmd.FailOnError(err, "creating admin object")

	// Finally, run the selected subcommand.
	if a.dryRun {
		a.log.AuditInfof("admin tool executing a dry-run with the following arguments: %q", strings.Join(os.Args, " "))
	} else {
		a.log.AuditInfof("admin tool executing with the following arguments: %q", strings.Join(os.Args, " "))
	}

	err = subcommand.Run(context.Background(), a)
	cmd.FailOnError(err, "executing subcommand")

	if a.dryRun {
		a.log.AuditInfof("admin tool has successfully completed executing a dry-run with the following arguments: %q", strings.Join(os.Args, " "))
		a.log.Info("Dry run complete. Pass -dry-run=false to mutate the database.")
	} else {
		a.log.AuditInfof("admin tool has successfully completed executing with the following arguments: %q", strings.Join(os.Args, " "))
	}
}
