package export

import (
	"strings"
	"testing"
)

func Test_Row_withoutTable(t *testing.T) {
	formatter := &tableFormatter{
		current: nil,
		writer:  &strings.Builder{},
	}
	if _, err := formatter.Row("col"); err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_EndTable_withoutTable(t *testing.T) {
	formatter := &tableFormatter{
		current: nil,
		writer:  &strings.Builder{},
	}
	if _, err := formatter.EndTable(); err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_byteArg(t *testing.T) {
	tests := []struct {
		name    string
		args    []interface{}
		want    byte
		wantErr bool
	}{
		{
			name: "empty",
			args: make([]interface{}, 0),
			want: ' ',
		},
		{
			name: "byte",
			args: makeArray(byte('.')),
			want: '.',
		},
		{
			name: "int",
			args: makeArray(int('.')),
			want: '.',
		},
		{
			name:    "negative int",
			args:    makeArray(int(-1)),
			wantErr: true,
		},
		{
			name:    "large int",
			args:    makeArray(int(256)),
			wantErr: true,
		},
		{
			name: "string",
			args: makeArray("."),
			want: '.',
		},
		{
			name:    "empty string",
			args:    makeArray(""),
			wantErr: true,
		},
		{
			name:    "long string",
			args:    makeArray("..."),
			wantErr: true,
		},
		{
			name:    "unsupported type",
			args:    makeArray(t),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := byteArg(tt.args, 0, ' ')
			if (err != nil) != tt.wantErr {
				t.Errorf("byteArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("byteArg() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_intArg(t *testing.T) {
	tests := []struct {
		name    string
		args    []interface{}
		want    int
		wantErr bool
	}{
		{
			name: "empty",
			args: make([]interface{}, 0),
			want: 0,
		},
		{
			name: "int",
			args: makeArray(int(1)),
			want: 1,
		},
		{
			name: "negative int",
			args: makeArray(int(-1)),
			want: -1,
		},
		{
			name:    "unsupported type",
			args:    makeArray(t),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := intArg(tt.args, 0, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("intArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("intArg() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_uintArg(t *testing.T) {
	tests := []struct {
		name    string
		args    []interface{}
		want    uint
		wantErr bool
	}{
		{
			name: "empty",
			args: make([]interface{}, 0),
			want: 0,
		},
		{
			name: "uint",
			args: makeArray(uint(1)),
			want: 1,
		},
		{
			name: "int",
			args: makeArray(int(1)),
			want: 1,
		},
		{
			name:    "negative int",
			args:    makeArray(int(-1)),
			wantErr: true,
		},
		{
			name:    "unsupported type",
			args:    makeArray(t),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := uintArg(tt.args, 0, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("intArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("intArg() = %v, want %v", got, tt.want)
			}
		})
	}
}

func makeArray(args ...interface{}) []interface{} {
	arr := make([]interface{}, len(args))
	copy(arr, args)
	return arr
}
