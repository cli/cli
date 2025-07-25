package test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
)

// Assert a boolean
func Assert(t *testing.T, result bool, message string) {
	t.Helper()
	if !result {
		t.Fatal(message)
	}
}

// AssertNil checks that an object is nil. Being a "boxed nil" (a nil value
// wrapped in a non-nil interface type) is not good enough.
func AssertNil(t *testing.T, obj interface{}, message string) {
	t.Helper()
	if obj != nil {
		t.Fatal(message)
	}
}

// AssertNotNil checks an object to be non-nil. Being a "boxed nil" (a nil value
// wrapped in a non-nil interface type) is not good enough.
// Note that there is a gap between AssertNil and AssertNotNil. Both fail when
// called with a boxed nil. This is intentional: we want to avoid boxed nils.
func AssertNotNil(t *testing.T, obj interface{}, message string) {
	t.Helper()
	if obj == nil {
		t.Fatal(message)
	}
	switch reflect.TypeOf(obj).Kind() {
	// .IsNil() only works on chan, func, interface, map, pointer, and slice.
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if reflect.ValueOf(obj).IsNil() {
			t.Fatal(message)
		}
	}
}

// AssertBoxedNil checks that an inner object is nil. This is intentional for
// testing purposes only.
func AssertBoxedNil(t *testing.T, obj interface{}, message string) {
	t.Helper()
	typ := reflect.TypeOf(obj).Kind()
	switch typ {
	// .IsNil() only works on chan, func, interface, map, pointer, and slice.
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if !reflect.ValueOf(obj).IsNil() {
			t.Fatal(message)
		}
	default:
		t.Fatalf("Cannot check type \"%s\". Needs to be of type chan, func, interface, map, pointer, or slice.", typ)
	}
}

// AssertNotError checks that err is nil
func AssertNotError(t *testing.T, err error, message string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %s", message, err)
	}
}

// AssertError checks that err is non-nil
func AssertError(t *testing.T, err error, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error but received none", message)
	}
}

// AssertErrorWraps checks that err can be unwrapped into the given target.
// NOTE: Has the side effect of actually performing that unwrapping.
func AssertErrorWraps(t *testing.T, err error, target interface{}) {
	t.Helper()
	if !errors.As(err, target) {
		t.Fatalf("error does not wrap an error of the expected type: %q !> %+T", err.Error(), target)
	}
}

// AssertErrorIs checks that err wraps the given error
func AssertErrorIs(t *testing.T, err error, target error) {
	t.Helper()

	if err == nil {
		t.Fatal("err was unexpectedly nil and should not have been")
	}

	if !errors.Is(err, target) {
		t.Fatalf("error does not wrap expected error: %q !> %q", err.Error(), target.Error())
	}
}

// AssertEquals uses the equality operator (==) to measure one and two
func AssertEquals(t *testing.T, one interface{}, two interface{}) {
	t.Helper()
	if reflect.TypeOf(one) != reflect.TypeOf(two) {
		t.Fatalf("cannot test equality of different types: %T != %T", one, two)
	}
	if one != two {
		t.Fatalf("%#v != %#v", one, two)
	}
}

// AssertDeepEquals uses the reflect.DeepEqual method to measure one and two
func AssertDeepEquals(t *testing.T, one interface{}, two interface{}) {
	t.Helper()
	if !reflect.DeepEqual(one, two) {
		t.Fatalf("[%#v] !(deep)= [%#v]", one, two)
	}
}

// AssertMarshaledEquals marshals one and two to JSON, and then uses
// the equality operator to measure them
func AssertMarshaledEquals(t *testing.T, one interface{}, two interface{}) {
	t.Helper()
	oneJSON, err := json.Marshal(one)
	AssertNotError(t, err, "Could not marshal 1st argument")
	twoJSON, err := json.Marshal(two)
	AssertNotError(t, err, "Could not marshal 2nd argument")

	if !bytes.Equal(oneJSON, twoJSON) {
		t.Fatalf("[%s] !(json)= [%s]", oneJSON, twoJSON)
	}
}

