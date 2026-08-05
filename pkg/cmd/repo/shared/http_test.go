package shared

import (
	"context"
	"fmt"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRESTClient(t *testing.T, reg *httpmock.Registry) *githubrest.Client {
	t.Helper()
	client, err := httpmock.RESTClientFunc(reg)("github.com")
	require.NoError(t, err)
	return client
}

func TestForkRepoReturnsErrorWhenForkIsNotPossible(t *testing.T) {
	// Given our API returns 202 with a Fork that is the same as
	// the repo we provided
	repoName := "test-repo"
	ownerLogin := "test-owner"
	stubbedForkResponse := repositoryV3{
		Name: repoName,
		Owner: struct{ Login string }{
			Login: ownerLogin,
		},
	}

	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.REST("POST", fmt.Sprintf("repos/%s/%s/forks", ownerLogin, repoName)),
		httpmock.StatusJSONResponse(202, stubbedForkResponse),
	)

	client := newTestRESTClient(t, reg)

	// When we fork the repo
	_, err := ForkRepo(context.Background(), client, ghrepo.New(ownerLogin, repoName), ownerLogin, "", false)

	// Then it provides a useful error message
	require.Equal(t, fmt.Errorf("%s/%s cannot be forked. A single user account cannot own both a parent and fork.", ownerLogin, repoName), err)
}

func TestListLicenseTemplatesReturnsLicenses(t *testing.T) {
	hostname := "api.github.com"
	httpStubs := func(reg *httpmock.Registry) {
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
	}
	wantLicenses := []api.License{
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
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)
	defer reg.Verify(t)

	client := newTestRESTClient(t, reg)

	gotLicenses, err := RepoLicenses(context.Background(), client, hostname)

	assert.NoError(t, err, "Expected no error while fetching /licenses")
	assert.Equal(t, wantLicenses, gotLicenses, "Licenses fetched is not as expected")
}

func TestLicenseTemplateReturnsLicense(t *testing.T) {
	licenseTemplateName := "mit"
	hostname := "api.github.com"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("licenses/%v", licenseTemplateName)),
			httpmock.StringResponse(`{
						"key": "mit",
						"name": "MIT License",
						"spdx_id": "MIT",
						"url": "https://api.github.com/licenses/mit",
						"node_id": "MDc6TGljZW5zZTEz",
						"html_url": "http://choosealicense.com/licenses/mit/",
						"description": "A short and simple permissive license with conditions only requiring preservation of copyright and license notices. Licensed works, modifications, and larger works may be distributed under different terms and without source code.",
						"implementation": "Create a text file (typically named LICENSE or LICENSE.txt) in the root of your source code and copy the text of the license into the file. Replace [year] with the current year and [fullname] with the name (or names) of the copyright holders.",
						"permissions": [
							"commercial-use",
							"modifications",
							"distribution",
							"private-use"
						],
						"conditions": [
							"include-copyright"
						],
						"limitations": [
							"liability",
							"warranty"
						],
						"body": "MIT License\n\nCopyright (c) [year] [fullname]\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\nof this software and associated documentation files (the \"Software\"), to deal\nin the Software without restriction, including without limitation the rights\nto use, copy, modify, merge, publish, distribute, sublicense, and/or sell\ncopies of the Software, and to permit persons to whom the Software is\nfurnished to do so, subject to the following conditions:\n\nThe above copyright notice and this permission notice shall be included in all\ncopies or substantial portions of the Software.\n\nTHE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR\nIMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,\nFITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE\nAUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER\nLIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,\nOUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE\nSOFTWARE.\n",
						"featured": true
						}`,
			))
	}
	wantLicense := &api.License{
		Key:            "mit",
		Name:           "MIT License",
		SPDXID:         "MIT",
		URL:            "https://api.github.com/licenses/mit",
		NodeID:         "MDc6TGljZW5zZTEz",
		HTMLURL:        "http://choosealicense.com/licenses/mit/",
		Description:    "A short and simple permissive license with conditions only requiring preservation of copyright and license notices. Licensed works, modifications, and larger works may be distributed under different terms and without source code.",
		Implementation: "Create a text file (typically named LICENSE or LICENSE.txt) in the root of your source code and copy the text of the license into the file. Replace [year] with the current year and [fullname] with the name (or names) of the copyright holders.",
		Permissions: []string{
			"commercial-use",
			"modifications",
			"distribution",
			"private-use",
		},
		Conditions: []string{
			"include-copyright",
		},
		Limitations: []string{
			"liability",
			"warranty",
		},
		Body:     "MIT License\n\nCopyright (c) [year] [fullname]\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\nof this software and associated documentation files (the \"Software\"), to deal\nin the Software without restriction, including without limitation the rights\nto use, copy, modify, merge, publish, distribute, sublicense, and/or sell\ncopies of the Software, and to permit persons to whom the Software is\nfurnished to do so, subject to the following conditions:\n\nThe above copyright notice and this permission notice shall be included in all\ncopies or substantial portions of the Software.\n\nTHE SOFTWARE IS PROVIDED \"AS IS\", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR\nIMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,\nFITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE\nAUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER\nLIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,\nOUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE\nSOFTWARE.\n",
		Featured: true,
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)
	defer reg.Verify(t)

	client := newTestRESTClient(t, reg)

	gotLicenseTemplate, err := RepoLicense(context.Background(), client, hostname, licenseTemplateName)

	assert.NoError(t, err, fmt.Sprintf("Expected no error while fetching /licenses/%v", licenseTemplateName))
	assert.Equal(t, wantLicense, gotLicenseTemplate, fmt.Sprintf("License \"%v\" fetched is not as expected", licenseTemplateName))
}

