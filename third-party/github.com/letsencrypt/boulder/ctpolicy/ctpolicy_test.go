package ctpolicy

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/ctpolicy/loglist"
	berrors "github.com/letsencrypt/boulder/errors"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	pubpb "github.com/letsencrypt/boulder/publisher/proto"
	"github.com/letsencrypt/boulder/test"
)

type mockPub struct{}

func (mp *mockPub) SubmitToSingleCTWithResult(_ context.Context, _ *pubpb.Request, _ ...grpc.CallOption) (*pubpb.Result, error) {
	return &pubpb.Result{Sct: []byte{0}}, nil
}

type mockFailPub struct{}

func (mp *mockFailPub) SubmitToSingleCTWithResult(_ context.Context, _ *pubpb.Request, _ ...grpc.CallOption) (*pubpb.Result, error) {
	return nil, errors.New("BAD")
}

type mockSlowPub struct{}

func (mp *mockSlowPub) SubmitToSingleCTWithResult(ctx context.Context, _ *pubpb.Request, _ ...grpc.CallOption) (*pubpb.Result, error) {
	<-ctx.Done()
	return nil, errors.New("timed out")
}

func TestGetSCTs(t *testing.T) {
	expired, cancel := context.WithDeadline(context.Background(), time.Now())
	defer cancel()
	missingSCTErr := berrors.MissingSCTs
	testCases := []struct {
		name       string
		mock       pubpb.PublisherClient
		logs       loglist.List
		ctx        context.Context
		result     core.SCTDERs
		expectErr  string
		berrorType *berrors.ErrorType
	}{
		{
			name: "basic success case",
			mock: &mockPub{},
			logs: loglist.List{
				{Name: "LogA1", Operator: "OperA", Url: "UrlA1", Key: []byte("KeyA1")},
				{Name: "LogA2", Operator: "OperA", Url: "UrlA2", Key: []byte("KeyA2")},
				{Name: "LogB1", Operator: "OperB", Url: "UrlB1", Key: []byte("KeyB1")},
				{Name: "LogC1", Operator: "OperC", Url: "UrlC1", Key: []byte("KeyC1")},
			},
			ctx:    context.Background(),
			result: core.SCTDERs{[]byte{0}, []byte{0}},
		},
		{
			name: "basic failure case",
			mock: &mockFailPub{},
			logs: loglist.List{
				{Name: "LogA1", Operator: "OperA", Url: "UrlA1", Key: []byte("KeyA1")},
				{Name: "LogA2", Operator: "OperA", Url: "UrlA2", Key: []byte("KeyA2")},
				{Name: "LogB1", Operator: "OperB", Url: "UrlB1", Key: []byte("KeyB1")},
				{Name: "LogC1", Operator: "OperC", Url: "UrlC1", Key: []byte("KeyC1")},
			},
			ctx:        context.Background(),
			expectErr:  "failed to get 2 SCTs, got 4 error(s)",
			berrorType: &missingSCTErr,
		},
		{
			name: "parent context timeout failure case",
			mock: &mockSlowPub{},
			logs: loglist.List{
				{Name: "LogA1", Operator: "OperA", Url: "UrlA1", Key: []byte("KeyA1")},
				{Name: "LogA2", Operator: "OperA", Url: "UrlA2", Key: []byte("KeyA2")},
				{Name: "LogB1", Operator: "OperB", Url: "UrlB1", Key: []byte("KeyB1")},
				{Name: "LogC1", Operator: "OperC", Url: "UrlC1", Key: []byte("KeyC1")},
			},
			ctx:        expired,
			expectErr:  "failed to get 2 SCTs before ctx finished",
			berrorType: &missingSCTErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctp := New(tc.mock, tc.logs, nil, nil, 0, blog.NewMock(), metrics.NoopRegisterer)
			ret, err := ctp.GetSCTs(tc.ctx, []byte{0}, time.Time{})
			if tc.result != nil {
				test.AssertDeepEquals(t, ret, tc.result)
			} else if tc.expectErr != "" {
				if !strings.Contains(err.Error(), tc.expectErr) {
					t.Errorf("Error %q did not match expected %q", err, tc.expectErr)
				}
				if tc.berrorType != nil {
					test.AssertErrorIs(t, err, *tc.berrorType)
				}
			}
		})
	}
}

