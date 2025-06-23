package wfe2

import (
	"net/http"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/web"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRequiredStale(t *testing.T) {
	testCases := []struct {
		name           string
		req            *http.Request
		logEvent       *web.RequestEvent
		expectRequired bool
	}{
		{
			name:           "not GET",
			req:            &http.Request{Method: http.MethodPost},
			logEvent:       &web.RequestEvent{},
			expectRequired: false,
		},
		{
			name:           "GET, not getAPIPrefix",
			req:            &http.Request{Method: http.MethodGet},
			logEvent:       &web.RequestEvent{},
			expectRequired: false,
		},
		{
			name:           "GET, getAPIPrefix",
			req:            &http.Request{Method: http.MethodGet},
			logEvent:       &web.RequestEvent{Endpoint: getAPIPrefix + "whatever"},
			expectRequired: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test.AssertEquals(t, requiredStale(tc.req, tc.logEvent), tc.expectRequired)
		})
	}
}

func TestSaleEnoughToGETOrder(t *testing.T) {
	fc := clock.NewFake()
	wfe := WebFrontEndImpl{clk: fc, staleTimeout: time.Minute * 30}
	fc.Add(time.Hour * 24)
	created := fc.Now()
	fc.Add(time.Hour)
	prob := wfe.staleEnoughToGETOrder(&corepb.Order{
		Created: timestamppb.New(created),
	})
	test.Assert(t, prob == nil, "wfe.staleEnoughToGETOrder returned a non-nil problem")
}

func TestStaleEnoughToGETAuthzDeactivated(t *testing.T) {
	fc := clock.NewFake()
	wfe := WebFrontEndImpl{
		clk:                          fc,
		staleTimeout:                 time.Minute * 30,
		pendingAuthorizationLifetime: 7 * 24 * time.Hour,
		authorizationLifetime:        30 * 24 * time.Hour,
	}
	fc.Add(time.Hour * 24)
	expires := fc.Now().Add(wfe.authorizationLifetime)
	fc.Add(time.Hour)
	prob := wfe.staleEnoughToGETAuthz(&corepb.Authorization{
		Status:  string(core.StatusDeactivated),
		Expires: timestamppb.New(expires),
	})
	test.Assert(t, prob == nil, "wfe.staleEnoughToGETOrder returned a non-nil problem")
}
