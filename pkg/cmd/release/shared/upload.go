package shared

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/api"
	"golang.org/x/sync/errgroup"
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type errNetwork struct{ error }

type AssetForUpload struct {
	Name  string
	Label string

	Size     int64
	MIMEType string
	Open     func() (io.ReadCloser, error)

	ExistingURL string
}

func AssetsFromArgs(args []string) (assets []*AssetForUpload, err error) {
	for _, arg := range args {
		var label string
		fn := arg
		if idx := strings.IndexRune(arg, '#'); idx > 0 {
			fn = arg[0:idx]
			label = arg[idx+1:]
		}

		var fi os.FileInfo
		fi, err = os.Stat(fn)
		if err != nil {
			return
		}

		assets = append(assets, &AssetForUpload{
			Open: func() (io.ReadCloser, error) {
				return os.Open(fn)
			},
			Size:     fi.Size(),
			Name:     fi.Name(),
			Label:    label,
			MIMEType: typeForFilename(fi.Name()),
		})
	}
	return
}

func typeForFilename(fn string) string {
	ext := fileExt(fn)
	switch ext {
	case ".zip":
		return "application/zip"
	case ".js":
		return "application/javascript"
	case ".tar":
		return "application/x-tar"
	case ".tgz", ".tar.gz":
		return "application/x-gtar"
	case ".bz2":
		return "application/x-bzip2"
	case ".dmg":
		return "application/x-apple-diskimage"
	case ".rpm":
		return "application/x-rpm"
	case ".deb":
		return "application/x-debian-package"
	}

	t := mime.TypeByExtension(ext)
	if t == "" {
		return "application/octet-stream"
	}
	return t
}

func fileExt(fn string) string {
	fn = strings.ToLower(fn)
	if strings.HasSuffix(fn, ".tar.gz") {
		return ".tar.gz"
	}
	return path.Ext(fn)
}

func ConcurrentUpload(httpClient httpDoer, uploadURL string, numWorkers int, assets []*AssetForUpload) error {
	if numWorkers == 0 {
		return errors.New("the number of concurrent workers needs to be greater than 0")
	}

	ctx := context.Background()
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(numWorkers)

	for _, a := range assets {
		asset := *a
		g.Go(func() error {
			return uploadWithDelete(gctx, httpClient, uploadURL, asset)
		})
	}

	return g.Wait()
}

func shouldRetry(err error) bool {
	var networkError errNetwork
	if errors.As(err, &networkError) {
		return true
	}
	var httpError api.HTTPError
	return errors.As(err, &httpError) && httpError.StatusCode >= 500
}

// Allow injecting backoff interval in tests.
var retryInterval = time.Millisecond * 200

func uploadWithDelete(ctx context.Context, httpClient httpDoer, uploadURL string, a AssetForUpload) error {
	if a.ExistingURL != "" {
		if err := deleteAsset(ctx, httpClient, a.ExistingURL); err != nil {
			return err
		}
	}
	bo := backoff.NewConstantBackOff(retryInterval)
	return backoff.Retry(func() error {
		_, err := uploadAsset(ctx, httpClient, uploadURL, a)
		if err == nil || shouldRetry(err) {
			return err
		}
		return backoff.Permanent(err)
	}, backoff.WithContext(backoff.WithMaxRetries(bo, 3), ctx))
}

func uploadAsset(ctx context.Context, httpClient httpDoer, uploadURL string, asset AssetForUpload) (*ReleaseAsset, error) {
	u, err := url.Parse(uploadURL)
	if err != nil {
		return nil, err
	}
	params := u.Query()
	params.Set("name", asset.Name)
	params.Set("label", asset.Label)
	u.RawQuery = params.Encode()

	f, err := asset.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), f)
	if err != nil {
		return nil, err
	}
	req.ContentLength = asset.Size
	req.Header.Set("Content-Type", asset.MIMEType)
	req.GetBody = asset.Open

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errNetwork{err}
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, api.HandleHTTPError(resp)
	}

	var newAsset ReleaseAsset
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&newAsset); err != nil {
		return nil, err
	}

	return &newAsset, nil
}

func deleteAsset(ctx context.Context, httpClient httpDoer, assetURL string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", assetURL, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return api.HandleHTTPError(resp)
	}

	return nil
}
