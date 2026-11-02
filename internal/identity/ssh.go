package identity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kballard/go-shellquote"
)

// SSHArguments routes a Git SSH invocation through its selected identity's declared alias.
// It does not authenticate the key or mutate remote URLs or SSH configuration.
func (s *Store) SSHArguments(args []string, dir string) ([]string, error) {
	return s.sshArguments(args, dir, "", "")
}

// SSHArgumentsForAccount selects a declared route for an invocation-local account override.
// The override applies only to its host and does not change Git attribution or saved choices.
func (s *Store) SSHArgumentsForAccount(args []string, dir, overrideHost, account string) ([]string, error) {
	return s.sshArguments(args, dir, overrideHost, account)
}

func (s *Store) sshArguments(args []string, dir, overrideHost, account string) ([]string, error) {
	hostIndex := -1
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				hostIndex = i + 1
			}
			break
		}
		if !strings.HasPrefix(arg, "-") {
			hostIndex = i
			break
		}
		if len(arg) == 2 && strings.ContainsAny(arg[1:], "BbcDEeFIiJLlmOoPpQRSWw") {
			i++
			if i == len(args) {
				return nil, fmt.Errorf("SSH option %s requires a value", arg)
			}
		}
	}
	if hostIndex < 0 {
		return nil, errors.New("Git SSH invocation is missing its host")
	}
	destination := args[hostIndex]
	user, host, hasUser := strings.Cut(destination, "@")
	if !hasUser {
		host = destination
	}
	owner := ""
	if len(args) > hostIndex+1 {
		command, err := shellquote.Split(args[hostIndex+1])
		if err != nil {
			return nil, fmt.Errorf("parse Git SSH command: %w", err)
		}
		if len(command) >= 2 {
			owner, _, _ = strings.Cut(strings.TrimPrefix(command[len(command)-1], "/"), "/")
		}
	}
	var id *Identity
	var err error
	if account != "" {
		id, err = s.accountSSHRoute(host, owner, dir, overrideHost, account)
	} else {
		id, err = s.Resolve(host, owner, dir)
	}
	if err != nil {
		return nil, err
	}
	if id == nil {
		return args, nil
	}
	if account == "" || !strings.EqualFold(id.Host, overrideHost) {
		if err := CheckEnvironment(id.Host); err != nil {
			return nil, err
		}
	}
	if id.SSHHost == "" {
		return nil, fmt.Errorf("identity %q has no SSH route; run 'gh auth identity edit %s --ssh-host <alias>'", id.Name, id.Name)
	}
	result := append([]string(nil), args...)
	result[hostIndex] = id.SSHHost
	if hasUser {
		result[hostIndex] = user + "@" + id.SSHHost
	}
	return result, nil
}

func (s *Store) accountSSHRoute(host, owner, dir, overrideHost, account string) (*Identity, error) {
	ids, err := s.List()
	if err != nil {
		return nil, err
	}
	canonical := host
	known := strings.EqualFold(host, overrideHost)
	for _, id := range ids {
		if strings.EqualFold(id.Host, host) {
			known = true
		}
		if !strings.EqualFold(id.SSHHost, host) {
			continue
		}
		if known && !strings.EqualFold(canonical, id.Host) {
			return nil, fmt.Errorf("SSH host %q is declared for multiple hosts", host)
		}
		canonical, known = id.Host, true
	}
	if !known {
		return nil, fmt.Errorf("cannot apply --account to undeclared SSH host %q; declare its route with 'gh auth identity create' or 'gh auth identity edit'", host)
	}
	if !strings.EqualFold(canonical, overrideHost) {
		return s.Resolve(canonical, owner, dir)
	}
	routes := map[string]Identity{}
	candidates := map[string]bool{}
	for _, id := range ids {
		if !strings.EqualFold(id.Host, overrideHost) || !strings.EqualFold(id.Account, account) || id.SSHHost == "" {
			continue
		}
		routes[strings.ToLower(id.SSHHost)] = id
		candidates[id.Name] = true
	}
	for _, id := range ids {
		if _, ok := routes[strings.ToLower(id.SSHHost)]; ok &&
			(!strings.EqualFold(id.Host, overrideHost) || !strings.EqualFold(id.Account, account)) {
			return nil, fmt.Errorf("SSH host %q is declared for multiple accounts or hosts; use distinct aliases in 'gh auth identity edit'", id.SSHHost)
		}
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("account %q has no declared SSH route on %s; declare one with 'gh auth identity create' or 'gh auth identity edit'", account, host)
	}
	if len(routes) == 1 {
		for _, id := range routes {
			return &id, nil
		}
	}
	bindings, err := s.Bindings()
	if err != nil {
		return nil, err
	}
	var matching []Binding
	for _, binding := range bindings {
		if candidates[binding.Identity] {
			matching = append(matching, binding)
		}
	}
	resolution, err := s.resolveBindings(overrideHost, owner, dir, matching)
	if err != nil {
		return nil, err
	}
	if resolution.Identity != nil {
		return resolution.Identity, nil
	}
	return nil, fmt.Errorf("multiple SSH routes are declared for account %q on %s; apply the intended identity with 'gh auth use'", account, host)
}
