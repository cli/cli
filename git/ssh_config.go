package git

import (
	"bufio"
	"io"
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

type sshAliasMap map[string]string

func sshParseFiles() sshAliasMap {
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

func sshParse(r ...io.Reader) sshAliasMap {
	config := sshAliasMap{}
	for _, file := range r {
		sshParseConfig(config, file)
	}
	return config
}

func sshParseConfig(c sshAliasMap, file io.Reader) error {
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
