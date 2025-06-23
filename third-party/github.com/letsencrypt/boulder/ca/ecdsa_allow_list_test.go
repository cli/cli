package ca

import (
	"testing"
)

func TestNewECDSAAllowListFromFile(t *testing.T) {
	t.Parallel()
	type args struct {
		filename string
	}
	tests := []struct {
		name              string
		args              args
		want1337Permitted bool
		wantEntries       int
		wantErrBool       bool
	}{
		{
			name:              "one entry",
			args:              args{"testdata/ecdsa_allow_list.yml"},
			want1337Permitted: true,
			wantEntries:       1,
			wantErrBool:       false,
		},
		{
			name:              "one entry but it's not 1337",
			args:              args{"testdata/ecdsa_allow_list2.yml"},
			want1337Permitted: false,
			wantEntries:       1,
			wantErrBool:       false,
		},
		{
			name:              "should error due to no file",
			args:              args{"testdata/ecdsa_allow_list_no_exist.yml"},
			want1337Permitted: false,
			wantEntries:       0,
			wantErrBool:       true,
		},
		{
			name:              "should error due to malformed YAML",
			args:              args{"testdata/ecdsa_allow_list_malformed.yml"},
			want1337Permitted: false,
			wantEntries:       0,
			wantErrBool:       true,
		},
	}

	for _, tt := range tests {
		// TODO(Remove this >= go1.22.3) This shouldn't be necessary due to
		// go1.22 changing loopvars.
		// https://github.com/golang/go/issues/65612#issuecomment-1943342030
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			allowList, gotEntries, err := NewECDSAAllowListFromFile(tt.args.filename)
			if (err != nil) != tt.wantErrBool {
				t.Errorf("NewECDSAAllowListFromFile() error = %v, wantErr %v", err, tt.wantErrBool)
				t.Error(allowList, gotEntries, err)
				return
			}
			if allowList != nil && allowList.permitted(1337) != tt.want1337Permitted {
				t.Errorf("NewECDSAAllowListFromFile() allowList = %v, want %v", allowList, tt.want1337Permitted)
			}
			if gotEntries != tt.wantEntries {
				t.Errorf("NewECDSAAllowListFromFile() gotEntries = %v, want %v", gotEntries, tt.wantEntries)
			}
		})
	}
}
