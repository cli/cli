package web

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
)

func TestNewServer(t *testing.T) {
	srv := NewServer(":0", nil, blog.NewMock())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := srv.ListenAndServe()
		test.Assert(t, errors.Is(err, http.ErrServerClosed), "Could not start server")
		wg.Done()
	}()

	err := srv.Shutdown(context.TODO())
	test.AssertNotError(t, err, "Could not shut down server")
	wg.Wait()
}

func TestUnorderedShutdownIsFine(t *testing.T) {
	srv := NewServer(":0", nil, blog.NewMock())
	err := srv.Shutdown(context.TODO())
	test.AssertNotError(t, err, "Could not shut down server")
	err = srv.ListenAndServe()
	test.Assert(t, errors.Is(err, http.ErrServerClosed), "Could not start server")
}
