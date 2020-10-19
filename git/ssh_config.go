package git

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mitchellh/go-homedir"
)

var (
	sshTokenRE *regexp.Regexp
)

func init() {
	sshTokenRE = regexp.MustCompile(`%[%h]`)
}

// SSHAliasMap encapsulates the translation of SSH hostname aliases
type SSHAliasMap map[string]string

// Translator returns a function that applies hostname aliases to URLs
func (m SSHAliasMap) Translator() func(*url.URL) *url.URL {
	return func(u *url.URL) *url.URL {
		if u.Scheme != "ssh" {
			return u
		}
		resolvedHost, ok := m[u.Hostname()]
		if !ok {
			return u
		}
		// FIXME: cleanup domain logic
		if strings.EqualFold(u.Hostname(), "github.com") && strings.EqualFold(resolvedHost, "ssh.github.com") {
			return u
		}
		newURL, _ := url.Parse(u.String())
		newURL.Host = resolvedHost
		return newURL
	}
}

type parser struct {
	aliasMap SSHAliasMap
}

func (p *parser) read(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	hosts := []string{"*"}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		if len(fields) < 2 {
			continue
		}

		directive, params := fields[0], fields[1:]
		switch {
		case strings.EqualFold(directive, "Host"):
			hosts = params
		case strings.EqualFold(directive, "Hostname"):
			for _, host := range hosts {
				for _, name := range params {
					p.aliasMap[host] = sshExpandTokens(name, host)
				}
			}
		case strings.EqualFold(directive, "Include"):
			for _, path := range absolutePaths(fileName, params) {
				fileNames, err := filepath.Glob(path)
				if err != nil {
					continue
				}

				for _, fileName := range fileNames {
					_ = p.read(fileName)
				}
			}
		}
	}

	return scanner.Err()
}

func isSystem(path string) bool {
	return strings.HasPrefix(path, "/etc/ssh")
}

func absolutePaths(parentFile string, paths []string) []string {
	absPaths := make([]string, len(paths))

	for i, path := range paths {
		switch {
		case filepath.IsAbs(path):
			absPaths[i] = path
		case strings.HasPrefix(path, "~"):
			absPaths[i], _ = homedir.Expand(path)
		case isSystem(parentFile):
			absPaths[i] = filepath.Join("/etc", "ssh", path)
		default:
			dir, _ := homedir.Dir()
			absPaths[i] = filepath.Join(dir, ".ssh", path)
		}
	}

	return absPaths
}

func parse(files ...string) SSHAliasMap {
	p := parser{aliasMap: make(SSHAliasMap)}

	for _, file := range files {
		_ = p.read(file)
	}

	return p.aliasMap
}

// ParseSSHConfig constructs a map of SSH hostname aliases based on user and
// system configuration files
func ParseSSHConfig() SSHAliasMap {
	configFiles := []string{
		"/etc/ssh_config",
		"/etc/ssh/ssh_config",
	}
	if homedir, err := homedir.Dir(); err == nil {
		userConfig := filepath.Join(homedir, ".ssh", "config")
		configFiles = append([]string{userConfig}, configFiles...)
	}

	return parse(configFiles...)
}

func sshExpandTokens(text, host string) string {
	return sshTokenRE.ReplaceAllStringFunc(text, func(match string) string {
		switch match {
		case "%h":
			return host
		case "%%":
			return "%"
		}
		return ""
	})
}
