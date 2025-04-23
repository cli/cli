package shared

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/api"
)

func Test_selectComment(t *testing.T) {
	type args struct {
		selector string
		comments []api.Comment
	}
	tests := []struct {
		name    string
		args    args
		want    *api.Comment
		wantErr error
	}{
		{
			name: "body contains expression",
			args: args{
				selector: `. | select(.body | contains("thank you"))`,
				comments: []api.Comment{
					{ID: "test-0", Body: "It is a test. Thanks."},
					{ID: "test-1", Body: "now thank you test"},
					{ID: "test-2", Body: "thank test you"},
				},
			},
			want: &api.Comment{ID: "test-1"},
		},
		{
			name: "id is known",
			args: args{
				selector: `. | select(.id == "test-1")`,
				comments: []api.Comment{
					{ID: "test-0"},
					{ID: "test-1"},
					{ID: "test-2"},
				},
			},
			want: &api.Comment{ID: "test-1"},
		},
		{
			name: "no comments matching expression",
			args: args{
				selector: `. | select(.author.login == "not-alice")`,
				comments: []api.Comment{
					{ID: "test-0", Author: api.CommentAuthor{Login: "alice"}},
					{ID: "test-1", Author: api.CommentAuthor{Login: "alice"}},
					{ID: "test-2", Author: api.CommentAuthor{Login: "alice"}},
				},
			},
			wantErr: errNoSelectorComments,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectComment(tt.args.selector, tt.args.comments)
			if tt.wantErr != nil {
				if got != nil {
					t.Errorf("selectComment() = %v, _, want %v, _", got, tt.want)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("selectComment() = _, %v, want _, %v", err, tt.wantErr)
				}
			} else {
				if got == nil || got.ID != tt.want.ID {
					t.Errorf("selectComment() = %v, _, want %v, _", got, tt.want)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("selectComment() = _, %v, want _, %v", err, tt.wantErr)
				}
			}
		})
	}
}
