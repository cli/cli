package githubtemplate

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"
)

func TestFind(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "gh-cli")
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		rootDir string
		name    string
	}
	tests := []struct {
		name    string
		prepare []string
		args    args
		want    []string
	}{
		{
			name: "Template in root",
			prepare: []string{
				"README.md",
				"ISSUE_TEMPLATE",
				"issue_template.md",
				"issue_template.txt",
				"pull_request_template.md",
			},
			args: args{
				rootDir: tmpdir,
				name:    "ISSUE_TEMPLATE",
			},
			want: []string{
				path.Join(tmpdir, "issue_template.md"),
			},
		},
		{
			name: "Template in .github takes precedence",
			prepare: []string{
				"ISSUE_TEMPLATE.md",
				".github/issue_template.md",
			},
			args: args{
				rootDir: tmpdir,
				name:    "ISSUE_TEMPLATE",
			},
			want: []string{
				path.Join(tmpdir, ".github/issue_template.md"),
			},
		},
		{
			name: "Template in docs",
			prepare: []string{
				"README.md",
				"docs/issue_template.md",
			},
			args: args{
				rootDir: tmpdir,
				name:    "ISSUE_TEMPLATE",
			},
			want: []string{
				path.Join(tmpdir, "docs/issue_template.md"),
			},
		},
		{
			name: "Multiple templates",
			prepare: []string{
				".github/ISSUE_TEMPLATE/nope.md",
				".github/PULL_REQUEST_TEMPLATE.md",
				".github/PULL_REQUEST_TEMPLATE/one.md",
				".github/PULL_REQUEST_TEMPLATE/two.md",
				".github/PULL_REQUEST_TEMPLATE/three.md",
				"docs/pull_request_template.md",
			},
			args: args{
				rootDir: tmpdir,
				name:    "PuLl_ReQuEsT_TeMpLaTe",
			},
			want: []string{
				path.Join(tmpdir, ".github/PULL_REQUEST_TEMPLATE/one.md"),
				path.Join(tmpdir, ".github/PULL_REQUEST_TEMPLATE/three.md"),
				path.Join(tmpdir, ".github/PULL_REQUEST_TEMPLATE/two.md"),
			},
		},
		{
			name: "Empty multiple templates directory",
			prepare: []string{
				".github/issue_template.md",
				".github/issue_template/.keep",
			},
			args: args{
				rootDir: tmpdir,
				name:    "ISSUE_TEMPLATE",
			},
			want: []string{
				path.Join(tmpdir, ".github/issue_template.md"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, p := range tt.prepare {
				fp := path.Join(tmpdir, p)
				_ = os.MkdirAll(path.Dir(fp), 0700)
				file, err := os.Create(fp)
				if err != nil {
					t.Fatal(err)
				}
				file.Close()
			}

			if got := Find(tt.args.rootDir, tt.args.name); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Find() = %v, want %v", got, tt.want)
			}
		})
		os.RemoveAll(tmpdir)
	}
}

func TestExtractName(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "gh-cli")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

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
			_ = ioutil.WriteFile(tmpfile.Name(), []byte(tt.prepare), 0600)
			if got := ExtractName(tt.args.filePath); got != tt.want {
				t.Errorf("ExtractName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractContents(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "gh-cli")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

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
			_ = ioutil.WriteFile(tmpfile.Name(), []byte(tt.prepare), 0600)
			if got := ExtractContents(tt.args.filePath); string(got) != tt.want {
				t.Errorf("ExtractContents() = %v, want %v", string(got), tt.want)
			}
		})
	}
}
