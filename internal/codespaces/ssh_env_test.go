package codespaces

import (
	"context"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestGetSSHCommand(t *testing.T) {
	// Save original env var to restore later
	originalValue := os.Getenv("GH_CS_SSH_COMMAND")
	defer os.Setenv("GH_CS_SSH_COMMAND", originalValue)

	tests := []struct {
		name           string
		envValue       string
		expectedCmd    string
		expectedArgs   []string
		expectedErrMsg string
	}{
		{
			name:         "no env var set",
			envValue:     "",
			expectedCmd:  "ssh", // This assumes ssh is in PATH
			expectedArgs: nil,
		},
		{
			name:         "simple command",
			envValue:     "/custom/path/to/ssh",
			expectedCmd:  "/custom/path/to/ssh",
			expectedArgs: []string{},
		},
		{
			name:         "command with args",
			envValue:     "/custom/path/to/ssh -o IdentityFile=/path/to/key",
			expectedCmd:  "/custom/path/to/ssh",
			expectedArgs: []string{"-o", "IdentityFile=/path/to/key"},
		},
		{
			name:         "command with quoted args",
			envValue:     "\"/path/with space/ssh\" -o \"Option=Value With Space\"",
			expectedCmd:  "/path/with space/ssh",
			expectedArgs: []string{"-o", "Option=Value With Space"},
		},
		{
			name:         "command with single quotes",
			envValue:     "'/path/with space/ssh' -o 'Option=Value With Space'",
			expectedCmd:  "/path/with space/ssh",
			expectedArgs: []string{"-o", "Option=Value With Space"},
		},
		{
			name:           "empty command after parsing",
			envValue:       "   ",
			expectedErrMsg: "empty GH_CS_SSH_COMMAND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			os.Setenv("GH_CS_SSH_COMMAND", tt.envValue)

			// Call the function
			cmd, args, err := getSSHCommand()

			// Check error
			if tt.expectedErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectedErrMsg)
				}
				if err.Error() != tt.expectedErrMsg {
					t.Fatalf("expected error %q, got %q", tt.expectedErrMsg, err.Error())
				}
				return
			}

			// Check no error
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// For the "no env var" case, we just check that we got a non-empty command
			if tt.envValue == "" {
				if cmd == "" {
					t.Fatal("expected non-empty command")
				}
				return
			}

			// Check command
			if cmd != tt.expectedCmd {
				t.Errorf("expected command %q, got %q", tt.expectedCmd, cmd)
			}

			// Check args
			if !reflect.DeepEqual(args, tt.expectedArgs) {
				t.Errorf("expected args %v, got %v", tt.expectedArgs, args)
			}
		})
	}
}

func TestGetSCPCommand(t *testing.T) {
	// Save original env var to restore later
	originalValue := os.Getenv("GH_CS_SCP_COMMAND")
	defer os.Setenv("GH_CS_SCP_COMMAND", originalValue)

	tests := []struct {
		name           string
		envValue       string
		expectedCmd    string
		expectedArgs   []string
		expectedErrMsg string
	}{
		{
			name:         "no env var set",
			envValue:     "",
			expectedCmd:  "scp", // This assumes scp is in PATH
			expectedArgs: nil,
		},
		{
			name:         "simple command",
			envValue:     "/custom/path/to/scp",
			expectedCmd:  "/custom/path/to/scp",
			expectedArgs: []string{},
		},
		{
			name:         "command with args",
			envValue:     "/custom/path/to/scp -F /path/to/config",
			expectedCmd:  "/custom/path/to/scp",
			expectedArgs: []string{"-F", "/path/to/config"},
		},
		{
			name:         "command with quoted args",
			envValue:     "\"/path/with space/scp\" -F \"/path/to/config with spaces\"",
			expectedCmd:  "/path/with space/scp",
			expectedArgs: []string{"-F", "/path/to/config with spaces"},
		},
		{
			name:         "command with single quotes",
			envValue:     "'/path/with space/scp' -F '/path/to/config with spaces'",
			expectedCmd:  "/path/with space/scp",
			expectedArgs: []string{"-F", "/path/to/config with spaces"},
		},
		{
			name:           "empty command after parsing",
			envValue:       "   ",
			expectedErrMsg: "empty GH_CS_SCP_COMMAND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			os.Setenv("GH_CS_SCP_COMMAND", tt.envValue)

			// Call the function
			cmd, args, err := getSCPCommand()

			// Check error
			if tt.expectedErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectedErrMsg)
				}
				if err.Error() != tt.expectedErrMsg {
					t.Fatalf("expected error %q, got %q", tt.expectedErrMsg, err.Error())
				}
				return
			}

			// Check no error
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// For the "no env var" case, we just check that we got a non-empty command
			if tt.envValue == "" {
				if cmd == "" {
					t.Fatal("expected non-empty command")
				}
				return
			}

			// Check command
			if cmd != tt.expectedCmd {
				t.Errorf("expected command %q, got %q", tt.expectedCmd, cmd)
			}

			// Check args
			if !reflect.DeepEqual(args, tt.expectedArgs) {
				t.Errorf("expected args %v, got %v", tt.expectedArgs, args)
			}
		})
	}
}

