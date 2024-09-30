package queries

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestListLicenseTemplates(t *testing.T) {
	tests := []struct {
		name         string
		httpStubs    func(t *testing.T, reg *httpmock.Registry)
		hostname     string
		wantLicenses []api.License
		wantErr      bool
		wantErrMsg   string
		httpClient   func() (*http.Client, error)
	}{
		{
			name: "happy path",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "licenses"),
					httpmock.StringResponse(`[
						{
							"key": "mit",
							"name": "MIT License",
							"spdx_id": "MIT",
							"url": "https://api.github.com/licenses/mit",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "lgpl-3.0",
							"name": "GNU Lesser General Public License v3.0",
							"spdx_id": "LGPL-3.0",
							"url": "https://api.github.com/licenses/lgpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "mpl-2.0",
							"name": "Mozilla Public License 2.0",
							"spdx_id": "MPL-2.0",
							"url": "https://api.github.com/licenses/mpl-2.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "agpl-3.0",
							"name": "GNU Affero General Public License v3.0",
							"spdx_id": "AGPL-3.0",
							"url": "https://api.github.com/licenses/agpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "unlicense",
							"name": "The Unlicense",
							"spdx_id": "Unlicense",
							"url": "https://api.github.com/licenses/unlicense",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "apache-2.0",
							"name": "Apache License 2.0",
							"spdx_id": "Apache-2.0",
							"url": "https://api.github.com/licenses/apache-2.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "gpl-3.0",
							"name": "GNU General Public License v3.0",
							"spdx_id": "GPL-3.0",
							"url": "https://api.github.com/licenses/gpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						}
						]`,
					))
			},
			hostname: "api.github.com",
			wantLicenses: []api.License{
				{
					Key:            "mit",
					Name:           "MIT License",
					SPDXID:         "MIT",
					URL:            "https://api.github.com/licenses/mit",
					NodeID:         "MDc6TGljZW5zZW1pdA==",
					HTMLURL:        "",
					Description:    "",
					Implementation: "",
					Permissions:    nil,
					Conditions:     nil,
					Limitations:    nil,
					Body:           "",
				},
				{
					Key:            "lgpl-3.0",
					Name:           "GNU Lesser General Public License v3.0",
					SPDXID:         "LGPL-3.0",
					URL:            "https://api.github.com/licenses/lgpl-3.0",
					NodeID:         "MDc6TGljZW5zZW1pdA==",
					HTMLURL:        "",
					Description:    "",
					Implementation: "",
					Permissions:    nil,
					Conditions:     nil,
					Limitations:    nil,
					Body:           "",
				},
				{
					Key:            "mpl-2.0",
					Name:           "Mozilla Public License 2.0",
					SPDXID:         "MPL-2.0",
					URL:            "https://api.github.com/licenses/mpl-2.0",
					NodeID:         "MDc6TGljZW5zZW1pdA==",
					HTMLURL:        "",
					Description:    "",
					Implementation: "",
					Permissions:    nil,
					Conditions:     nil,
					Limitations:    nil,
					Body:           "",
				},
				{
					Key:            "agpl-3.0",
					Name:           "GNU Affero General Public License v3.0",
					SPDXID:         "AGPL-3.0",
					URL:            "https://api.github.com/licenses/agpl-3.0",
					NodeID:         "MDc6TGljZW5zZW1pdA==",
					HTMLURL:        "",
					Description:    "",
					Implementation: "",
					Permissions:    nil,
					Conditions:     nil,
					Limitations:    nil,
					Body:           "",
				},
				{
					Key:            "unlicense",
					Name:           "The Unlicense",
					SPDXID:         "Unlicense",
					URL:            "https://api.github.com/licenses/unlicense",
					NodeID:         "MDc6TGljZW5zZW1pdA==",
					HTMLURL:        "",
					Description:    "",
					Implementation: "",
					Permissions:    nil,
					Conditions:     nil,
					Limitations:    nil,
					Body:           "",
				},
				{
					Key:            "apache-2.0",
					Name:           "Apache License 2.0",
					SPDXID:         "Apache-2.0",
					URL:            "https://api.github.com/licenses/apache-2.0",
					NodeID:         "MDc6TGljZW5zZW1pdA==",
					HTMLURL:        "",
					Description:    "",
					Implementation: "",
					Permissions:    nil,
					Conditions:     nil,
					Limitations:    nil,
					Body:           "",
				},
				{
					Key:            "gpl-3.0",
					Name:           "GNU General Public License v3.0",
					SPDXID:         "GPL-3.0",
					URL:            "https://api.github.com/licenses/gpl-3.0",
					NodeID:         "MDc6TGljZW5zZW1pdA==",
					HTMLURL:        "",
					Description:    "",
					Implementation: "",
					Permissions:    nil,
					Conditions:     nil,
					Limitations:    nil,
					Body:           "",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(t, reg)
		}
		tt.httpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		client, _ := tt.httpClient()
		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			gotLicenses, err := ListLicenseTemplates(client, tt.hostname)
			if !tt.wantErr {
				assert.NoError(t, err, "Expected no error while fetching /licenses")
			}
			if tt.wantErr {
				assert.Error(t, err, "Expected error while fetching /licenses")
			}
			assert.Equal(t, tt.wantLicenses, gotLicenses, "Licenses fetched is not as expected")
		})
	}
}

