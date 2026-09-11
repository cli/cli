package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type versionInfoTemplate struct {
	StringFileInfo struct {
		LegalCopyright string `json:"LegalCopyright"`
	} `json:"StringFileInfo"`
}

func TestVersionInfoTemplateIncludesLegalCopyright(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("versioninfo.template.json"))
	require.NoError(t, err)

	var template versionInfoTemplate
	err = json.Unmarshal(content, &template)
	require.NoError(t, err)

	assert.NotEmpty(t, template.StringFileInfo.LegalCopyright)
}

func TestGenWinresScriptSetsFixedFileVersionFields(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("gen-winres.ps1"))
	require.NoError(t, err)

	script := string(content)
	assert.Contains(t, script, "-ver-major")
	assert.Contains(t, script, "-ver-minor")
	assert.Contains(t, script, "-ver-patch")
	assert.Contains(t, script, "-ver-build")
	assert.Contains(t, script, "-product-ver-major")
	assert.Contains(t, script, "-product-ver-minor")
	assert.Contains(t, script, "-product-ver-patch")
	assert.Contains(t, script, "-product-ver-build")
}