func TestNewSSHCommandWithEnvVar(t *testing.T) {
	// Save original env var to restore later
	originalValue := os.Getenv("GH_CS_SSH_COMMAND")
	defer os.Setenv("GH_CS_SSH_COMMAND", originalValue)

	tests := []struct {
		name         string
		envValue     string
		cmdArgs      []string
		command      []string
		expectedArgs []string
	}{
		{
			name:     "no env var",
			envValue: "",
			cmdArgs:  []string{"-v"},
			command:  []string{"ls", "-la"},
			// We don't check the exact args here because they'll include connection args
		},
		{
			name:     "env var with args",
			envValue: "/custom/ssh -o CustomOption=Value",
			cmdArgs:  []string{"-v"},
			command:  []string{"ls", "-la"},
			// We expect the env var args to be prepended to the command args
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			os.Setenv("GH_CS_SSH_COMMAND", tt.envValue)

			// Call the function
			cmd, connArgs, err := newSSHCommand(context.Background(), 2222, "user@host", tt.cmdArgs, tt.command)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check that we got a command
			if cmd == nil {
				t.Fatal("expected non-nil command")
			}

			// Check that connArgs is non-nil
			if connArgs == nil {
				t.Fatal("expected non-nil connArgs")
			}

			// Check command path and arguments from env var
			if tt.envValue != "" {
				// Get the command path
				expectedPath := tt.envValue
				if i := strings.Index(tt.envValue, " "); i != -1 {
					expectedPath = tt.envValue[:i]

					// Check that the arguments are included
					parts := strings.Split(tt.envValue, " ")
					for _, arg := range parts[1:] { // Skip the command path
						found := slices.Contains(cmd.Args, arg)
						if !found {
							t.Errorf("env var arg %q not found in args: %v", arg, cmd.Args)
						}
					}
				}

				// Check the command path
				if cmd.Path != expectedPath {
					t.Errorf("expected command path %q, got %q", expectedPath, cmd.Path)
				}
			}

			// Check that the command args include the user command
			for _, cmdArg := range tt.command {
				found := slices.Contains(cmd.Args, cmdArg)
				if !found {
					t.Errorf("command arg %q not found in args: %v", cmdArg, cmd.Args)
				}
			}
		})
	}
}

func TestNewSCPCommandWithEnvVar(t *testing.T) {
	// Save original env var to restore later
	originalValue := os.Getenv("GH_CS_SCP_COMMAND")
	defer os.Setenv("GH_CS_SCP_COMMAND", originalValue)

	tests := []struct {
		name         string
		envValue     string
		cmdArgs      []string
		expectedArgs []string
	}{
		{
			name:     "no env var",
			envValue: "",
			cmdArgs:  []string{"-v", "local/file", "remote:file"},
			// We don't check the exact args here because they'll include connection args
		},
		{
			name:     "env var with args",
			envValue: "/custom/scp -F /custom/config",
			cmdArgs:  []string{"-v", "local/file", "remote:file"},
			// We expect the env var args to be prepended to the command args
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			os.Setenv("GH_CS_SCP_COMMAND", tt.envValue)

			// Call the function
			cmd, err := newSCPCommand(context.Background(), 2222, "user@host", tt.cmdArgs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check that we got a command
			if cmd == nil {
				t.Fatal("expected non-nil command")
			}

			// Check command path and arguments from env var
			if tt.envValue != "" {
				// Get the command path
				expectedPath := tt.envValue
				if i := strings.Index(tt.envValue, " "); i != -1 {
					expectedPath = tt.envValue[:i]

					// Check that the arguments are included
					parts := strings.Split(tt.envValue, " ")
					for _, arg := range parts[1:] { // Skip the command path
						found := slices.Contains(cmd.Args, arg)
						if !found {
							t.Errorf("custom option %q not found in args: %v", arg, cmd.Args)
						}
					}
				}

				// Check the command path
				if cmd.Path != expectedPath {
					t.Errorf("expected command path %q, got %q", expectedPath, cmd.Path)
				}
			}

			// Check that the command args include the file arguments
			for _, cmdArg := range []string{"local/file", "user@host:file"} {
				found := false
				for _, arg := range cmd.Args {
					if strings.Contains(arg, cmdArg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("file arg containing %q not found in args: %v", cmdArg, cmd.Args)
				}
			}
		})
	}
}