type mockFailOnePub struct {
	badURL string
}

func (mp *mockFailOnePub) SubmitToSingleCTWithResult(_ context.Context, req *pubpb.Request, _ ...grpc.CallOption) (*pubpb.Result, error) {
	if req.LogURL == mp.badURL {
		return nil, errors.New("BAD")
	}
	return &pubpb.Result{Sct: []byte{0}}, nil
}

func TestGetSCTsMetrics(t *testing.T) {
	ctp := New(&mockFailOnePub{badURL: "UrlA1"}, loglist.List{
		{Name: "LogA1", Operator: "OperA", Url: "UrlA1", Key: []byte("KeyA1")},
		{Name: "LogB1", Operator: "OperB", Url: "UrlB1", Key: []byte("KeyB1")},
		{Name: "LogC1", Operator: "OperC", Url: "UrlC1", Key: []byte("KeyC1")},
	}, nil, nil, 0, blog.NewMock(), metrics.NoopRegisterer)
	_, err := ctp.GetSCTs(context.Background(), []byte{0}, time.Time{})
	test.AssertNotError(t, err, "GetSCTs failed")
	test.AssertMetricWithLabelsEquals(t, ctp.winnerCounter, prometheus.Labels{"url": "UrlB1", "result": succeeded}, 1)
	test.AssertMetricWithLabelsEquals(t, ctp.winnerCounter, prometheus.Labels{"url": "UrlC1", "result": succeeded}, 1)
}

func TestGetSCTsFailMetrics(t *testing.T) {
	// Ensure the proper metrics are incremented when GetSCTs fails.
	ctp := New(&mockFailOnePub{badURL: "UrlA1"}, loglist.List{
		{Name: "LogA1", Operator: "OperA", Url: "UrlA1", Key: []byte("KeyA1")},
	}, nil, nil, 0, blog.NewMock(), metrics.NoopRegisterer)
	_, err := ctp.GetSCTs(context.Background(), []byte{0}, time.Time{})
	test.AssertError(t, err, "GetSCTs should have failed")
	test.AssertErrorIs(t, err, berrors.MissingSCTs)
	test.AssertMetricWithLabelsEquals(t, ctp.winnerCounter, prometheus.Labels{"url": "UrlA1", "result": failed}, 1)

	// Ensure the proper metrics are incremented when GetSCTs times out.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ctp = New(&mockSlowPub{}, loglist.List{
		{Name: "LogA1", Operator: "OperA", Url: "UrlA1", Key: []byte("KeyA1")},
	}, nil, nil, 0, blog.NewMock(), metrics.NoopRegisterer)
	_, err = ctp.GetSCTs(ctx, []byte{0}, time.Time{})
	test.AssertError(t, err, "GetSCTs should have timed out")
	test.AssertErrorIs(t, err, berrors.MissingSCTs)
	test.AssertContains(t, err.Error(), context.DeadlineExceeded.Error())
	test.AssertMetricWithLabelsEquals(t, ctp.winnerCounter, prometheus.Labels{"url": "UrlA1", "result": failed}, 1)
}

