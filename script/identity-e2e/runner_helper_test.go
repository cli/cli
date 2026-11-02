//go:build unix

package identitye2e

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRunnerHelperProcess supplies credential-free gh and SSH executables to
// the real Bash bootstrap. The test binary is invoked by test-owned wrappers.
func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("IDENTITY_RUNNER_HELPER") != "1" {
		return
	}
	args := os.Args[4:]
	var err error
	switch os.Args[3] {
	case "gh":
		err = runnerFakeGH(args)
	case "ssh":
		err = runnerFakeSSH(args)
	default:
		err = fmt.Errorf("unexpected runner helper: %q", os.Args[3])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

type runnerAccounts struct {
	Users   []string `json:"users"`
	Active  string   `json:"active"`
	Browser int      `json:"browser"`
}

func runnerValue(args []string, flag string) string {
	index := slices.Index(args, flag)
	if index < 0 || index+1 >= len(args) {
		return ""
	}
	return args[index+1]
}

func runnerStartsWith(args []string, prefix ...string) bool {
	return len(args) >= len(prefix) && slices.Equal(args[:len(prefix)], prefix)
}

func runnerFakeGH(args []string) error {
	root := os.Getenv("GH_CONFIG_DIR")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(os.Getenv("FIXTURE_CALLS"), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(log).Encode(runnerCall{
		Args: args, Config: root, Home: os.Getenv("HOME"),
		KeyringDisabled: os.Getenv("GH_INTERNAL_DISABLE_KEYRING"),
	})
	closeErr := log.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	normal := root == os.Getenv("FIXTURE_NORMAL_CONFIG")
	mode := os.Getenv("FIXTURE_MODE")
	statePath := filepath.Join(root, "fake-accounts.json")
	var state runnerAccounts
	data, err := os.ReadFile(statePath)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
	case os.IsNotExist(err):
		if normal && mode != "missing" && mode != "login-disabled" {
			state.Users = []string{"alice", "bob"}
		}
		if normal && mode == "multiple" {
			state.Users = append(state.Users, "charlie")
		}
		if len(state.Users) > 0 {
			state.Active = state.Users[len(state.Users)-1]
		}
	default:
		return err
	}
	save := func() error {
		data, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return os.WriteFile(statePath, data, 0o600)
	}
	switch {
	case slices.Contains(args, "--help"), slices.Equal(args, []string{"version"}):
		fmt.Println("fake gh for account bootstrap tests")
		if slices.Equal(args, []string{"version"}) && os.Getenv("GH_INTERNAL_DISABLE_KEYRING") == "true" && mode != "no-keyring-isolation" {
			fmt.Println("OS keyring: disabled (isolated debug workflow)")
		}
	case runnerStartsWith(args, "auth", "status"):
		if !slices.Contains(args, "--json") {
			if len(state.Users) == 0 {
				return fmt.Errorf("fixture: no saved accounts")
			}
			return nil
		}
		if !slices.Contains(args, "--jq") {
			return fmt.Errorf("fixture expects a jq projection")
		}
		query := runnerValue(args, "--jq")
		if strings.Contains(query, "state ==") && normal && mode == "unhealthy" {
			return fmt.Errorf("fixture: unhealthy accounts")
		}
		for _, user := range state.Users {
			switch {
			case strings.Contains(query, ".scopes"):
				scopes := "repo, delete_repo, admin:public_key"
				if normal && mode == "missing-scopes" {
					scopes = "read:org, gist"
				}
				fmt.Printf("%s\t%s\n", user, scopes)
			case strings.Contains(query, ".active"):
				fmt.Printf("%s\t%t\n", user, user == state.Active)
			default:
				fmt.Println(user)
			}
		}
	case runnerStartsWith(args, "auth", "login"):
		if mode == "login-disabled" {
			return fmt.Errorf("fixture: browser login disabled")
		}
		var user string
		if slices.Contains(args, "--with-token") {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			token := strings.TrimSpace(string(data))
			if token != "fixture-alice-token" && token != "fixture-bob-token" {
				return fmt.Errorf("fixture: invalid restored token")
			}
			user = strings.Split(token, "-")[1]
		} else {
			state.Browser++
			user = "bob"
			if state.Browser == 1 || mode == "duplicate-login" {
				user = "alice"
			}
		}
		if !slices.Contains(state.Users, user) {
			state.Users = append(state.Users, user)
		}
		state.Active = user
		return save()
	case runnerStartsWith(args, "auth", "logout"):
		index := slices.Index(state.Users, runnerValue(args, "--user"))
		if index < 0 {
			return fmt.Errorf("fixture: unknown logout account")
		}
		state.Users = slices.Delete(state.Users, index, index+1)
		state.Active = ""
		if len(state.Users) > 0 {
			state.Active = state.Users[len(state.Users)-1]
		}
		return save()
	case runnerStartsWith(args, "auth", "token"):
		if normal && mode == "unavailable-token" {
			return fmt.Errorf("fixture: token unavailable")
		}
		fmt.Printf("fixture-%s-token\n", runnerValue(args, "--user"))
	case runnerStartsWith(args, "auth", "identity", "delete"):
	case runnerStartsWith(args, "api", "user"):
		fmt.Println(state.Active)
	case runnerStartsWith(args, "api", "meta"):
		fmt.Println("ssh-ed25519 fixture-public-host-key")
	case runnerStartsWith(args, "ssh-key", "add"):
	case runnerStartsWith(args, "repo", "delete"):
		if os.Getenv("FIXTURE_CLEANUP_FAIL") == "1" {
			return fmt.Errorf("fixture: deletion failed")
		}
	case runnerStartsWith(args, "api", "--method", "DELETE"):
	case runnerStartsWith(args, "api", "user/keys"):
		fmt.Println("123")
	default:
		return fmt.Errorf("unexpected fake gh invocation: %q", args)
	}
	return nil
}

func runnerFakeSSH(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("fixture: missing SSH destination")
	}
	var user, index string
	switch args[len(args)-1] {
	case "git@identity-account-1":
		user, index = "alice", "1"
	case "git@identity-account-2":
		user, index = "bob", "2"
	default:
		return fmt.Errorf("unexpected fake SSH destination: %q", args)
	}
	switch {
	case slices.Contains(args, "-G"):
		fmt.Println("hostname github.com")
		fmt.Println("identityfile " + filepath.Join(filepath.Dir(runnerValue(args, "-F")), "key-"+index))
		return nil
	case slices.Contains(args, "-T"):
		fmt.Printf("Hi %s! You've successfully authenticated.\n", user)
		// GitHub's successful SSH authentication probe exits with status 1.
		os.Exit(1)
	}
	return fmt.Errorf("unexpected fake SSH invocation: %q", args)
}
