package listcmd_test

import (
	"errors"
	"net/http"
	"testing"

	listcmd "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func TestNewCmdSponsors(t *testing.T) {
	tests := []struct {
		name            string
		args            string
		expectedErr     error
		expectedOptions listcmd.Options
	}{
		{
			name:        "when no arguments provided, returns a useful error",
			args:        "",
			expectedErr: cmdutil.FlagErrorf("must specify a user"),
		},
		{
			name: "org",
			args: "testusername",
			expectedOptions: listcmd.Options{
				User: "testusername",
			},
		},
		{
			name:        "when too many arguments provided, returns a useful error",
			args:        "foo bar",
			expectedErr: cmdutil.FlagErrorf("too many arguments"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{
				HttpClient: func() (*http.Client, error) {
					return nil, nil
				},
			}

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)

			var optsSpy listcmd.Options
			cmd := listcmd.NewCmdList(f, func(opts listcmd.Options) error {
				optsSpy = opts
				return nil
			})
			cmd.SetArgs(argv)

			_, err = cmd.ExecuteC()
			require.Equal(t, tt.expectedErr, err)
			require.Equal(t, tt.expectedOptions.User, optsSpy.User)
		})
	}
}

type fakeSponsorLister struct {
	stubbedSponsors map[listcmd.User][]listcmd.Sponsor
	stubbedErr      error
}

func (s fakeSponsorLister) ListSponsors(user listcmd.User) ([]listcmd.Sponsor, error) {
	if s.stubbedErr != nil {
		return nil, s.stubbedErr
	}
	return s.stubbedSponsors[user], nil
}

type spySponsorTableRenderer struct {
	spiedSponsors []listcmd.Sponsor
}

func (s *spySponsorTableRenderer) Render(sponsors []listcmd.Sponsor) error {
	s.spiedSponsors = sponsors
	return nil
}

func TestListSponsorsRendersRetrievedSponsors(t *testing.T) {
	// Given our sponsor lister retrieves some sponsors
	sponsorListRendererSpy := &spySponsorTableRenderer{}
	listOptions := listcmd.Options{
		SponsorLister: fakeSponsorLister{
			stubbedSponsors: map[listcmd.User][]listcmd.Sponsor{
				"testusername": {"sponsor1", "sponsor2"},
			},
		},
		SponsorListRenderer: sponsorListRendererSpy,
		User:                "testusername",
	}

	// When I run the list command
	err := listcmd.ListRun(listOptions)

	// Then it is successful
	require.NoError(t, err)

	// And it renders the list
	require.Equal(t, sponsorListRendererSpy.spiedSponsors, []listcmd.Sponsor{"sponsor1", "sponsor2"})
}

func TestListSponsorsWrapsErrorRetrievingSponsors(t *testing.T) {
	// Given our sponsor lister returns with an error
	listOptions := listcmd.Options{
		SponsorLister: fakeSponsorLister{
			stubbedErr: errors.New("expected test error"),
		},
		User: "testusername",
	}

	// When I run the list command
	err := listcmd.ListRun(listOptions)

	// Then it returns an informational error
	require.ErrorContains(t, err, "sponsor list: expected test error")
}
