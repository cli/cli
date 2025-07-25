/*
 *
 * Copyright 2018 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/resolver"

	"github.com/letsencrypt/boulder/grpc/internal/leakcheck"
	"github.com/letsencrypt/boulder/grpc/internal/testutils"
	"github.com/letsencrypt/boulder/test"
)

func TestMain(m *testing.M) {
	// Set a non-zero duration only for tests which are actually testing that
	// feature.
	replaceDNSResRate(time.Duration(0)) // No need to clean up since we os.Exit
	overrideDefaultResolver(false)      // No need to clean up since we os.Exit
	code := m.Run()
	os.Exit(code)
}

const (
	txtBytesLimit           = 255
	defaultTestTimeout      = 10 * time.Second
	defaultTestShortTimeout = 10 * time.Millisecond
)

type testClientConn struct {
	resolver.ClientConn // For unimplemented functions
	target              string
	m1                  sync.Mutex
	state               resolver.State
	updateStateCalls    int
	errChan             chan error
	updateStateErr      error
}

func (t *testClientConn) UpdateState(s resolver.State) error {
	t.m1.Lock()
	defer t.m1.Unlock()
	t.state = s
	t.updateStateCalls++
	// This error determines whether DNS Resolver actually decides to exponentially backoff or not.
	// This can be any error.
	return t.updateStateErr
}

func (t *testClientConn) getState() (resolver.State, int) {
	t.m1.Lock()
	defer t.m1.Unlock()
	return t.state, t.updateStateCalls
}

func (t *testClientConn) ReportError(err error) {
	t.errChan <- err
}

type testResolver struct {
	// A write to this channel is made when this resolver receives a resolution
	// request. Tests can rely on reading from this channel to be notified about
	// resolution requests instead of sleeping for a predefined period of time.
	lookupHostCh *testutils.Channel
}

func (tr *testResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	if tr.lookupHostCh != nil {
		tr.lookupHostCh.Send(nil)
	}
	return hostLookup(host)
}

func (*testResolver) LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error) {
	return srvLookup(service, proto, name)
}

// overrideDefaultResolver overrides the defaultResolver used by the code with
// an instance of the testResolver. pushOnLookup controls whether the
// testResolver created here pushes lookupHost events on its channel.
func overrideDefaultResolver(pushOnLookup bool) func() {
	oldResolver := defaultResolver

	var lookupHostCh *testutils.Channel
	if pushOnLookup {
		lookupHostCh = testutils.NewChannel()
	}
	defaultResolver = &testResolver{lookupHostCh: lookupHostCh}

	return func() {
		defaultResolver = oldResolver
	}
}

func replaceDNSResRate(d time.Duration) func() {
	oldMinDNSResRate := minDNSResRate
	minDNSResRate = d

	return func() {
		minDNSResRate = oldMinDNSResRate
	}
}

var hostLookupTbl = struct {
	sync.Mutex
	tbl map[string][]string
}{
	tbl: map[string][]string{
		"ipv4.single.fake": {"2.4.6.8"},
		"ipv4.multi.fake":  {"1.2.3.4", "5.6.7.8", "9.10.11.12"},
		"ipv6.single.fake": {"2607:f8b0:400a:801::1001"},
		"ipv6.multi.fake":  {"2607:f8b0:400a:801::1001", "2607:f8b0:400a:801::1002", "2607:f8b0:400a:801::1003"},
	},
}

func hostLookup(host string) ([]string, error) {
	hostLookupTbl.Lock()
	defer hostLookupTbl.Unlock()
	if addrs, ok := hostLookupTbl.tbl[host]; ok {
		return addrs, nil
	}
	return nil, &net.DNSError{
		Err:         "hostLookup error",
		Name:        host,
		Server:      "fake",
		IsTemporary: true,
	}
}

var srvLookupTbl = struct {
	sync.Mutex
	tbl map[string][]*net.SRV
}{
	tbl: map[string][]*net.SRV{
		"_foo._tcp.ipv4.single.fake": {&net.SRV{Target: "ipv4.single.fake", Port: 1234}},
		"_foo._tcp.ipv4.multi.fake":  {&net.SRV{Target: "ipv4.multi.fake", Port: 1234}},
		"_foo._tcp.ipv6.single.fake": {&net.SRV{Target: "ipv6.single.fake", Port: 1234}},
		"_foo._tcp.ipv6.multi.fake":  {&net.SRV{Target: "ipv6.multi.fake", Port: 1234}},
	},
}

func srvLookup(service, proto, name string) (string, []*net.SRV, error) {
	cname := "_" + service + "._" + proto + "." + name
	srvLookupTbl.Lock()
	defer srvLookupTbl.Unlock()
	if srvs, cnt := srvLookupTbl.tbl[cname]; cnt {
		return cname, srvs, nil
	}
	return "", nil, &net.DNSError{
		Err:         "srvLookup error",
		Name:        cname,
		Server:      "fake",
		IsTemporary: true,
	}
}

func TestResolve(t *testing.T) {
	testDNSResolver(t)
	testDNSResolveNow(t)
}

func testDNSResolver(t *testing.T) {
	defer func(nt func(d time.Duration) *time.Timer) {
		newTimer = nt
	}(newTimer)
	newTimer = func(_ time.Duration) *time.Timer {
		// Will never fire on its own, will protect from triggering exponential backoff.
		return time.NewTimer(time.Hour)
	}
	tests := []struct {
		target   string
		addrWant []resolver.Address
	}{
		{
			"foo.ipv4.single.fake",
			[]resolver.Address{{Addr: "2.4.6.8:1234", ServerName: "ipv4.single.fake"}},
		},
		{
			"foo.ipv4.multi.fake",
			[]resolver.Address{
				{Addr: "1.2.3.4:1234", ServerName: "ipv4.multi.fake"},
				{Addr: "5.6.7.8:1234", ServerName: "ipv4.multi.fake"},
				{Addr: "9.10.11.12:1234", ServerName: "ipv4.multi.fake"},
			},
		},
		{
			"foo.ipv6.single.fake",
			[]resolver.Address{{Addr: "[2607:f8b0:400a:801::1001]:1234", ServerName: "ipv6.single.fake"}},
		},
		{
			"foo.ipv6.multi.fake",
			[]resolver.Address{
				{Addr: "[2607:f8b0:400a:801::1001]:1234", ServerName: "ipv6.multi.fake"},
				{Addr: "[2607:f8b0:400a:801::1002]:1234", ServerName: "ipv6.multi.fake"},
				{Addr: "[2607:f8b0:400a:801::1003]:1234", ServerName: "ipv6.multi.fake"},
			},
		},
	}

	for _, a := range tests {
		b := NewDefaultSRVBuilder()
		cc := &testClientConn{target: a.target}
		r, err := b.Build(resolver.Target{URL: *testutils.MustParseURL(fmt.Sprintf("scheme:///%s", a.target))}, cc, resolver.BuildOptions{})
		if err != nil {
			t.Fatalf("%v\n", err)
		}
		var state resolver.State
		var cnt int
		for range 2000 {
			state, cnt = cc.getState()
			if cnt > 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if cnt == 0 {
			t.Fatalf("UpdateState not called after 2s; aborting")
		}

		if !slices.Equal(a.addrWant, state.Addresses) {
			t.Errorf("Resolved addresses of target: %q = %+v, want %+v", a.target, state.Addresses, a.addrWant)
		}
		r.Close()
	}
}

// DNS Resolver immediately starts polling on an error from grpc. This should continue until the ClientConn doesn't
// send back an error from updating the DNS Resolver's state.
func TestDNSResolverExponentialBackoff(t *testing.T) {
	defer leakcheck.Check(t)
	defer func(nt func(d time.Duration) *time.Timer) {
		newTimer = nt
	}(newTimer)
	timerChan := testutils.NewChannel()
	newTimer = func(d time.Duration) *time.Timer {
		// Will never fire on its own, allows this test to call timer immediately.
		t := time.NewTimer(time.Hour)
		timerChan.Send(t)
		return t
	}
	target := "foo.ipv4.single.fake"
	wantAddr := []resolver.Address{{Addr: "2.4.6.8:1234", ServerName: "ipv4.single.fake"}}

	b := NewDefaultSRVBuilder()
	cc := &testClientConn{target: target}
	// Cause ClientConn to return an error.
	cc.updateStateErr = balancer.ErrBadResolverState
	r, err := b.Build(resolver.Target{URL: *testutils.MustParseURL(fmt.Sprintf("scheme:///%s", target))}, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Error building resolver for target %v: %v", target, err)
	}
	defer r.Close()
	var state resolver.State
	var cnt int
	for range 2000 {
		state, cnt = cc.getState()
		if cnt > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if cnt == 0 {
		t.Fatalf("UpdateState not called after 2s; aborting")
	}
	if !slices.Equal(wantAddr, state.Addresses) {
		t.Errorf("Resolved addresses of target: %q = %+v, want %+v", target, state.Addresses, target)
	}
	ctx, ctxCancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer ctxCancel()
	// Cause timer to go off 10 times, and see if it calls updateState() correctly.
	for range 10 {
		timer, err := timerChan.Receive(ctx)
		if err != nil {
			t.Fatalf("Error receiving timer from mock NewTimer call: %v", err)
		}
		timerPointer := timer.(*time.Timer)
		timerPointer.Reset(0)
	}
	// Poll to see if DNS Resolver updated state the correct number of times, which allows time for the DNS Resolver to call
	// ClientConn update state.
	deadline := time.Now().Add(defaultTestTimeout)
	for {
		cc.m1.Lock()
		got := cc.updateStateCalls
		cc.m1.Unlock()
		if got == 11 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("Exponential backoff is not working as expected - should update state 11 times instead of %d", got)
		}

		time.Sleep(time.Millisecond)
	}

	// Update resolver.ClientConn to not return an error anymore - this should stop it from backing off.
	cc.updateStateErr = nil
	timer, err := timerChan.Receive(ctx)
	if err != nil {
		t.Fatalf("Error receiving timer from mock NewTimer call: %v", err)
	}
	timerPointer := timer.(*time.Timer)
	timerPointer.Reset(0)
	// Poll to see if DNS Resolver updated state the correct number of times, which allows time for the DNS Resolver to call
	// ClientConn update state the final time. The DNS Resolver should then stop polling.
	deadline = time.Now().Add(defaultTestTimeout)
	for {
		cc.m1.Lock()
		got := cc.updateStateCalls
		cc.m1.Unlock()
		if got == 12 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("Exponential backoff is not working as expected - should stop backing off at 12 total UpdateState calls instead of %d", got)
		}

		_, err := timerChan.ReceiveOrFail()
		if err {
			t.Fatalf("Should not poll again after Client Conn stops returning error.")
		}

		time.Sleep(time.Millisecond)
	}
}

func mutateTbl(target string) func() {
	hostLookupTbl.Lock()
	oldHostTblEntry := hostLookupTbl.tbl[target]

	// Remove the last address from the target's entry.
	hostLookupTbl.tbl[target] = hostLookupTbl.tbl[target][:len(oldHostTblEntry)-1]
	hostLookupTbl.Unlock()

	return func() {
		hostLookupTbl.Lock()
		hostLookupTbl.tbl[target] = oldHostTblEntry
		hostLookupTbl.Unlock()
	}
}

func testDNSResolveNow(t *testing.T) {
	defer leakcheck.Check(t)
	defer func(nt func(d time.Duration) *time.Timer) {
		newTimer = nt
	}(newTimer)
	newTimer = func(_ time.Duration) *time.Timer {
		// Will never fire on its own, will protect from triggering exponential backoff.
		return time.NewTimer(time.Hour)
	}
	tests := []struct {
		target   string
		addrWant []resolver.Address
		addrNext []resolver.Address
	}{
		{
			"foo.ipv4.multi.fake",
			[]resolver.Address{
				{Addr: "1.2.3.4:1234", ServerName: "ipv4.multi.fake"},
				{Addr: "5.6.7.8:1234", ServerName: "ipv4.multi.fake"},
				{Addr: "9.10.11.12:1234", ServerName: "ipv4.multi.fake"},
			},
			[]resolver.Address{
				{Addr: "1.2.3.4:1234", ServerName: "ipv4.multi.fake"},
				{Addr: "5.6.7.8:1234", ServerName: "ipv4.multi.fake"},
			},
		},
	}

	for _, a := range tests {
		b := NewDefaultSRVBuilder()
		cc := &testClientConn{target: a.target}
		r, err := b.Build(resolver.Target{URL: *testutils.MustParseURL(fmt.Sprintf("scheme:///%s", a.target))}, cc, resolver.BuildOptions{})
		if err != nil {
			t.Fatalf("%v\n", err)
		}
		defer r.Close()
		var state resolver.State
		var cnt int
		for range 2000 {
			state, cnt = cc.getState()
			if cnt > 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if cnt == 0 {
			t.Fatalf("UpdateState not called after 2s; aborting.  state=%v", state)
		}
		if !slices.Equal(a.addrWant, state.Addresses) {
			t.Errorf("Resolved addresses of target: %q = %+v, want %+v", a.target, state.Addresses, a.addrWant)
		}

		revertTbl := mutateTbl(strings.TrimPrefix(a.target, "foo."))
		r.ResolveNow(resolver.ResolveNowOptions{})
		for range 2000 {
			state, cnt = cc.getState()
			if cnt == 2 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if cnt != 2 {
			t.Fatalf("UpdateState not called after 2s; aborting.  state=%v", state)
		}
		if !slices.Equal(a.addrNext, state.Addresses) {
			t.Errorf("Resolved addresses of target: %q = %+v, want %+v", a.target, state.Addresses, a.addrNext)
		}
		revertTbl()
	}
}

func TestDNSResolverRetry(t *testing.T) {
	defer func(nt func(d time.Duration) *time.Timer) {
		newTimer = nt
	}(newTimer)
	newTimer = func(d time.Duration) *time.Timer {
		// Will never fire on its own, will protect from triggering exponential backoff.
		return time.NewTimer(time.Hour)
	}
	b := NewDefaultSRVBuilder()
	target := "foo.ipv4.single.fake"
	cc := &testClientConn{target: target}
	r, err := b.Build(resolver.Target{URL: *testutils.MustParseURL(fmt.Sprintf("scheme:///%s", target))}, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("%v\n", err)
	}
	defer r.Close()
	var state resolver.State
	for range 2000 {
		state, _ = cc.getState()
		if len(state.Addresses) == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(state.Addresses) != 1 {
		t.Fatalf("UpdateState not called with 1 address after 2s; aborting.  state=%v", state)
	}
	want := []resolver.Address{{Addr: "2.4.6.8:1234", ServerName: "ipv4.single.fake"}}
	if !slices.Equal(want, state.Addresses) {
		t.Errorf("Resolved addresses of target: %q = %+v, want %+v", target, state.Addresses, want)
	}
	// mutate the host lookup table so the target has 0 address returned.
	revertTbl := mutateTbl(strings.TrimPrefix(target, "foo."))
	// trigger a resolve that will get empty address list
	r.ResolveNow(resolver.ResolveNowOptions{})
	for range 2000 {
		state, _ = cc.getState()
		if len(state.Addresses) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(state.Addresses) != 0 {
		t.Fatalf("UpdateState not called with 0 address after 2s; aborting.  state=%v", state)
	}
	revertTbl()
	// wait for the retry to happen in two seconds.
	r.ResolveNow(resolver.ResolveNowOptions{})
	for range 2000 {
		state, _ = cc.getState()
		if len(state.Addresses) == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !slices.Equal(want, state.Addresses) {
		t.Errorf("Resolved addresses of target: %q = %+v, want %+v", target, state.Addresses, want)
	}
}

func TestCustomAuthority(t *testing.T) {
	defer leakcheck.Check(t)
	defer func(nt func(d time.Duration) *time.Timer) {
		newTimer = nt
	}(newTimer)
	newTimer = func(d time.Duration) *time.Timer {
		// Will never fire on its own, will protect from triggering exponential backoff.
		return time.NewTimer(time.Hour)
	}

	tests := []struct {
		authority     string
		authorityWant string
		expectError   bool
	}{
		{
			"4.3.2.1:" + defaultDNSSvrPort,
			"4.3.2.1:" + defaultDNSSvrPort,
			false,
		},
		{
			"4.3.2.1:123",
			"4.3.2.1:123",
			false,
		},
		{
			"4.3.2.1",
			"4.3.2.1:" + defaultDNSSvrPort,
			false,
		},
		{
			"::1",
			"[::1]:" + defaultDNSSvrPort,
			false,
		},
		{
			"[::1]",
			"[::1]:" + defaultDNSSvrPort,
			false,
		},
		{
			"[::1]:123",
			"[::1]:123",
			false,
		},
		{
			"dnsserver.com",
			"dnsserver.com:" + defaultDNSSvrPort,
			false,
		},
		{
			":123",
			"localhost:123",
			false,
		},
		{
			":",
			"",
			true,
		},
		{
			"[::1]:",
			"",
			true,
		},
		{
			"dnsserver.com:",
			"",
			true,
		},
	}
	oldcustomAuthorityDialer := customAuthorityDialer
	defer func() {
		customAuthorityDialer = oldcustomAuthorityDialer
	}()

	for _, a := range tests {
		errChan := make(chan error, 1)
		customAuthorityDialer = func(authority string) func(ctx context.Context, network, address string) (net.Conn, error) {
			if authority != a.authorityWant {
				errChan <- fmt.Errorf("wrong custom authority passed to resolver. input: %s expected: %s actual: %s", a.authority, a.authorityWant, authority)
			} else {
				errChan <- nil
			}
			return func(ctx context.Context, network, address string) (net.Conn, error) {
				return nil, errors.New("no need to dial")
			}
		}

		mockEndpointTarget := "foo.bar.com"
		b := NewDefaultSRVBuilder()
		cc := &testClientConn{target: mockEndpointTarget, errChan: make(chan error, 1)}
		target := resolver.Target{
			URL: *testutils.MustParseURL(fmt.Sprintf("scheme://%s/%s", a.authority, mockEndpointTarget)),
		}
		r, err := b.Build(target, cc, resolver.BuildOptions{})

		if err == nil {
			r.Close()

			err = <-errChan
			if err != nil {
				t.Error(err.Error())
			}

			if a.expectError {
				t.Errorf("custom authority should have caused an error: %s", a.authority)
			}
		} else if !a.expectError {
			t.Errorf("unexpected error using custom authority %s: %s", a.authority, err)
		}
	}
}

// TestRateLimitedResolve exercises the rate limit enforced on re-resolution
// requests. It sets the re-resolution rate to a small value and repeatedly
// calls ResolveNow() and ensures only the expected number of resolution
// requests are made.
func TestRateLimitedResolve(t *testing.T) {
	defer leakcheck.Check(t)
	defer func(nt func(d time.Duration) *time.Timer) {
		newTimer = nt
	}(newTimer)
	newTimer = func(d time.Duration) *time.Timer {
		// Will never fire on its own, will protect from triggering exponential
		// backoff.
		return time.NewTimer(time.Hour)
	}
	defer func(nt func(d time.Duration) *time.Timer) {
		newTimerDNSResRate = nt
	}(newTimerDNSResRate)

	timerChan := testutils.NewChannel()
	newTimerDNSResRate = func(d time.Duration) *time.Timer {
		// Will never fire on its own, allows this test to call timer
		// immediately.
		t := time.NewTimer(time.Hour)
		timerChan.Send(t)
		return t
	}

	// Create a new testResolver{} for this test because we want the exact count
	// of the number of times the resolver was invoked.
	nc := overrideDefaultResolver(true)
	defer nc()

	target := "foo.ipv4.single.fake"
	b := NewDefaultSRVBuilder()
	cc := &testClientConn{target: target}

	r, err := b.Build(resolver.Target{URL: *testutils.MustParseURL(fmt.Sprintf("scheme:///%s", target))}, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("resolver.Build() returned error: %v\n", err)
	}
	defer r.Close()

	dnsR, ok := r.(*dnsResolver)
	if !ok {
		t.Fatalf("resolver.Build() returned unexpected type: %T\n", dnsR)
	}

	tr, ok := dnsR.resolver.(*testResolver)
	if !ok {
		t.Fatalf("delegate resolver returned unexpected type: %T\n", tr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	// Wait for the first resolution request to be done. This happens as part
	// of the first iteration of the for loop in watcher().
	if _, err := tr.lookupHostCh.Receive(ctx); err != nil {
		t.Fatalf("Timed out waiting for lookup() call.")
	}

	// Call Resolve Now 100 times, shouldn't continue onto next iteration of
	// watcher, thus shouldn't lookup again.
	for range 100 {
		r.ResolveNow(resolver.ResolveNowOptions{})
	}

	continueCtx, continueCancel := context.WithTimeout(context.Background(), defaultTestShortTimeout)
	defer continueCancel()

	if _, err := tr.lookupHostCh.Receive(continueCtx); err == nil {
		t.Fatalf("Should not have looked up again as DNS Min Res Rate timer has not gone off.")
	}

	// Make the DNSMinResRate timer fire immediately (by receiving it, then
	// resetting to 0), this will unblock the resolver which is currently
	// blocked on the DNS Min Res Rate timer going off, which will allow it to
	// continue to the next iteration of the watcher loop.
	timer, err := timerChan.Receive(ctx)
	if err != nil {
		t.Fatalf("Error receiving timer from mock NewTimer call: %v", err)
	}
	timerPointer := timer.(*time.Timer)
	timerPointer.Reset(0)

	// Now that DNS Min Res Rate timer has gone off, it should lookup again.
	if _, err := tr.lookupHostCh.Receive(ctx); err != nil {
		t.Fatalf("Timed out waiting for lookup() call.")
	}

	// Resolve Now 1000 more times, shouldn't lookup again as DNS Min Res Rate
	// timer has not gone off.
	for range 1000 {
		r.ResolveNow(resolver.ResolveNowOptions{})
	}

	if _, err = tr.lookupHostCh.Receive(continueCtx); err == nil {
		t.Fatalf("Should not have looked up again as DNS Min Res Rate timer has not gone off.")
	}

	// Make the DNSMinResRate timer fire immediately again.
	timer, err = timerChan.Receive(ctx)
	if err != nil {
		t.Fatalf("Error receiving timer from mock NewTimer call: %v", err)
	}
	timerPointer = timer.(*time.Timer)
	timerPointer.Reset(0)

	// Now that DNS Min Res Rate timer has gone off, it should lookup again.
	if _, err = tr.lookupHostCh.Receive(ctx); err != nil {
		t.Fatalf("Timed out waiting for lookup() call.")
	}

	wantAddrs := []resolver.Address{{Addr: "2.4.6.8:1234", ServerName: "ipv4.single.fake"}}
	var state resolver.State
	for {
		var cnt int
		state, cnt = cc.getState()
		if cnt > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !slices.Equal(state.Addresses, wantAddrs) {
		t.Errorf("Resolved addresses of target: %q = %+v, want %+v", target, state.Addresses, wantAddrs)
	}
}

// DNS Resolver immediately starts polling on an error. This will cause the re-resolution to return another error.
// Thus, test that it constantly sends errors to the grpc.ClientConn.
func TestReportError(t *testing.T) {
	const target = "not.found"
	defer func(nt func(d time.Duration) *time.Timer) {
		newTimer = nt
	}(newTimer)
	timerChan := testutils.NewChannel()
	newTimer = func(d time.Duration) *time.Timer {
		// Will never fire on its own, allows this test to call timer immediately.
		t := time.NewTimer(time.Hour)
		timerChan.Send(t)
		return t
	}
	cc := &testClientConn{target: target, errChan: make(chan error)}
	totalTimesCalledError := 0
	b := NewDefaultSRVBuilder()
	r, err := b.Build(resolver.Target{URL: *testutils.MustParseURL(fmt.Sprintf("scheme:///%s", target))}, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Error building resolver for target %v: %v", target, err)
	}
	// Should receive first error.
	err = <-cc.errChan
	if !strings.Contains(err.Error(), "srvLookup error") {
		t.Fatalf(`ReportError(err=%v) called; want err contains "srvLookupError"`, err)
	}
	totalTimesCalledError++
	ctx, ctxCancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer ctxCancel()
	timer, err := timerChan.Receive(ctx)
	if err != nil {
		t.Fatalf("Error receiving timer from mock NewTimer call: %v", err)
	}
	timerPointer := timer.(*time.Timer)
	timerPointer.Reset(0)
	defer r.Close()

	// Cause timer to go off 10 times, and see if it matches DNS Resolver updating Error.
	for range 10 {
		// Should call ReportError().
		err = <-cc.errChan
		if !strings.Contains(err.Error(), "srvLookup error") {
			t.Fatalf(`ReportError(err=%v) called; want err contains "srvLookupError"`, err)
		}
		totalTimesCalledError++
		timer, err := timerChan.Receive(ctx)
		if err != nil {
			t.Fatalf("Error receiving timer from mock NewTimer call: %v", err)
		}
		timerPointer := timer.(*time.Timer)
		timerPointer.Reset(0)
	}

	if totalTimesCalledError != 11 {
		t.Errorf("ReportError() not called 11 times, instead called %d times.", totalTimesCalledError)
	}
	// Clean up final watcher iteration.
	<-cc.errChan
	_, err = timerChan.Receive(ctx)
	if err != nil {
		t.Fatalf("Error receiving timer from mock NewTimer call: %v", err)
	}
}

func Test_parseServiceDomain(t *testing.T) {
	tests := []struct {
		target        string
		expectService string
		expectDomain  string
		wantErr       bool
	}{
		// valid
		{"foo.bar", "foo", "bar", false},
		{"foo.bar.baz", "foo", "bar.baz", false},
		{"foo.bar.baz.", "foo", "bar.baz.", false},

		// invalid
		{"", "", "", true},
		{".", "", "", true},
		{"foo", "", "", true},
		{".foo", "", "", true},
		{"foo.", "", "", true},
		{".foo.bar.baz", "", "", true},
		{".foo.bar.baz.", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			gotService, gotDomain, err := parseServiceDomain(tt.target)
			if tt.wantErr {
				test.AssertError(t, err, "expect err got nil")
			} else {
				test.AssertNotError(t, err, "expect nil err")
				test.AssertEquals(t, gotService, tt.expectService)
				test.AssertEquals(t, gotDomain, tt.expectDomain)
			}
		})
	}
}
