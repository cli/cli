package browser

type Stub struct {
	urls []string
}

func (b *Stub) Browse(url string) error {
	b.urls = append(b.urls, url)
	return nil
}

func (b *Stub) BrowsedURL() string {
	if len(b.urls) > 0 {
		return b.urls[0]
	}
	return ""
}

type _testing interface {
	Errorf(string, ...interface{})
	Helper()
}

func (b *Stub) Verify(t _testing, url string) {
	t.Helper()
	if url != "" {
		switch len(b.urls) {
		case 0:
			t.Errorf("expected browser to open URL %q, but it was never invoked", url)
		case 1:
			if url != b.urls[0] {
				t.Errorf("expected browser to open URL %q, got %q", url, b.urls[0])
			}
		default:
			t.Errorf("expected browser to open one URL, but was invoked %d times", len(b.urls))
		}
	} else if len(b.urls) > 0 {
		t.Errorf("expected no browser to open, but was invoked %d times: %v", len(b.urls), b.urls)
	}
}
