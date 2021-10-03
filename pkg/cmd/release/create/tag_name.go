package create

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
)

var (
	defaultVersionRegexp = regexp.MustCompile(`^(.*)(\d)+\.(\d)+\.(\d)+(.*)$`)
)

func getTagNameOptions(httpClient *http.Client, baseRepo ghrepo.Interface) (string, []string, error) {
	release, err := shared.FetchLatestRelease(httpClient, baseRepo)
	if errors.Is(err, shared.ErrReleaseNotFound) {
		return "", nil, nil
	}

	if err != nil {
		return "", nil, err
	}

	submatches := defaultVersionRegexp.FindStringSubmatch(release.TagName)
	if len(submatches) == 0 {
		return "", nil, nil
	}

	prefix := submatches[1]
	majorVersion, minorVersion, patchVersion := getVersionsFromTagNameSubmatches(submatches)
	suffix := submatches[5]

	options := []string{
		fmt.Sprintf("%s%d.%d.%d%s", prefix, majorVersion+1, minorVersion, patchVersion, suffix),
		fmt.Sprintf("%s%d.%d.%d%s", prefix, majorVersion, minorVersion+1, patchVersion, suffix),
		fmt.Sprintf("%s%d.%d.%d%s", prefix, majorVersion, minorVersion, patchVersion+1, suffix),
	}

	return release.TagName, options, nil
}

func getVersionsFromTagNameSubmatches(submatches []string) (int, int, int) {
	// In this point this is unliklely to fail but is best practice to avoid ignoring the errors from conversion
	majorVersion, err := strconv.Atoi(submatches[2])
	if err != nil {
		return -1, -1, -1
	}

	minorVersion, err := strconv.Atoi(submatches[3])
	if err != nil {
		return -1, -1, -1
	}

	patchVersion, err := strconv.Atoi(submatches[4])
	if err != nil {
		return -1, -1, -1
	}

	return majorVersion, minorVersion, patchVersion
}
