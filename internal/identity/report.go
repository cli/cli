package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kballard/go-shellquote"
)

// Effect distinguishes configured intent from observed settings and blockers.
type Effect struct {
	Status string `json:"status"`
	Value  string `json:"value,omitempty"`
	Origin string `json:"origin,omitempty"`
	Detail string `json:"detail"`
}

// Include describes an include directive encountered by Git, not an inferred binding.
type Include struct {
	Source         string `json:"source"`
	Target         string `json:"target"`
	Condition      string `json:"condition,omitempty"`
	TargetObserved bool   `json:"targetObserved"`
}

// Report is the shared semantic explanation for text and structured output.
// It describes persistent configuration, not verified remote authentication.
type Report struct {
	Context struct {
		Directory      string `json:"directory"`
		Repository     string `json:"repository,omitempty"`
		CommonGitDir   string `json:"commonGitDir,omitempty"`
		ScopeDirectory string `json:"scopeDirectory"`
		Host           string `json:"host"`
		Owner          string `json:"owner,omitempty"`
		Scope          string `json:"scope"`
	} `json:"context"`
	Selection struct {
		Identity    *Identity `json:"identity"`
		Selected    *Binding  `json:"selected"`
		Superseded  []Match   `json:"superseded"`
		Descendants []Binding `json:"descendants"`
	} `json:"selection"`
	Authentication struct {
		API   Effect `json:"api"`
		HTTPS Effect `json:"https"`
		SSH   Effect `json:"ssh"`
	} `json:"authentication"`
	Attribution struct {
		Status    string      `json:"status"`
		Name      ConfigValue `json:"name"`
		Email     ConfigValue `json:"email"`
		Overrides []string    `json:"overrides"`
	} `json:"attribution"`
	Configuration []Include `json:"configuration"`
	Warnings      []string  `json:"warnings"`
}

// Match explains why a matching binding did not change the selection.
type Match struct {
	Binding Binding `json:"binding"`
	Reason  string  `json:"reason"`
}

