package ctconfig

import (
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/test"
)

func TestTemporalSetup(t *testing.T) {
	for _, tc := range []struct {
		ts  TemporalSet
		err string
	}{
		{
			ts:  TemporalSet{},
			err: "Name cannot be empty",
		},
		{
			ts: TemporalSet{
				Name: "temporal set",
			},
			err: "temporal set contains no shards",
		},
		{
			ts: TemporalSet{
				Name: "temporal set",
				Shards: []LogShard{
					{
						WindowStart: time.Time{},
						WindowEnd:   time.Time{},
					},
				},
			},
			err: "WindowStart must be before WindowEnd",
		},
		{
			ts: TemporalSet{
				Name: "temporal set",
				Shards: []LogShard{
					{
						WindowStart: time.Time{}.Add(time.Hour),
						WindowEnd:   time.Time{},
					},
				},
			},
			err: "WindowStart must be before WindowEnd",
		},
		{
			ts: TemporalSet{
				Name: "temporal set",
				Shards: []LogShard{
					{
						WindowStart: time.Time{},
						WindowEnd:   time.Time{}.Add(time.Hour),
					},
				},
			},
			err: "",
		},
	} {
		err := tc.ts.Setup()
		if err != nil && tc.err != err.Error() {
			t.Errorf("got error %q, wanted %q", err, tc.err)
		} else if err == nil && tc.err != "" {
			t.Errorf("unexpected error %q", err)
		}
	}
}

func TestLogInfo(t *testing.T) {
	ld := LogDescription{
		URI: "basic-uri",
		Key: "basic-key",
	}
	uri, key, err := ld.Info(time.Time{})
	test.AssertNotError(t, err, "Info failed")
	test.AssertEquals(t, uri, ld.URI)
	test.AssertEquals(t, key, ld.Key)

	fc := clock.NewFake()
	ld.TemporalSet = &TemporalSet{}
	_, _, err = ld.Info(fc.Now())
	test.AssertError(t, err, "Info should fail with a TemporalSet with no viable shards")
	ld.TemporalSet.Shards = []LogShard{{WindowStart: fc.Now().Add(time.Hour), WindowEnd: fc.Now().Add(time.Hour * 2)}}
	_, _, err = ld.Info(fc.Now())
	test.AssertError(t, err, "Info should fail with a TemporalSet with no viable shards")

	fc.Add(time.Hour * 4)
	now := fc.Now()
	ld.TemporalSet.Shards = []LogShard{
		{
			WindowStart: now.Add(time.Hour * -4),
			WindowEnd:   now.Add(time.Hour * -2),
			URI:         "a",
			Key:         "a",
		},
		{
			WindowStart: now.Add(time.Hour * -2),
			WindowEnd:   now.Add(time.Hour * 2),
			URI:         "b",
			Key:         "b",
		},
		{
			WindowStart: now.Add(time.Hour * 2),
			WindowEnd:   now.Add(time.Hour * 4),
			URI:         "c",
			Key:         "c",
		},
	}
	uri, key, err = ld.Info(now)
	test.AssertNotError(t, err, "Info failed")
	test.AssertEquals(t, uri, "b")
	test.AssertEquals(t, key, "b")
}