func TestListGitIgnoreTemplates(t *testing.T) {
	tests := []struct {
		name                   string
		httpStubs              func(t *testing.T, reg *httpmock.Registry)
		wantGitIgnoreTemplates []string
		wantErr                bool
		wantErrMsg             string
		httpClient             func() (*http.Client, error)
	}{
		{
			name: "happy path",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "gitignore/templates"),
					httpmock.StringResponse(`[
						"AL",
						"Actionscript",
						"Ada",
						"Agda",
						"Android",
						"AppEngine",
						"AppceleratorTitanium",
						"ArchLinuxPackages",
						"Autotools",
						"Ballerina",
						"C",
						"C++",
						"CFWheels",
						"CMake",
						"CUDA",
						"CakePHP",
						"ChefCookbook",
						"Clojure",
						"CodeIgniter",
						"CommonLisp",
						"Composer",
						"Concrete5",
						"Coq",
						"CraftCMS",
						"D"
						]`,
					))
			},
			wantGitIgnoreTemplates: []string{
				"AL",
				"Actionscript",
				"Ada",
				"Agda",
				"Android",
				"AppEngine",
				"AppceleratorTitanium",
				"ArchLinuxPackages",
				"Autotools",
				"Ballerina",
				"C",
				"C++",
				"CFWheels",
				"CMake",
				"CUDA",
				"CakePHP",
				"ChefCookbook",
				"Clojure",
				"CodeIgniter",
				"CommonLisp",
				"Composer",
				"Concrete5",
				"Coq",
				"CraftCMS",
				"D",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(t, reg)
		}
		tt.httpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		client, _ := tt.httpClient()
		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			gotGitIgnoreTemplates, err := ListGitIgnoreTemplates(client, "api.github.com")
			if !tt.wantErr {
				assert.NoError(t, err, "Expected no error while fetching /gitignore/templates")
			}
			if tt.wantErr {
				assert.Error(t, err, "Expected error while fetching /gitignore/templates")
			}
			assert.Equal(t, tt.wantGitIgnoreTemplates, gotGitIgnoreTemplates, "GitIgnore templates fetched is not as expected")
		})
	}
}

func TestGitIgnoreTemplate(t *testing.T) {
	tests := []struct {
		name                  string
		httpStubs             func(t *testing.T, reg *httpmock.Registry)
		gitIgnoreTemplateName string
		wantGitIgnoreTemplate api.GitIgnore
		wantErr               bool
		wantErrMsg            string
		httpClient            func() (*http.Client, error)
	}{
		{
			name:                  "happy path",
			gitIgnoreTemplateName: "Go",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "gitignore/templates/Go"),
					httpmock.StringResponse(`{
						"name": "Go",
						"source": "# If you prefer the allow list template instead of the deny list, see community template:\n# https://github.com/github/gitignore/blob/main/community/Golang/Go.AllowList.gitignore\n#\n# Binaries for programs and plugins\n*.exe\n*.exe~\n*.dll\n*.so\n*.dylib\n\n# Test binary, built with go test -c\n*.test\n\n# Output of the go coverage tool, specifically when used with LiteIDE\n*.out\n\n# Dependency directories (remove the comment below to include it)\n# vendor/\n\n# Go workspace file\ngo.work\ngo.work.sum\n\n# env file\n.env\n"
						}`,
					))
			},
			wantGitIgnoreTemplate: api.GitIgnore{
				Name:   "Go",
				Source: "# If you prefer the allow list template instead of the deny list, see community template:\n# https://github.com/github/gitignore/blob/main/community/Golang/Go.AllowList.gitignore\n#\n# Binaries for programs and plugins\n*.exe\n*.exe~\n*.dll\n*.so\n*.dylib\n\n# Test binary, built with go test -c\n*.test\n\n# Output of the go coverage tool, specifically when used with LiteIDE\n*.out\n\n# Dependency directories (remove the comment below to include it)\n# vendor/\n\n# Go workspace file\ngo.work\ngo.work.sum\n\n# env file\n.env\n",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(t, reg)
		}
		tt.httpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		client, _ := tt.httpClient()
		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			gotGitIgnoreTemplate, err := GitIgnoreTemplate(client, "api.github.com", tt.gitIgnoreTemplateName)
			if !tt.wantErr {
				assert.NoError(t, err, fmt.Sprintf("Expected no error while fetching /gitignore/templates/%v", tt.gitIgnoreTemplateName))
			}
			if tt.wantErr {
				assert.Error(t, err, fmt.Sprintf("Expected error while fetching /gitignore/templates/%v", tt.gitIgnoreTemplateName))
			}
			assert.Equal(t, tt.wantGitIgnoreTemplate, gotGitIgnoreTemplate, fmt.Sprintf("GitIgnore template \"%v\" fetched is not as expected", tt.gitIgnoreTemplateName))
		})
	}
}
