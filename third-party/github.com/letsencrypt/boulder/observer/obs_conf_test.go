package observer

import (
	"errors"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/observer/probers"
	_ "github.com/letsencrypt/boulder/observer/probers/mock"
	"github.com/letsencrypt/boulder/test"
)

const (
	debugAddr = ":8040"
	errDBZMsg = "over 9000"
	mockConf  = "Mock"
)

func TestObsConf_makeMonitors(t *testing.T) {
	var errDBZ = errors.New(errDBZMsg)
	var cfgSyslog = cmd.SyslogConfig{StdoutLevel: 6, SyslogLevel: 6}
	var cfgDur = config.Duration{Duration: time.Second * 5}
	var cfgBuckets = []float64{.001}
	var validMonConf = &MonConf{
		cfgDur, mockConf, probers.Settings{"valid": true, "pname": "foo", "pkind": "bar"}}
	var invalidMonConf = &MonConf{
		cfgDur, mockConf, probers.Settings{"valid": false, "errmsg": errDBZMsg, "pname": "foo", "pkind": "bar"}}
	type fields struct {
		Syslog    cmd.SyslogConfig
		Buckets   []float64
		DebugAddr string
		MonConfs  []*MonConf
	}
	tests := []struct {
		name    string
		fields  fields
		errs    []error
		wantErr bool
	}{
		// valid
		{"1 valid", fields{cfgSyslog, cfgBuckets, debugAddr, []*MonConf{validMonConf}}, nil, false},
		{"2 valid", fields{
			cfgSyslog, cfgBuckets, debugAddr, []*MonConf{validMonConf, validMonConf}}, nil, false},
		{"1 valid, 1 invalid", fields{
			cfgSyslog, cfgBuckets, debugAddr, []*MonConf{validMonConf, invalidMonConf}}, []error{errDBZ}, false},
		{"1 valid, 2 invalid", fields{
			cfgSyslog, cfgBuckets, debugAddr, []*MonConf{invalidMonConf, validMonConf, invalidMonConf}}, []error{errDBZ, errDBZ}, false},
		// invalid
		{"1 invalid", fields{cfgSyslog, cfgBuckets, debugAddr, []*MonConf{invalidMonConf}}, []error{errDBZ}, true},
		{"0", fields{cfgSyslog, cfgBuckets, debugAddr, []*MonConf{}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ObsConf{
				Syslog:    tt.fields.Syslog,
				Buckets:   tt.fields.Buckets,
				DebugAddr: tt.fields.DebugAddr,
				MonConfs:  tt.fields.MonConfs,
			}
			_, errs, err := c.makeMonitors(metrics.NoopRegisterer)
			if len(errs) != len(tt.errs) {
				t.Errorf("ObsConf.validateMonConfs() errs = %d, want %d", len(errs), len(tt.errs))
				t.Logf("%v", errs)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("ObsConf.validateMonConfs() err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestObsConf_ValidateDebugAddr(t *testing.T) {
	type fields struct {
		DebugAddr string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// valid
		{"max len and range", fields{":65535"}, false},
		{"min len and range", fields{":1"}, false},
		{"2 digits", fields{":80"}, false},
		// invalid
		{"out of range high", fields{":65536"}, true},
		{"out of range low", fields{":0"}, true},
		{"not even a port", fields{":foo"}, true},
		{"missing :", fields{"foo"}, true},
		{"missing port", fields{"foo:"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ObsConf{
				DebugAddr: tt.fields.DebugAddr,
			}
			err := c.validateDebugAddr()
			if tt.wantErr {
				test.AssertError(t, err, "ObsConf.ValidateDebugAddr() should have errored")
			} else {
				test.AssertNotError(t, err, "ObsConf.ValidateDebugAddr() shouldn't have errored")
			}
		})
	}
}

func TestObsConf_validateSyslog(t *testing.T) {
	type fields struct {
		Syslog cmd.SyslogConfig
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// valid
		{"valid", fields{cmd.SyslogConfig{StdoutLevel: 6, SyslogLevel: 6}}, false},
		// invalid
		{"both too high", fields{cmd.SyslogConfig{StdoutLevel: 9, SyslogLevel: 9}}, true},
		{"stdout too high", fields{cmd.SyslogConfig{StdoutLevel: 9, SyslogLevel: 6}}, true},
		{"syslog too high", fields{cmd.SyslogConfig{StdoutLevel: 6, SyslogLevel: 9}}, true},
		{"both too low", fields{cmd.SyslogConfig{StdoutLevel: -1, SyslogLevel: -1}}, true},
		{"stdout too low", fields{cmd.SyslogConfig{StdoutLevel: -1, SyslogLevel: 6}}, true},
		{"syslog too low", fields{cmd.SyslogConfig{StdoutLevel: 6, SyslogLevel: -1}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ObsConf{
				Syslog: tt.fields.Syslog,
			}
			err := c.validateSyslog()
			if tt.wantErr {
				test.AssertError(t, err, "ObsConf.validateSyslog() should have errored")
			} else {
				test.AssertNotError(t, err, "ObsConf.validateSyslog() shouldn't have errored")
			}
		})
	}
}
