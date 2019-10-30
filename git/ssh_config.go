package git

import (
	"bufio"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mitchellh/go-homedir"
)

var (
	sshHostRE,
	sshTokenRE *regexp.Regexp
)

func init() {
	sshHostRE = regexp.MustCompile("(?i)^[ \t]*(host|hostname)[ \t]+(.+)$")
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

	openFiles := []io.Reader{}
	for _, file := range configFiles {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		defer f.Close()
		openFiles = append(openFiles, f)
	}
	return sshParse(openFiles...)
}

func sshParse(r ...io.Reader) SSHAliasMap {
	config := SSHAliasMap{}
	for _, file := range r {
		sshParseConfig(config, file)
	}
	return config
}

func sshParseConfig(c SSHAliasMap, file io.Reader) error {
	hosts := []string{"*"}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		match := sshHostRE.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		names := strings.Fields(match[2])
		if strings.EqualFold(match[1], "host") {
			hosts = names
		} else {
			for _, host := range hosts {
				for _, name := range names {
					c[host] = sshExpandTokens(name, host)
				}
			}
		}
	}

	return scanner.Err()
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