func TestLogListMetrics(t *testing.T) {
	fc := clock.NewFake()
	Tomorrow := fc.Now().Add(24 * time.Hour)
	NextWeek := fc.Now().Add(7 * 24 * time.Hour)

	// Multiple operator groups with configured logs.
	ctp := New(&mockPub{}, loglist.List{
		{Name: "LogA1", Operator: "OperA", Url: "UrlA1", Key: []byte("KeyA1"), EndExclusive: Tomorrow},
		{Name: "LogA2", Operator: "OperA", Url: "UrlA2", Key: []byte("KeyA2"), EndExclusive: NextWeek},
		{Name: "LogB1", Operator: "OperB", Url: "UrlB1", Key: []byte("KeyB1"), EndExclusive: Tomorrow},
	}, nil, nil, 0, blog.NewMock(), metrics.NoopRegisterer)
	test.AssertMetricWithLabelsEquals(t, ctp.shardExpiryGauge, prometheus.Labels{"operator": "OperA", "logID": "LogA1"}, 86400)
	test.AssertMetricWithLabelsEquals(t, ctp.shardExpiryGauge, prometheus.Labels{"operator": "OperA", "logID": "LogA2"}, 604800)
	test.AssertMetricWithLabelsEquals(t, ctp.shardExpiryGauge, prometheus.Labels{"operator": "OperB", "logID": "LogB1"}, 86400)
}

func TestCompliantSet(t *testing.T) {
	for _, tc := range []struct {
		name    string
		results []result
		want    core.SCTDERs
	}{
		{
			name:    "nil input",
			results: nil,
			want:    nil,
		},
		{
			name:    "zero length input",
			results: []result{},
			want:    nil,
		},
		{
			name: "only one result",
			results: []result{
				{log: loglist.Log{Operator: "A", Tiled: false}, sct: []byte("sct1")},
			},
			want: nil,
		},
		{
			name: "only one good result",
			results: []result{
				{log: loglist.Log{Operator: "A", Tiled: false}, sct: []byte("sct1")},
				{log: loglist.Log{Operator: "B", Tiled: false}, err: errors.New("oops")},
			},
			want: nil,
		},
		{
			name: "only one operator",
			results: []result{
				{log: loglist.Log{Operator: "A", Tiled: false}, sct: []byte("sct1")},
				{log: loglist.Log{Operator: "A", Tiled: false}, sct: []byte("sct2")},
			},
			want: nil,
		},
		{
			name: "all tiled",
			results: []result{
				{log: loglist.Log{Operator: "A", Tiled: true}, sct: []byte("sct1")},
				{log: loglist.Log{Operator: "B", Tiled: true}, sct: []byte("sct2")},
			},
			want: nil,
		},
		{
			name: "happy path",
			results: []result{
				{log: loglist.Log{Operator: "A", Tiled: false}, err: errors.New("oops")},
				{log: loglist.Log{Operator: "A", Tiled: true}, sct: []byte("sct2")},
				{log: loglist.Log{Operator: "A", Tiled: false}, sct: []byte("sct3")},
				{log: loglist.Log{Operator: "B", Tiled: false}, err: errors.New("oops")},
				{log: loglist.Log{Operator: "B", Tiled: true}, sct: []byte("sct4")},
				{log: loglist.Log{Operator: "B", Tiled: false}, sct: []byte("sct6")},
				{log: loglist.Log{Operator: "C", Tiled: false}, err: errors.New("oops")},
				{log: loglist.Log{Operator: "C", Tiled: true}, sct: []byte("sct8")},
				{log: loglist.Log{Operator: "C", Tiled: false}, sct: []byte("sct9")},
			},
			// The second and sixth results should be picked, because first and fourth
			// are skipped for being errors, and fifth is skipped for also being tiled.
			want: core.SCTDERs{[]byte("sct2"), []byte("sct6")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := compliantSet(tc.results)
			if len(got) != len(tc.want) {
				t.Fatalf("compliantSet(%#v) returned %d SCTs, but want %d", tc.results, len(got), len(tc.want))
			}
			for i, sct := range tc.want {
				if !bytes.Equal(got[i], sct) {
					t.Errorf("compliantSet(%#v) returned unexpected SCT at index %d", tc.results, i)
				}
			}
		})
	}
}
