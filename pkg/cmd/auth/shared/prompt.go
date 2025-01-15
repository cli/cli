package shared

type Prompt interface {
	Select(string, string, []string) (int, error)
	Confirm(string, bool) (bool, error)
	InputHostname() (string, error)
	AuthToken() (string, error)
	Input(string, string) (string, error)
	Password(string) (string, error)
}
