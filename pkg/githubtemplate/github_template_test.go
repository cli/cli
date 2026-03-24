package githubtemplate

import (
	"os"
	"path"
	"reflect"
	"testing"
)

func TestFindNonLegacy(t *testing.T) {
	tests := []struct {
		name    string
		prepare []string
		argName string
		want    []string
	}{
		{
			name: "Legacy templates ignored",
			prepare: []string{
				"README.md",
				"ISSUE_TEMPLATE",
				"issue_template.md",
				"issue_template.txt",
				"pull_request_template.md",
				".github/issue_template.md",
				"docs/issue_template.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    []string{},
		},
		{
			name: "Template folder in .github takes precedence",
			prepare: []string{
				"ISSUE_TEMPLATE.md",
				"docs/ISSUE_TEMPLATE/abc.md",
				"ISSUE_TEMPLATE/abc.md",
				".github/ISSUE_TEMPLATE/abc.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    []string{".github/ISSUE_TEMPLATE/abc.md"},
		},
		{
			name: "Template folder in root",
			prepare: []string{
				"ISSUE_TEMPLATE.md",
				"docs/ISSUE_TEMPLATE/abc.md",
				"ISSUE_TEMPLATE/abc.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    []string{"ISSUE_TEMPLATE/abc.md"},
		},
		{
			name: "Template folder in docs",
			prepare: []string{
				"ISSUE_TEMPLATE.md",
				"docs/ISSUE_TEMPLATE/abc.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    []string{"docs/ISSUE_TEMPLATE/abc.md"},
		},
		{
			name: "Multiple templates in template folder",
			prepare: []string{
				".github/ISSUE_TEMPLATE/nope.md",
				".github/PULL_REQUEST_TEMPLATE.md",
				".github/PULL_REQUEST_TEMPLATE/one.md",
				".github/PULL_REQUEST_TEMPLATE/two.md",
				".github/PULL_REQUEST_TEMPLATE/three.md",
				"docs/pull_request_template.md",
			},
			argName: "PuLl_ReQuEsT_TeMpLaTe",
			want: []string{
				".github/PULL_REQUEST_TEMPLATE/one.md",
				".github/PULL_REQUEST_TEMPLATE/three.md",
				".github/PULL_REQUEST_TEMPLATE/two.md",
			},
		},
		{
			name: "Empty template directories",
			prepare: []string{
				".github/ISSUE_TEMPLATE/.keep",
				".docs/ISSUE_TEMPLATE/.keep",
				"ISSUE_TEMPLATE/.keep",
			},
			argName: "ISSUE_TEMPLATE",
			want:    []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpdir := t.TempDir()
			for _, p := range tt.prepare {
				fp := path.Join(tmpdir, p)
				_ = os.MkdirAll(path.Dir(fp), 0700)
				file, err := os.Create(fp)
				if err != nil {
					t.Fatal(err)
				}
				file.Close()
			}

			want := make([]string, len(tt.want))
			for i, w := range tt.want {
				want[i] = path.Join(tmpdir, w)
			}
			if got := FindNonLegacy(tmpdir, tt.argName); !reflect.DeepEqual(got, want) {
				t.Errorf("Find() = %v, want %v", got, want)
			}
		})
	}
}

func TestFindLegacy(t *testing.T) {
	tests := []struct {
		name    string
		prepare []string
		argName string
		want    string
	}{
		{
			name: "Template in root",
			prepare: []string{
				"README.md",
				"issue_template.md",
				"issue_template.txt",
				"pull_request_template.md",
				"docs/issue_template.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    "issue_template.md",
		},
		{
			name: "No extension",
			prepare: []string{
				"README.md",
				"issue_template",
				"docs/issue_template.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    "issue_template",
		},
		{
			name: "Dash instead of underscore",
			prepare: []string{
				"README.md",
				"issue-template.txt",
				"docs/issue_template.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    "issue-template.txt",
		},
		{
			name: "Template in .github takes precedence",
			prepare: []string{
				"ISSUE_TEMPLATE.md",
				".github/issue_template.md",
				"docs/issue_template.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    ".github/issue_template.md",
		},
		{
			name: "Template in docs",
			prepare: []string{
				"README.md",
				"docs/issue_template.md",
			},
			argName: "ISSUE_TEMPLATE",
			want:    "docs/issue_template.md",
		},
		{
			name: "Non legacy templates ignored",
			prepare: []string{
				".github/PULL_REQUEST_TEMPLATE/abc.md",
				"PULL_REQUEST_TEMPLATE/abc.md",
				"docs/PULL_REQUEST_TEMPLATE/abc.md",
				".github/PULL_REQUEST_TEMPLATE.md",
			},
			argName: "PuLl_ReQuEsT_TeMpLaTe",
			want:    ".github/PULL_REQUEST_TEMPLATE.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpdir := t.TempDir()
			for _, p := range tt.prepare {
				fp := path.Join(tmpdir, p)
				_ = os.MkdirAll(path.Dir(fp), 0700)
				file, err := os.Create(fp)
				if err != nil {
					t.Fatal(err)
				}
				file.Close()
			}

			want := path.Join(tmpdir, tt.want)
			got := FindLegacy(tmpdir, tt.argName)
			if got == "" {
				t.Errorf("FindLegacy() = nil, want %v", want)
			} else if got != want {
				t.Errorf("FindLegacy() = %v, want %v", got, want)
			}
		})
	}
}

func TestExtractName(t *testing.T) {
	tmpfile, err := os.CreateTemp(t.TempDir(), "gh-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer tmpfile.Close()

	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		prepare string
		args    args
		want    string
	}{
		{
			name: "Complete front-matter",
			prepare: `---
name: Bug Report
about: This is how you report bugs
---

**Template contents**
`,
			args: args{
				filePath: tmpfile.Name(),
			},
			want: "Bug Report",
		},
		{
			name: "Incomplete front-matter",
			prepare: `---
about: This is how you report bugs
---
`,
			args: args{
				filePath: tmpfile.Name(),
			},
			want: path.Base(tmpfile.Name()),
		},
		{
			name:    "No front-matter",
			prepare: `name: This is not yaml!`,
			args: args{
				filePath: tmpfile.Name(),
			},
			want: path.Base(tmpfile.Name()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.WriteFile(tmpfile.Name(), []byte(tt.prepare), 0600)
			if got := ExtractName(tt.args.filePath); got != tt.want {
				t.Errorf("ExtractName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractTitle(t *testing.T) {
	tmpfile, err := os.CreateTemp(t.TempDir(), "gh-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer tmpfile.Close()

	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		prepare string
		args    args
		want    string
	}{
		{
			name: "Complete front-matter",
			prepare: `---
name: Bug Report
title: 'bug: '
about: This is how you report bugs
---

**Template contents**
`,
			args: args{
				filePath: tmpfile.Name(),
			},
			want: "bug: ",
		},
		{
			name: "Incomplete front-matter",
			prepare: `---
about: This is how you report bugs
---
`,
			args: args{
				filePath: tmpfile.Name(),
			},
			want: "",
		},
		{
			name:    "No front-matter",
			prepare: `name: This is not yaml!`,
			args: args{
				filePath: tmpfile.Name(),
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.WriteFile(tmpfile.Name(), []byte(tt.prepare), 0600)
			if got := ExtractTitle(tt.args.filePath); got != tt.want {
				t.Errorf("ExtractTitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractContents(t *testing.T) {
	tmpfile, err := os.CreateTemp(t.TempDir(), "gh-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer tmpfile.Close()

	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		prepare string
		args    args
		want    string
	}{
		{
			name: "Has front-matter",
			prepare: `---
name: Bug Report
---


Template contents
---
More of template
`,
			args: args{
				filePath: tmpfile.Name(),
			},
			want: `Template contents
---
More of template
`,
		},
		{
			name: "No front-matter",
			prepare: `Template contents
---
More of template
---
Even more
`,
			args: args{
				filePath: tmpfile.Name(),
			},
			want: `Template contents
---
More of template
---
Even more
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.WriteFile(tmpfile.Name(), []byte(tt.prepare), 0600)
			if got := ExtractContents(tt.args.filePath); string(got) != tt.want {
				t.Errorf("ExtractContents() = %v, want %v", string(got), tt.want)
			}
		})
	}
}
