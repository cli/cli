package must

import (
	"net/url"
	"testing"
)

func TestDo(t *testing.T) {
	url := Do(url.Parse("http://example.com"))
	if url.Host != "example.com" {
		t.Errorf("expected host to be example.com, got %s", url.Host)
	}
}
