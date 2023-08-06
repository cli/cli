package shared

import (
	"errors"
)

type Visibility string

const (
	All      = "all"
	Private  = "private"
	Selected = "selected"
)

type VariableEntity string

const (
	Repository   = "repository"
	Organization = "organization"
	Environment  = "environment"
)

func GetVariableEntity(orgName, envName string) (VariableEntity, error) {
	orgSet := orgName != ""
	envSet := envName != ""

	if orgSet && envSet {
		return "", errors.New("cannot specify multiple variable entities")
	}

	if orgSet {
		return Organization, nil
	}
	if envSet {
		return Environment, nil
	}
	return Repository, nil
}