// ExplainReport collects evidence once, without network probes or configuration writes.
func (s *Store) ExplainReport(host, owner, dir, executable string) (*Report, error) {
	raw, err := s.Explain(host, owner, dir)
	if err != nil {
		return nil, err
	}
	r := &Report{Warnings: []string{}}
	r.Context.Directory, r.Context.Repository, r.Context.Host, r.Context.Owner = raw.Directory, raw.Repository, host, owner
	r.Context.CommonGitDir, r.Context.ScopeDirectory = raw.CommonGitDir, raw.ScopeDirectory
	r.Context.Scope = "persistent"
	r.Selection.Identity = raw.Identity
	r.Selection.Superseded = []Match{}
	r.Selection.Descendants = append([]Binding{}, raw.Descendants...)
	r.Attribution.Overrides = []string{}
	if len(raw.Matches) > 0 {
		selected := raw.Matches[0]
		r.Selection.Selected = &selected
		for _, other := range raw.Matches[1:] {
			reason := "more specific directory"
			if selected.Repository != "" {
				reason = "repository choices take precedence"
			} else if other.Owner != "" {
				reason = "directory choices take precedence"
				if selected.Owner != "" {
					reason = "same identity"
				}
			}
			r.Selection.Superseded = append(r.Selection.Superseded, Match{other, reason})
		}
	}
	r.Authentication.API = Effect{Status: "unmanaged", Detail: "Uses your usual gh account settings"}
	r.Authentication.HTTPS = Effect{Status: "unmanaged", Detail: "Uses your usual Git credential settings"}
	r.Authentication.SSH = Effect{Status: "unmanaged", Detail: "Uses your usual SSH settings"}
	if id := raw.Identity; id != nil {
		r.Authentication.API = Effect{Status: "declared", Value: id.Account, Detail: "Uses the saved account on " + id.Host}
		r.Authentication.HTTPS = Effect{Status: "declared", Value: id.Account, Detail: "Uses gh's Git credential helper"}
		if err := CheckEnvironment(id.Host); err != nil {
			r.Authentication.API.Status, r.Authentication.HTTPS.Status = "blocked", "blocked"
			r.Authentication.API.Detail, r.Authentication.HTTPS.Detail = err.Error(), err.Error()
		}
		switch {
		case os.Getenv("GIT_SSH_COMMAND") != "":
			r.Authentication.SSH = Effect{Status: "overridden", Origin: "GIT_SSH_COMMAND", Detail: "Uses GIT_SSH_COMMAND instead of this identity's SSH host"}
		case raw.SSHCommand.Value == "":
			r.Authentication.SSH.Detail = "Uses your usual SSH settings; Git is not set up to use this identity's SSH host"
			if os.Getenv("GIT_SSH") != "" {
				r.Authentication.SSH.Origin = "GIT_SSH"
			}
		case raw.SSHCommand.Value != shellquote.Join(executable, "auth", "git-ssh"):
			r.Authentication.SSH = Effect{Status: "overridden", Origin: raw.SSHCommand.Origin, Detail: "Uses core.sshCommand instead of this identity's SSH host"}
		case id.SSHHost == "":
			r.Authentication.SSH = Effect{Status: "blocked", Detail: "Not configured. To use SSH, run: gh auth identity edit " + id.Name + " --ssh-host <alias>"}
		default:
			r.Authentication.SSH = Effect{Status: "declared", Value: id.SSHHost, Origin: raw.SSHCommand.Origin, Detail: "gh has not checked which account this SSH host signs in as."}
			command, err := s.SSHCommand(raw.Directory, false)
			if err != nil {
				r.Authentication.SSH.Status = "blocked"
				r.Authentication.SSH.Detail = err.Error()
			} else {
				r.Authentication.SSH.Detail = "Connects using " + command[0] + ". " + r.Authentication.SSH.Detail
			}
		}
		if r.Authentication.SSH.Status == "declared" && r.Authentication.API.Status == "blocked" {
			r.Authentication.SSH.Status = "blocked"
			r.Authentication.SSH.Detail = r.Authentication.API.Detail
		}
	}
	r.Attribution.Status = "unavailable"
	if raw.Repository != "" {
		r.Attribution.Status = "observed"
		r.Attribution.Name, r.Attribution.Email = raw.GitUserName, raw.GitUserEmail
	} else if id := raw.Identity; id != nil {
		r.Attribution.Status = "declared"
		r.Attribution.Name.Value, r.Attribution.Email.Value = id.GitUserName, id.GitUserEmail
	}
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		if _, set := os.LookupEnv(name); set {
			r.Attribution.Overrides = append(r.Attribution.Overrides, name)
		}
	}
	configDir := raw.Directory
	if raw.Repository != "" {
		configDir = raw.Repository
	}
	r.Configuration, err = configurationIncludes(configDir)
	if err != nil {
		return nil, err
	}
	if raw.Identity != nil && raw.Repository != "" {
		observed := false
		for _, edge := range r.Configuration {
			if edge.Target == s.IdentityPath(raw.Identity.Name) && edge.TargetObserved {
				observed = true
			}
		}
		if !observed {
			r.Warnings = append(r.Warnings, "Git has not loaded settings from the selected identity's file. Check the includes in your Git configuration.")
		}
	}
	return r, nil
}

func configurationIncludes(dir string) ([]Include, error) {
	out, err := gitOutput(dir, "config", "--includes", "--show-origin", "--null", "--list")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	edges := []Include{}
	observed := map[string]bool{}
	if out == "" {
		return edges, nil
	}
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("Git returned malformed configuration origins")
	}
	for i := 0; i < len(fields); i += 2 {
		source := strings.TrimPrefix(fields[i], "file:")
		if strings.HasPrefix(fields[i], "file:") {
			if !filepath.IsAbs(source) {
				source = filepath.Join(dir, source)
			}
			source = filepath.Clean(source)
			observed[source] = true
		}
		key, value, _ := strings.Cut(fields[i+1], "\n")
		condition := ""
		if key != "include.path" {
			if !strings.HasPrefix(key, "includeif.") || !strings.HasSuffix(key, ".path") {
				continue
			}
			condition = strings.TrimSuffix(strings.TrimPrefix(key, "includeif."), ".path")
		}
		target := value
		if strings.HasPrefix(target, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			target = filepath.Join(home, target[2:])
		} else if !filepath.IsAbs(target) && strings.HasPrefix(fields[i], "file:") {
			target = filepath.Join(filepath.Dir(source), target)
		}
		edges = append(edges, Include{Source: source, Target: target, Condition: condition})
	}
	for i := range edges {
		edges[i].TargetObserved = observed[edges[i].Target]
	}
	return edges, nil
}
