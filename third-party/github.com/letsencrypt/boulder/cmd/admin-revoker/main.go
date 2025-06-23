package notmain

import (
	"fmt"
	"os"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/features"
)

type Config struct {
	Revoker struct {
		DB cmd.DBConfig
		// Similarly, the Revoker needs a TLSConfig to set up its GRPC client
		// certs, but doesn't get the TLS field from ServiceConfig, so declares
		// its own.
		TLS cmd.TLSConfig

		RAService *cmd.GRPCClientConfig
		SAService *cmd.GRPCClientConfig

		Features features.Config
	}

	Syslog cmd.SyslogConfig
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println("use `admin -h` to learn how to use the new admin tool")
		os.Exit(1)
	}

	command := os.Args[1]
	switch {
	case command == "serial-revoke":
		fmt.Println("use `admin -config path/to/cfg.json revoke-cert -serial deadbeef -reason X` instead")

	case command == "batched-serial-revoke":
		fmt.Println("use `admin -config path/to/cfg.json revoke-cert -serials-file path -reason X` instead")

	case command == "reg-revoke":
		fmt.Println("use `admin -config path/to/cfg.json revoke-cert -reg-id Y -reason X` instead")

	case command == "malformed-revoke":
		fmt.Println("use `admin -config path/to/cfg.json revoke-cert -serial deadbeef -reason X -malformed` instead")

	case command == "list-reasons":
		fmt.Println("use `admin -config path/to/cfg.json revoke-cert -h` instead")

	case command == "private-key-revoke":
		fmt.Println("use `admin -config path/to/cfg.json revoke-cert -private-key path -reason X` instead")

	case command == "private-key-block":
		fmt.Println("use `admin -config path/to/cfg.json block-key -private-key path -comment foo` instead")

	case command == "incident-table-revoke":
		fmt.Println("use `admin -config path/to/cfg.json revoke-cert -incident-table tablename -reason X` instead")

	case command == "clear-email":
		fmt.Println("use `admin -config path/to/cfg.json update-email -address foo@bar.org -clear` instead")

	default:
		fmt.Println("use `admin -h` to see a list of flags and subcommands for the new admin tool")
	}
}

func init() {
	cmd.RegisterCommand("admin-revoker", main, &cmd.ConfigValidator{Config: &Config{}})
}
