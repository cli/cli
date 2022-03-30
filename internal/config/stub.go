package config

import (
	"errors"
)

type ConfigStub map[string]string

func genKey(host, key string) string {
	if host != "" {
		return host + ":" + key
	}
	return key
}

func (c ConfigStub) Get(host, key string) (string, error) {
	val, _, err := c.GetWithSource(host, key)
	return val, err
}

func (c ConfigStub) GetWithSource(host, key string) (string, string, error) {
	if v, found := c[genKey(host, key)]; found {
		return v, "(memory)", nil
	}
	return "", "", errors.New("not found")
}

func (c ConfigStub) GetOrDefault(hostname, key string) (val string, err error) {
	val, _, err = c.GetOrDefaultWithSource(hostname, key)
	return
}

func (c ConfigStub) GetOrDefaultWithSource(hostname, key string) (val string, src string, err error) {
	val, src, err = c.GetWithSource(hostname, key)
	if err == nil && val == "" {
		val = c.Default(key)
	}
	return
}

func (c ConfigStub) Default(key string) string {
	return defaultFor(key)
}

func (c ConfigStub) Set(host, key, value string) error {
	c[genKey(host, key)] = value
	return nil
}

func (c ConfigStub) Aliases() (*AliasConfig, error) {
	return nil, nil
}

func (c ConfigStub) Hosts() ([]string, error) {
	return nil, nil
}

func (c ConfigStub) UnsetHost(hostname string) {
}

func (c ConfigStub) CheckWriteable(host, key string) error {
	return nil
}

func (c ConfigStub) Write() error {
	c["_written"] = "true"
	return nil
}

func (c ConfigStub) WriteHosts() error {
	return nil
}

func (c ConfigStub) DefaultHost() (string, error) {
	return "", nil
}

func (c ConfigStub) DefaultHostWithSource() (string, string, error) {
	return "", "", nil
}
