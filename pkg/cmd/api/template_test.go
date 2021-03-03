package api

import "testing"

func Test_jsonScalarToString(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    string
		wantErr bool
	}{
		{
			name:  "string",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "int",
			input: float64(1234),
			want:  "1234",
		},
		{
			name:  "float",
			input: float64(12.34),
			want:  "12.34",
		},
		{
			name:  "null",
			input: nil,
			want:  "",
		},
		{
			name:  "true",
			input: true,
			want:  "true",
		},
		{
			name:  "false",
			input: false,
			want:  "false",
		},
		{
			name:    "object",
			input:   map[string]interface{}{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsonScalarToString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("jsonScalarToString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("jsonScalarToString() = %v, want %v", got, tt.want)
			}
		})
	}
}
