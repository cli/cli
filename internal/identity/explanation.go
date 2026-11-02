package identity

import (
	"fmt"
	"strings"
)

// ConfigValue is an effective Git value together with the file or scope supplying it.
type ConfigValue struct {
	Value  string `json:"value"`
	Origin string `json:"origin,omitempty"`
}

// Explanation describes the context, precedence, and effective Git defaults of a selection.
type Explanation struct {
	Directory      string
	Repository     string
	CommonGitDir   string
	ScopeDirectory string
	Host           string
	Identity       *Identity
	Matches        []Binding
	Descendants    []Binding
	GitUserName    ConfigValue
	GitUserEmail   ConfigValue
	SSHCommand     ConfigValue
}

// Explain uses the normal selection path and reads Git's actual effective defaults without writing configuration.
func (s *Store) Explain(host, owner, dir string) (*Explanation, error) {
	result, err := s.resolve(host, owner, dir)
	if err != nil {
		return nil, err
	}
	for _, field := range []struct {
		key   string
		value *ConfigValue
	}{
		{"user.name", &result.GitUserName},
		{"user.email", &result.GitUserEmail},
		{"core.sshCommand", &result.SSHCommand},
	} {
		out, err := gitOptional(result.Directory, "config", "--show-origin", "--get", field.key)
		if err != nil {
			return nil, err
		}
		if out == "" {
			continue
		}
		origin, value, ok := strings.Cut(out, "\t")
		if !ok {
			return nil, fmt.Errorf("Git did not report an origin for %s", field.key)
		}
		*field.value = ConfigValue{Value: value, Origin: origin}
	}
	return result, nil
}
