package github

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/github/gh-cli/ui"
	"github.com/github/gh-cli/utils"
)

const apiPayloadVersion = "application/vnd.github.v3+json;charset=utf-8"
const patchMediaType = "application/vnd.github.v3.patch;charset=utf-8"
const textMediaType = "text/plain;charset=utf-8"
const checksType = "application/vnd.github.antiope-preview+json;charset=utf-8"
const draftsType = "application/vnd.github.shadow-cat-preview+json;charset=utf-8"
const cacheVersion = 2

var inspectHeaders = []string{
	"Authorization",
	"X-GitHub-OTP",
	"Location",
	"Link",
	"Accept",
}

type verboseTransport struct {
	Transport   *http.Transport
	Verbose     bool
	OverrideURL *url.URL
	Out         io.Writer
	Colorized   bool
}

func (t *verboseTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if t.Verbose {
		t.dumpRequest(req)
	}

	if t.OverrideURL != nil {
		port := "80"
		if s := strings.Split(req.URL.Host, ":"); len(s) > 1 {
			port = s[1]
		}

		req = cloneRequest(req)
		req.Header.Set("X-Original-Scheme", req.URL.Scheme)
		req.Header.Set("X-Original-Port", port)
		req.Host = req.URL.Host
		req.URL.Scheme = t.OverrideURL.Scheme
		req.URL.Host = t.OverrideURL.Host
	}

	resp, err = t.Transport.RoundTrip(req)

	if err == nil && t.Verbose {
		t.dumpResponse(resp)
	}

	return
}

func (t *verboseTransport) dumpRequest(req *http.Request) {
	info := fmt.Sprintf("> %s %s://%s%s", req.Method, req.URL.Scheme, req.URL.Host, req.URL.RequestURI())
	t.verbosePrintln(info)
	t.dumpHeaders(req.Header, ">")
	body := t.dumpBody(req.Body)
	if body != nil {
		// reset body since it's been read
		req.Body = body
	}
}

func (t *verboseTransport) dumpResponse(resp *http.Response) {
	info := fmt.Sprintf("< HTTP %d", resp.StatusCode)
	t.verbosePrintln(info)
	t.dumpHeaders(resp.Header, "<")
	body := t.dumpBody(resp.Body)
	if body != nil {
		// reset body since it's been read
		resp.Body = body
	}
}

func (t *verboseTransport) dumpHeaders(header http.Header, indent string) {
	for _, listed := range inspectHeaders {
		for name, vv := range header {
			if !strings.EqualFold(name, listed) {
				continue
			}
			for _, v := range vv {
				if v != "" {
					r := regexp.MustCompile("(?i)^(basic|token) (.+)")
					if r.MatchString(v) {
						v = r.ReplaceAllString(v, "$1 [REDACTED]")
					}

					info := fmt.Sprintf("%s %s: %s", indent, name, v)
					t.verbosePrintln(info)
				}
			}
		}
	}
}

func (t *verboseTransport) dumpBody(body io.ReadCloser) io.ReadCloser {
	if body == nil {
		return nil
	}

	defer body.Close()
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, body)
	utils.Check(err)

	if buf.Len() > 0 {
		t.verbosePrintln(buf.String())
	}

	return ioutil.NopCloser(buf)
}

func (t *verboseTransport) verbosePrintln(msg string) {
	if t.Colorized {
		msg = fmt.Sprintf("\033[36m%s\033[0m", msg)
	}

	fmt.Fprintln(t.Out, msg)
}

func newHttpClient(testHost string, verbose bool, unixSocket string) *http.Client {
	var testURL *url.URL
	if testHost != "" {
		testURL, _ = url.Parse(testHost)
	}
	var httpTransport *http.Transport
	if unixSocket != "" {
		dialFunc := func(network, addr string) (net.Conn, error) {
			return net.Dial("unix", unixSocket)
		}
		dialContext := func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", unixSocket)
		}
		httpTransport = &http.Transport{
			DialContext:           dialContext,
			DialTLS:               dialFunc,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 10 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
		}
	} else {
		httpTransport = &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		}
	}
	tr := &verboseTransport{
		Transport:   httpTransport,
		Verbose:     verbose,
		OverrideURL: testURL,
		Out:         ui.Stderr,
		Colorized:   ui.IsTerminal(os.Stderr),
	}

	return &http.Client{
		Transport: tr,
	}
}