// AssertUnmarshaledEquals unmarshals two JSON strings (got and expected) to
// a map[string]interface{} and then uses reflect.DeepEqual to check they are
// the same
func AssertUnmarshaledEquals(t *testing.T, got, expected string) {
	t.Helper()
	var gotMap, expectedMap map[string]interface{}
	err := json.Unmarshal([]byte(got), &gotMap)
	AssertNotError(t, err, "Could not unmarshal 'got'")
	err = json.Unmarshal([]byte(expected), &expectedMap)
	AssertNotError(t, err, "Could not unmarshal 'expected'")
	if len(gotMap) != len(expectedMap) {
		t.Errorf("Expected %d keys, but got %d", len(expectedMap), len(gotMap))
	}
	for k, v := range expectedMap {
		if !reflect.DeepEqual(v, gotMap[k]) {
			t.Errorf("Field %q: Expected \"%v\", got \"%v\"", k, v, gotMap[k])
		}
	}
}

// AssertNotEquals uses the equality operator to measure that one and two
// are different
func AssertNotEquals(t *testing.T, one interface{}, two interface{}) {
	t.Helper()
	if one == two {
		t.Fatalf("%#v == %#v", one, two)
	}
}

// AssertByteEquals uses bytes.Equal to measure one and two for equality.
func AssertByteEquals(t *testing.T, one []byte, two []byte) {
	t.Helper()
	if !bytes.Equal(one, two) {
		t.Fatalf("Byte [%s] != [%s]",
			base64.StdEncoding.EncodeToString(one),
			base64.StdEncoding.EncodeToString(two))
	}
}

// AssertContains determines whether needle can be found in haystack
func AssertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("String [%s] does not contain [%s]", haystack, needle)
	}
}

// AssertNotContains determines if needle is not found in haystack
func AssertNotContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("String [%s] contains [%s]", haystack, needle)
	}
}

// AssertSliceContains determines if needle can be found in haystack
func AssertSliceContains[T comparable](t *testing.T, haystack []T, needle T) {
	t.Helper()
	for _, item := range haystack {
		if item == needle {
			return
		}
	}
	t.Fatalf("Slice %v does not contain %v", haystack, needle)
}

// AssertMetricWithLabelsEquals determines whether the value held by a prometheus Collector
// (e.g. Gauge, Counter, CounterVec, etc) is equal to the expected float64.
// In order to make useful assertions about just a subset of labels (e.g. for a
// CounterVec with fields "host" and "valid", being able to assert that two
// "valid": "true" increments occurred, without caring which host was tagged in
// each), takes a set of labels and ignores any metrics which have different
// label values.
// Only works for simple metrics (Counters and Gauges), or for the *count*
// (not value) of data points in a Histogram.
func AssertMetricWithLabelsEquals(t *testing.T, c prometheus.Collector, l prometheus.Labels, expected float64) {
	t.Helper()
	ch := make(chan prometheus.Metric)
	done := make(chan struct{})
	go func() {
		c.Collect(ch)
		close(done)
	}()
	var total float64
	timeout := time.After(time.Second)
loop:
	for {
	metric:
		select {
		case <-timeout:
			t.Fatal("timed out collecting metrics")
		case <-done:
			break loop
		case m := <-ch:
			var iom io_prometheus_client.Metric
			_ = m.Write(&iom)
			for _, lp := range iom.Label {
				// If any of the labels on this metric have the same name as but
				// different value than a label in `l`, skip this metric.
				val, ok := l[lp.GetName()]
				if ok && lp.GetValue() != val {
					break metric
				}
			}
			// Exactly one of the Counter, Gauge, or Histogram values will be set by
			// the .Write() operation, so add them all because the others will be 0.
			total += iom.Counter.GetValue()
			total += iom.Gauge.GetValue()
			total += float64(iom.Histogram.GetSampleCount())
		}
	}
	if total != expected {
		t.Errorf("metric with labels %+v: got %g, want %g", l, total, expected)
	}
}
