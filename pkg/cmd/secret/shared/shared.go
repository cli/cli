package shared

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/internal/text"
)

type Visibility string

const (
	All      = "all"
	Private  = "private"
	Selected = "selected"
)

type App string

const (
	Actions    = "actions"
	Codespaces = "codespaces"
	Dependabot = "dependabot"
	Unknown    = "unknown"
)

func (app App) String() string {
	return string(app)
}

func (app App) Title() string {
	return text.Title(app.String())
}

type SecretEntity string

const (
	Repository   = "repository"
	Organization = "organization"
	User         = "user"
	Environment  = "environment"
)

func GetSecretEntity(orgName, envName string, userSecrets bool) (SecretEntity, error) {
	orgSet := orgName != ""
	envSet := envName != ""

	if orgSet && envSet || orgSet && userSecrets || envSet && userSecrets {
		return "", errors.New("cannot specify multiple secret entities")
	}

	if orgSet {
		return Organization, nil
	}
	if envSet {
		return Environment, nil
	}
	if userSecrets {
		return User, nil
	}
	return Repository, nil
}

func GetSecretApp(app string, entity SecretEntity) (App, error) {
	switch strings.ToLower(app) {
	case Actions:
		return Actions, nil
	case Codespaces:
		return Codespaces, nil
	case Dependabot:
		return Dependabot, nil
	case "":
		if entity == User {
			return Codespaces, nil
		}
		return Actions, nil
	default:
		return Unknown, fmt.Errorf("invalid application: %s", app)
	}
}

func IsSupportedSecretEntity(app App, entity SecretEntity) bool {
	switch app {
	case Actions:
		return entity == Repository || entity == Organization || entity == Environment
	case Codespaces:
		return entity == User || entity == Organization || entity == Repository
	case Dependabot:
		return entity == Repository || entity == Organization
	default:
		return false
	}
}
