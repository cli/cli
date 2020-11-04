package cmdutil

import "testing"

func TestMinimumArgs(t *testing.T) {
	tests := []struct {
		N    int
		Args []string
	}{
		{
			N:    1,
			Args: []string{"v1.2.3"},
		},
		{
			N:    2,
			Args: []string{"v1.2.3", "cli/cli"},
		},
	}

	for _, test := range tests {
		if got := MinimumArgs(test.N, "")(nil, test.Args); got != nil {
			t.Errorf("Got: %v, Want: (nil)", got)
		}
	}
}

func TestMinimumNs_with_error(t *testing.T) {
	tests := []struct {
		N             int
		CustomMessage string
		WantMessage   string
	}{
		{
			N:             1,
			CustomMessage: "A custom msg",
			WantMessage:   "A custom msg",
		},
		{
			N:             1,
			CustomMessage: "",
			WantMessage:   "requires at least 1 arg(s), only received 0",
		},
	}

	for _, test := range tests {
		if got := MinimumArgs(test.N, test.CustomMessage)(nil, nil); got.Error() != test.WantMessage {
			t.Errorf("Got: %v, Want: %v", got, test.WantMessage)
		}
	}
}
