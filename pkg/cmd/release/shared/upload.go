package shared

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
)

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

func ConcurrentUpload(httpClient *http.Client, uploadURL string, numWorkers int, assets []*AssetForUpload) error {
	if numWorkers == 0 {
		return errors.New("the number of concurrent workers needs to be greater than 0")
	}

	jobs := make(chan AssetForUpload, len(assets))
	results := make(chan error, len(assets))

	if len(assets) < numWorkers {
		numWorkers = len(assets)
	}

	for w := 1; w <= numWorkers; w++ {
		go func() {
			for a := range jobs {
				results <- uploadWithDelete(httpClient, uploadURL, a)
			}
		}()
	}

	for _, a := range assets {
		jobs <- *a
	}
	close(jobs)

	var uploadError error
	for i := 0; i < len(assets); i++ {
		if err := <-results; err != nil {
			uploadError = err
		}
	}
	return uploadError
}

const maxRetries = 3

func uploadWithDelete(httpClient *http.Client, uploadURL string, a AssetForUpload) error {
	if a.ExistingURL != "" {
		err := deleteAsset(httpClient, a.ExistingURL)
		if err != nil {
			return err
		}
	}

	retries := 0
	for {
		var httpError api.HTTPError
		_, err := uploadAsset(httpClient, uploadURL, a)
		// retry upload several times upon receiving HTTP 5xx
		if err == nil || !errors.As(err, &httpError) || httpError.StatusCode < 500 || retries < maxRetries {
			return err
		}
		retries++
		time.Sleep(time.Duration(retries) * time.Second)
	}
}

func uploadAsset(httpClient *http.Client, uploadURL string, asset AssetForUpload) (*ReleaseAsset, error) {
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

	req, err := http.NewRequest("POST", u.String(), f)
	if err != nil {
		return nil, err
	}
	req.ContentLength = asset.Size
	req.Header.Set("Content-Type", asset.MIMEType)
	req.GetBody = asset.Open

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var newAsset ReleaseAsset
	err = json.Unmarshal(b, &newAsset)
	if err != nil {
		return nil, err
	}

	return &newAsset, nil
}

func deleteAsset(httpClient *http.Client, assetURL string) error {
	req, err := http.NewRequest("DELETE", assetURL, nil)
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