func TestLicenseTemplateReturnsErrorWhenLicenseTemplateNotFound(t *testing.T) {
	licenseTemplateName := "invalid-license"
	hostname := "api.github.com"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("licenses/%v", licenseTemplateName)),
			httpmock.StatusStringResponse(404, heredoc.Doc(`
			{
				"message": "Not Found",
				"documentation_url": "https://docs.github.com/rest/licenses/licenses#get-a-license",
				"status": "404"
			}`)),
		)
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)
	defer reg.Verify(t)

	client := newTestRESTClient(t, reg)

	_, err := RepoLicense(context.Background(), client, hostname, licenseTemplateName)

	assert.Error(t, err, fmt.Sprintf("Expected error while fetching /licenses/%v", licenseTemplateName))
}

func TestListGitIgnoreTemplatesReturnsGitIgnoreTemplates(t *testing.T) {
	hostname := "api.github.com"
	httpStubs := func(reg *httpmock.Registry) {
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
	}
	wantGitIgnoreTemplates := []string{
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
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)
	defer reg.Verify(t)

	client := newTestRESTClient(t, reg)

	gotGitIgnoreTemplates, err := RepoGitIgnoreTemplates(context.Background(), client, hostname)

	assert.NoError(t, err, "Expected no error while fetching /gitignore/templates")
	assert.Equal(t, wantGitIgnoreTemplates, gotGitIgnoreTemplates, "GitIgnore templates fetched is not as expected")
}

func TestGitIgnoreTemplateReturnsGitIgnoreTemplate(t *testing.T) {
	gitIgnoreTemplateName := "Go"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("gitignore/templates/%v", gitIgnoreTemplateName)),
			httpmock.StringResponse(`{
						"name": "Go",
						"source": "# If you prefer the allow list template instead of the deny list, see community template:\n# https://github.com/github/gitignore/blob/main/community/Golang/Go.AllowList.gitignore\n#\n# Binaries for programs and plugins\n*.exe\n*.exe~\n*.dll\n*.so\n*.dylib\n\n# Test binary, built with go test -c\n*.test\n\n# Output of the go coverage tool, specifically when used with LiteIDE\n*.out\n\n# Dependency directories (remove the comment below to include it)\n# vendor/\n\n# Go workspace file\ngo.work\ngo.work.sum\n\n# env file\n.env\n"
						}`,
			))
	}
	wantGitIgnoreTemplate := &api.GitIgnore{
		Name:   "Go",
		Source: "# If you prefer the allow list template instead of the deny list, see community template:\n# https://github.com/github/gitignore/blob/main/community/Golang/Go.AllowList.gitignore\n#\n# Binaries for programs and plugins\n*.exe\n*.exe~\n*.dll\n*.so\n*.dylib\n\n# Test binary, built with go test -c\n*.test\n\n# Output of the go coverage tool, specifically when used with LiteIDE\n*.out\n\n# Dependency directories (remove the comment below to include it)\n# vendor/\n\n# Go workspace file\ngo.work\ngo.work.sum\n\n# env file\n.env\n",
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)
	defer reg.Verify(t)

	client := newTestRESTClient(t, reg)

	gotGitIgnoreTemplate, err := RepoGitIgnoreTemplate(context.Background(), client, "api.github.com", gitIgnoreTemplateName)

	assert.NoError(t, err, fmt.Sprintf("Expected no error while fetching /gitignore/templates/%v", gitIgnoreTemplateName))
	assert.Equal(t, wantGitIgnoreTemplate, gotGitIgnoreTemplate, fmt.Sprintf("GitIgnore template \"%v\" fetched is not as expected", gitIgnoreTemplateName))
}

func TestGitIgnoreTemplateReturnsErrorWhenGitIgnoreTemplateNotFound(t *testing.T) {
	gitIgnoreTemplateName := "invalid-gitignore"
	httpStubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("gitignore/templates/%v", gitIgnoreTemplateName)),
			httpmock.StatusStringResponse(404, heredoc.Doc(`
			{
				"message": "Not Found",
				"documentation_url": "https://docs.github.com/v3/gitignore",
				"status": "404"
			}`)),
		)
	}

	reg := &httpmock.Registry{}
	httpStubs(reg)
	defer reg.Verify(t)

	client := newTestRESTClient(t, reg)

	_, err := RepoGitIgnoreTemplate(context.Background(), client, "api.github.com", gitIgnoreTemplateName)

	assert.Error(t, err, fmt.Sprintf("Expected error while fetching /gitignore/templates/%v", gitIgnoreTemplateName))
}
