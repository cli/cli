package main

import (
	"fmt"
	"os"
	"strings"

	_ "github.com/letsencrypt/boulder/cmd/akamai-purger"
	_ "github.com/letsencrypt/boulder/cmd/bad-key-revoker"
	_ "github.com/letsencrypt/boulder/cmd/boulder-ca"
	_ "github.com/letsencrypt/boulder/cmd/boulder-observer"
	_ "github.com/letsencrypt/boulder/cmd/boulder-publisher"
	_ "github.com/letsencrypt/boulder/cmd/boulder-ra"
	_ "github.com/letsencrypt/boulder/cmd/boulder-sa"
	_ "github.com/letsencrypt/boulder/cmd/boulder-va"
	_ "github.com/letsencrypt/boulder/cmd/boulder-wfe2"
	_ "github.com/letsencrypt/boulder/cmd/cert-checker"
	_ "github.com/letsencrypt/boulder/cmd/crl-checker"
	_ "github.com/letsencrypt/boulder/cmd/crl-storer"
	_ "github.com/letsencrypt/boulder/cmd/crl-updater"
	_ "github.com/letsencrypt/boulder/cmd/email-exporter"
	_ "github.com/letsencrypt/boulder/cmd/log-validator"
	_ "github.com/letsencrypt/boulder/cmd/nonce-service"
	_ "github.com/letsencrypt/boulder/cmd/ocsp-responder"
	_ "github.com/letsencrypt/boulder/cmd/remoteva"
	_ "github.com/letsencrypt/boulder/cmd/reversed-hostname-checker"
	_ "github.com/letsencrypt/boulder/cmd/rocsp-tool"
	_ "github.com/letsencrypt/boulder/cmd/sfe"
	"github.com/letsencrypt/boulder/core"

	"github.com/letsencrypt/boulder/cmd"
)

// readAndValidateConfigFile uses the ConfigValidator registered for the given
// command to validate the provided config file. If the command does not have a
// registered ConfigValidator, this function does nothing.
func readAndValidateConfigFile(name, filename string) error {
	cv := cmd.LookupConfigValidator(name)
	if cv == nil {
		return nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	if name == "boulder-observer" {
		// Only the boulder-observer uses YAML config files.
		return cmd.ValidateYAMLConfig(cv, file)
	}
	return cmd.ValidateJSONConfig(cv, file)
}

// getConfigPath returns the path to the config file if it was provided as a
// command line flag. If the flag was not provided, it returns an empty string.
func getConfigPath() string {
	for i := range len(os.Args) {
		arg := os.Args[i]
		if arg == "--config" || arg == "-config" {
			if i+1 < len(os.Args) {
				return os.Args[i+1]
			}
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config=")
		}
		if strings.HasPrefix(arg, "-config=") {
			return strings.TrimPrefix(arg, "-config=")
		}
	}
	return ""
}

var boulderUsage = fmt.Sprintf(`Usage: %s <subcommand> [flags]

  Each boulder component has its own subcommand. Use --list to see
  a list of the available components. Use <subcommand> --help to
  see the usage for a specific component.
`,
	core.Command())

func main() {
	defer cmd.AuditPanic()

	if len(os.Args) <= 1 {
		// No arguments passed.
		fmt.Fprint(os.Stderr, boulderUsage)
		return
	}

	if os.Args[1] == "--help" || os.Args[1] == "-help" {
		// Help flag passed.
		fmt.Fprint(os.Stderr, boulderUsage)
		return
	}

	if os.Args[1] == "--list" || os.Args[1] == "-list" {
		// List flag passed.
		for _, c := range cmd.AvailableCommands() {
			fmt.Println(c)
		}
		return
	}

	// Remove the subcommand from the arguments.
	command := os.Args[1]
	os.Args = os.Args[1:]

	config := getConfigPath()
	if config != "" {
		// Config flag passed.
		err := readAndValidateConfigFile(command, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error validating config file %q for command %q: %s\n", config, command, err)
			os.Exit(1)
		}
	}

	commandFunc := cmd.LookupCommand(command)
	if commandFunc == nil {
		fmt.Fprintf(os.Stderr, "Unknown subcommand %q.\n", command)
		os.Exit(1)
	}
	commandFunc()
}
