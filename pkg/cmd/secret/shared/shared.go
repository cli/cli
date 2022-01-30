package shared

import "strings"

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
	return strings.Title(app.String())
}

type SecretEntity string

const (
	Repository   = "repository"
	Organization = "organization"
	User         = "user"
	Environment  = "environment"
)

func GetSecretEntity(orgName, envName string, userSecrets bool) SecretEntity {
	if orgName != "" {
		return Organization
	}
	if envName != "" {
		return Environment
	}
	if userSecrets {
		return User
	}
	return Repository
}

func GetSecretApp(app string, entity SecretEntity) App {
	switch strings.ToLower(app) {
	case Actions:
		return Actions
	case Codespaces:
		return Codespaces
	case Dependabot:
		return Dependabot
	case "":
		if entity == User {
			return Codespaces
		}
		return Actions
	default:
		return Unknown
	}
}

func IsSupportedSecretEntity(app App, entity SecretEntity) bool {
	switch app {
	case Actions:
		return entity == Repository || entity == Organization || entity == Environment
	case Codespaces:
		return entity == User
	case Dependabot:
		return entity == Repository || entity == Organization
	default:
		return false
	}
}