func cloneRequest(req *http.Request) *http.Request {
	dup := new(http.Request)
	*dup = *req
	dup.URL, _ = url.Parse(req.URL.String())
	dup.Header = make(http.Header)
	for k, s := range req.Header {
		dup.Header[k] = s
	}
	return dup
}

type simpleClient struct {
	httpClient     *http.Client
	rootUrl        *url.URL
	PrepareRequest func(*http.Request)
	CacheTTL       int
}

func (c *simpleClient) performRequest(method, path string, body io.Reader, configure func(*http.Request)) (*simpleResponse, error) {
	url, err := url.Parse(path)
	if err == nil {
		url = c.rootUrl.ResolveReference(url)
		return c.performRequestUrl(method, url, body, configure)
	} else {
		return nil, err
	}
}

func (c *simpleClient) performRequestUrl(method string, url *url.URL, body io.Reader, configure func(*http.Request)) (res *simpleResponse, err error) {
	req, err := http.NewRequest(method, url.String(), body)
	if err != nil {
		return
	}
	if c.PrepareRequest != nil {
		c.PrepareRequest(req)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", apiPayloadVersion)

	if configure != nil {
		configure(req)
	}

	key := cacheKey(req)
	if cachedResponse := c.cacheRead(key, req); cachedResponse != nil {
		res = &simpleResponse{cachedResponse}
		return
	}

	httpResponse, err := c.httpClient.Do(req)
	if err != nil {
		return
	}

	c.cacheWrite(key, httpResponse)
	res = &simpleResponse{httpResponse}

	return
}

func isGraphQL(req *http.Request) bool {
	return req.URL.Path == "/graphql"
}

func canCache(req *http.Request) bool {
	return strings.EqualFold(req.Method, "GET") || isGraphQL(req)
}

func (c *simpleClient) cacheRead(key string, req *http.Request) (res *http.Response) {
	if c.CacheTTL > 0 && canCache(req) {
		f := cacheFile(key)
		cacheInfo, err := os.Stat(f)
		if err != nil {
			return
		}
		if time.Since(cacheInfo.ModTime()).Seconds() > float64(c.CacheTTL) {
			return
		}
		cf, err := os.Open(f)
		if err != nil {
			return
		}
		defer cf.Close()

		cb, err := ioutil.ReadAll(cf)
		if err != nil {
			return
		}
		parts := strings.SplitN(string(cb), "\r\n\r\n", 2)
		if len(parts) < 2 {
			return
		}

		res = &http.Response{
			Body:   ioutil.NopCloser(bytes.NewBufferString(parts[1])),
			Header: http.Header{},
		}
		headerLines := strings.Split(parts[0], "\r\n")
		if len(headerLines) < 1 {
			return
		}
		if proto := strings.SplitN(headerLines[0], " ", 3); len(proto) >= 3 {
			res.Proto = proto[0]
			res.Status = fmt.Sprintf("%s %s", proto[1], proto[2])
			if code, _ := strconv.Atoi(proto[1]); code > 0 {
				res.StatusCode = code
			}
		}
		for _, line := range headerLines[1:] {
			kv := strings.SplitN(line, ":", 2)
			if len(kv) >= 2 {
				res.Header.Add(kv[0], strings.TrimLeft(kv[1], " "))
			}
		}
	}
	return
}

func (c *simpleClient) cacheWrite(key string, res *http.Response) {
	if c.CacheTTL > 0 && canCache(res.Request) && res.StatusCode < 500 && res.StatusCode != 403 {
		bodyCopy := &bytes.Buffer{}
		bodyReplacement := readCloserCallback{
			Reader: io.TeeReader(res.Body, bodyCopy),
			Closer: res.Body,
			Callback: func() {
				f := cacheFile(key)
				err := os.MkdirAll(filepath.Dir(f), 0771)
				if err != nil {
					return
				}
				cf, err := os.OpenFile(f, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
				if err != nil {
					return
				}
				defer cf.Close()
				fmt.Fprintf(cf, "%s %s\r\n", res.Proto, res.Status)
				res.Header.Write(cf)
				fmt.Fprintf(cf, "\r\n")
				io.Copy(cf, bodyCopy)
			},
		}
		res.Body = &bodyReplacement
	}
}

type readCloserCallback struct {
	Callback func()
	Closer   io.Closer
	io.Reader
}

func (rc *readCloserCallback) Close() error {
	err := rc.Closer.Close()
	if err == nil {
		rc.Callback()
	}
	return err
}

func cacheKey(req *http.Request) string {
	path := strings.Replace(req.URL.EscapedPath(), "/", "-", -1)
	if len(path) > 1 {
		path = strings.TrimPrefix(path, "-")
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	hash := md5.New()
	fmt.Fprintf(hash, "%d:", cacheVersion)
	io.WriteString(hash, req.Header.Get("Accept"))
	io.WriteString(hash, req.Header.Get("Authorization"))
	queryParts := strings.Split(req.URL.RawQuery, "&")
	sort.Strings(queryParts)
	for _, q := range queryParts {
		fmt.Fprintf(hash, "%s&", q)
	}
	if isGraphQL(req) && req.Body != nil {
		if b, err := ioutil.ReadAll(req.Body); err == nil {
			req.Body = ioutil.NopCloser(bytes.NewBuffer(b))
			hash.Write(b)
		}
	}
	return fmt.Sprintf("%s/%s_%x", host, path, hash.Sum(nil))
}

func cacheFile(key string) string {
	return path.Join(os.TempDir(), "hub", "api", key)
}

func (c *simpleClient) jsonRequest(method, path string, body interface{}, configure func(*http.Request)) (*simpleResponse, error) {
	json, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(json)

	return c.performRequest(method, path, buf, func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		if configure != nil {
			configure(req)
		}
	})
}

func (c *simpleClient) Get(path string) (*simpleResponse, error) {
	return c.performRequest("GET", path, nil, nil)
}

func (c *simpleClient) GetFile(path string, mimeType string) (*simpleResponse, error) {
	return c.performRequest("GET", path, nil, func(req *http.Request) {
		req.Header.Set("Accept", mimeType)
	})
}

func (c *simpleClient) PostJSON(path string, payload interface{}) (*simpleResponse, error) {
	return c.jsonRequest("POST", path, payload, nil)
}

func (c *simpleClient) PostJSONPreview(path string, payload interface{}, mimeType string) (*simpleResponse, error) {
	return c.jsonRequest("POST", path, payload, func(req *http.Request) {
		req.Header.Set("Accept", mimeType)
	})
}

func (c *simpleClient) PatchJSON(path string, payload interface{}) (*simpleResponse, error) {
	return c.jsonRequest("PATCH", path, payload, nil)
}

type simpleResponse struct {
	*http.Response
}

type errorInfo struct {
	Message  string       `json:"message"`
	Errors   []fieldError `json:"errors"`
	Response *http.Response
}
type errorInfoSimple struct {
	Message string   `json:"message"`
	Errors  []string `json:"errors"`
}
type fieldError struct {
	Resource string `json:"resource"`
	Message  string `json:"message"`
	Code     string `json:"code"`
	Field    string `json:"field"`
}

func (e *errorInfo) Error() string {
	return e.Message
}

func (res *simpleResponse) Unmarshal(dest interface{}) (err error) {
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	return json.Unmarshal(body, dest)
}

func (res *simpleResponse) ErrorInfo() (msg *errorInfo, err error) {
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	msg = &errorInfo{}
	err = json.Unmarshal(body, msg)
	if err != nil {
		msgSimple := &errorInfoSimple{}
		if err = json.Unmarshal(body, msgSimple); err == nil {
			msg.Message = msgSimple.Message
			for _, errMsg := range msgSimple.Errors {
				msg.Errors = append(msg.Errors, fieldError{
					Code:    "custom",
					Message: errMsg,
				})
			}
		}
	}
	if err == nil {
		msg.Response = res.Response
	}

	return
}

func (res *simpleResponse) Link(name string) string {
	linkVal := res.Header.Get("Link")
	re := regexp.MustCompile(`<([^>]+)>; rel="([^"]+)"`)
	for _, match := range re.FindAllStringSubmatch(linkVal, -1) {
		if match[2] == name {
			return match[1]
		}
	}
	return ""
}
