package git

import (
	"bufio"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/internal/config"
)

var (
	sshConfigLineRE = regexp.MustCompile(`\A\s*(?P<keyword>[A-Za-z][A-Za-z0-9]*)(?:\s+|\s*=\s*)(?P<argument>.+)`)
	sshTokenRE      = regexp.MustCompile(`%[%h]`)
)

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

type sshParser struct {
	homeDir string

	aliasMap SSHAliasMap
	hosts    []string

	open func(string) (io.Reader, error)
	glob func(string) ([]string, error)
}

func (p *sshParser) read(fileName string) error {
	var file io.Reader
	if p.open == nil {
		f, err := os.Open(fileName)
		if err != nil {
			return err
		}
		defer f.Close()
		file = f
	} else {
		var err error
		file, err = p.open(fileName)
		if err != nil {
			return err
		}
	}

	if len(p.hosts) == 0 {
		p.hosts = []string{"*"}
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		m := sshConfigLineRE.FindStringSubmatch(scanner.Text())
		if len(m) < 3 {
			continue
		}

		keyword, arguments := strings.ToLower(m[1]), m[2]
		switch keyword {
		case "host":
			p.hosts = strings.Fields(arguments)
		case "hostname":
			for _, host := range p.hosts {
				for _, name := range strings.Fields(arguments) {
					if p.aliasMap == nil {
						p.aliasMap = make(SSHAliasMap)
					}
					p.aliasMap[host] = sshExpandTokens(name, host)
				}
			}
		case "include":
			for _, arg := range strings.Fields(arguments) {
				path := p.absolutePath(fileName, arg)

				var fileNames []string
				if p.glob == nil {
					paths, _ := filepath.Glob(path)
					for _, p := range paths {
						if s, err := os.Stat(p); err == nil && !s.IsDir() {
							fileNames = append(fileNames, p)
						}
					}
				} else {
					var err error
					fileNames, err = p.glob(path)
					if err != nil {
						continue
					}
				}

				for _, fileName := range fileNames {
					_ = p.read(fileName)
				}
			}
		}
	}

	return scanner.Err()
}

func (p *sshParser) absolutePath(parentFile, path string) string {
	if filepath.IsAbs(path) || strings.HasPrefix(filepath.ToSlash(path), "/") {
		return path
	}

	if strings.HasPrefix(path, "~") {
		return filepath.Join(p.homeDir, strings.TrimPrefix(path, "~"))
	}

	if strings.HasPrefix(filepath.ToSlash(parentFile), "/etc/ssh") {
		return filepath.Join("/etc/ssh", path)
	}

	return filepath.Join(p.homeDir, ".ssh", path)
}

// ParseSSHConfig constructs a map of SSH hostname aliases based on user and
// system configuration files
func ParseSSHConfig() SSHAliasMap {
	configFiles := []string{
		"/etc/ssh_config",
		"/etc/ssh/ssh_config",
	}

	p := sshParser{}

	if sshDir, err := config.HomeDirPath(".ssh"); err == nil {
		userConfig := filepath.Join(sshDir, "config")
		configFiles = append([]string{userConfig}, configFiles...)
		p.homeDir = filepath.Dir(sshDir)
	}

	for _, file := range configFiles {
		_ = p.read(file)
	}
	return p.aliasMap
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
