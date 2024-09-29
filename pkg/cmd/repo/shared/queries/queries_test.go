package queries

import (
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
					Key:  "mit",
					Name: "MIT License",
				},
				{
					Key:  "lgpl-3.0",
					Name: "GNU Lesser General Public License v3.0",
				},
				{
					Key:  "mpl-2.0",
					Name: "Mozilla Public License 2.0",
				},
				{
					Key:  "agpl-3.0",
					Name: "GNU Affero General Public License v3.0",
				},
				{
					Key:  "unlicense",
					Name: "The Unlicense",
				},
				{
					Key:  "apache-2.0",
					Name: "Apache License 2.0",
				},
				{
					Key:  "gpl-3.0",
					Name: "GNU General Public License v3.0",
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
