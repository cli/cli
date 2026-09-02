package attachments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/go-gh/v2/pkg/auth"
)

// Uploader attaches files to one upload target.
type Uploader struct {
	client           *api.Client
	host             string
	targetRepository int64
}

// NewUploader prepares uploads against targetRepository, the numeric REST id of
// the repository the assets are uploaded against, and viewerPermission, the
// GraphQL viewerPermission field on that same repository. It validates
// everything an upload needs, so a caller that gets an Uploader back can use
// it.
//
// The host is checked first. On an enterprise server no token and no permission
// makes an upload work, so any other order would name a fault the user cannot
// fix. The other checks are free to move.
func NewUploader(httpClient *http.Client, tokenType gh.TokenType, host string, targetRepository int64, viewerPermission string) (*Uploader, error) {
	if err := checkHost(host); err != nil {
		return nil, err
	}

	if err := checkUploadTokenType(tokenType); err != nil {
		return nil, err
	}

	if targetRepository <= 0 {
		return nil, errors.New("could not determine which repository to attach files to")
	}

	if err := checkPermission(viewerPermission); err != nil {
		return nil, err
	}

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return &Uploader{client: api.NewClientFromHTTP(httpClient), host: host, targetRepository: targetRepository}, nil
}

// checkHost rejects a host that cannot serve the upload endpoint. IsEnterprise
// is false for ghe.com tenants, so data residency keeps working.
func checkHost(host string) error {
	if auth.IsEnterprise(host) {
		return errors.New("attaching files is not supported on GitHub Enterprise Server")
	}
	return nil
}

// uploadTokenTypes lists the credentials that can attach a file.
var uploadTokenTypes = []gh.TokenType{
	gh.TokenTypeOAuth,
	gh.TokenTypePersonalAccess,
	gh.TokenTypeFineGrainedPAT,
}

// checkUploadTokenType rejects a credential that cannot upload. It is an
// allowlist, so an unlisted kind is rejected rather than sent.
func checkUploadTokenType(tokenType gh.TokenType) error {
	if slices.Contains(uploadTokenTypes, tokenType) {
		return nil
	}
	return errors.New("unsupported authentication type")
}

// uploadPermissions lists the repository permissions that can attach a file.
// The list matches api.Repository.ViewerCanPush, and it is measured against the
// upload endpoint: READ and TRIAGE get a 404.
var uploadPermissions = []string{"ADMIN", "MAINTAIN", "WRITE"}

// checkPermission rejects a permission that cannot upload. It is an allowlist,
// so an unlisted value is rejected rather than sent.
func checkPermission(viewerPermission string) error {
	// A caller that never requested the field hands over an empty string. That
	// is a different fault from a permission that is too low.
	if viewerPermission == "" {
		return errors.New("could not determine your permission on the repository to attach files")
	}

	if slices.Contains(uploadPermissions, viewerPermission) {
		return nil
	}
	return errors.New("attaching files requires write access to the repository")
}

// upload sends the file's bytes and returns the asset URL to reference
// from the markdown.
func (u *Uploader) upload(ctx context.Context, a UserAsset) (string, error) {
	assetURL, err := u.postAsset(ctx, a.getAsset())
	if err != nil {
		return "", newUploadError(err, a)
	}
	return assetURL, nil
}

// openFile is indirected so tests can stub file opening.
var openFile = func(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// postAsset does the request and reads the asset URL back. It takes the file
// rather than the UserAsset, because nothing about sending the bytes depends on
// how they render. It is separate so every way it can fail picks up the same
// explanation on the way out.
func (u *Uploader) postAsset(ctx context.Context, a asset) (string, error) {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.UserAssetUploadPrefix(u.host), "user-attachments", "assets")
	if err != nil {
		return "", err
	}
	url.SetQuery("name", filepath.Base(a.path))
	url.SetQuery("content_type", a.contentType)
	url.SetQuery("repository_id", strconv.FormatInt(u.targetRepository, 10))

	open := func() (io.ReadCloser, error) { return openFile(a.path) }

	f, err := open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", url.String(), f)
	if err != nil {
		return "", err
	}
	req.ContentLength = a.info.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", "application/vnd.github+json")
	// Without GetBody a redirect re-reads an exhausted reader.
	req.GetBody = open

	// The request is hand-built and sent with DoRequest rather than Request so
	// it can set ContentLength and GetBody, which Request cannot express.
	resp, err := u.client.DoRequest(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var asset struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&asset); err != nil {
		return "", err
	}
	if asset.URL == "" {
		return "", errors.New("the server returned no asset URL")
	}

	return asset.URL, nil
}

// newUploadError captures the status code so the message can name what the
// endpoint refused.
func newUploadError(err error, a UserAsset) error {
	uploadErr := &uploadError{Path: a.Path(), err: err}
	if httpError, ok := errors.AsType[api.HTTPError](err); ok {
		uploadErr.StatusCode = httpError.StatusCode
		uploadErr.Message = httpError.Message
		uploadErr.RetryAfter = httpError.Headers.Get("Retry-After")
	}
	return uploadErr
}

// uploadError reports a failed upload. StatusCode is zero when the request
// never reached the server.
type uploadError struct {
	Path       string
	StatusCode int
	RetryAfter string
	// Message is what the endpoint said, which api.HandleHTTPError has already
	// joined from the top level message and the per field ones.
	Message string
	err     error
}

func (e *uploadError) Error() string {
	switch e.StatusCode {
	case http.StatusNotFound:
		// The endpoint answers 404 rather than 403 when the token cannot write,
		// so the status code alone points at the wrong problem.
		return fmt.Sprintf("could not upload %s: attaching files requires write access to the repository", e.Path)
	case http.StatusUnprocessableEntity:
		if e.Message == "" {
			return fmt.Sprintf("could not upload %s", e.Path)
		}
		return fmt.Sprintf("could not upload %s: %s", e.Path, strings.ReplaceAll(e.Message, "\n", "; "))
	case http.StatusTooManyRequests:
		if e.RetryAfter == "" {
			return fmt.Sprintf("could not upload %s: rate limited; wait and try again", e.Path)
		}
		retryAfter := e.RetryAfter
		if seconds, err := strconv.Atoi(e.RetryAfter); err == nil {
			retryAfter = fmt.Sprintf("%d seconds", seconds)
		}
		return fmt.Sprintf("could not upload %s: rate limited; retry after %s", e.Path, retryAfter)
	}
	return fmt.Sprintf("failed to upload %s: %v", e.Path, e.err)
}

func (e *uploadError) Unwrap() error {
	return e.err
}
