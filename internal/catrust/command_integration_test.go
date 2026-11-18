//go:build integration

package catrust

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandRejectsDistrustedCA(t *testing.T) {
	root := issueCertificate(t, "command test root", nil, nil, true)
	leaf := issueCertificate(t, "command test server", &root, nil, false)
	server := certificateServer(t, []testCertificate{leaf}, 0)
	work := t.TempDir()
	caFile := filepath.Join(work, "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: root.cert.Raw,
	}), 0o600))

	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	sourcePath := filepath.Join(repo, "internal", "catrust", "transport.go")
	source, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	// Substitute one real entry only in the test build; no real CA private key is needed.
	entry := `"8d417db2dd8bf5e3084d1e3f196d583849d81bdd4c00c70b9d39369e96b8c782": true,`
	require.Equal(t, 1, strings.Count(string(source), entry))
	replacement := fmt.Sprintf("%q: true,", fmt.Sprintf("%x", sha256.Sum256(root.cert.RawSubjectPublicKeyInfo)))
	overlaySource := filepath.Join(work, "transport.go")
	require.NoError(t, os.WriteFile(overlaySource, []byte(strings.Replace(string(source), entry, replacement, 1)), 0o600))
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{sourcePath: overlaySource}})
	require.NoError(t, err)
	overlayPath := filepath.Join(work, "overlay.json")
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))

	buildEnv := slices.DeleteFunc(os.Environ(), func(value string) bool {
		return strings.HasPrefix(value, "GH_") || strings.HasPrefix(value, "GITHUB_") ||
			strings.HasPrefix(value, "OP_") || strings.HasPrefix(value, "SSH_AUTH_SOCK=")
	})
	buildEnv = append(buildEnv, "GOENV=off", "GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local", "GOCACHEPROG=")
	commandEnv := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + work,
		"GH_CONFIG_DIR=" + filepath.Join(work, "config"),
		"GH_TOKEN=not-a-secret",
		"GH_ENTERPRISE_TOKEN=not-a-secret",
		"GH_TELEMETRY=false",
		"GH_NO_UPDATE_NOTIFIER=1",
		"GH_NO_EXTENSION_UPDATE_NOTIFIER=1",
		"SSL_CERT_FILE=" + caFile,
		"SSL_CERT_DIR=" + t.TempDir(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for _, tt := range []struct {
		name   string
		denied bool
	}{
		{name: "valid connection control"},
		{name: "listed issuer is rejected", denied: true},
	} {
		require.True(t, t.Run(tt.name, func(t *testing.T) {
			binary := filepath.Join(work, strings.ReplaceAll(tt.name, " ", "-")+".exe")
			args := []string{"build", "-mod=readonly", "-buildvcs=false", "-o", binary}
			if tt.denied {
				args = append(args, "-overlay", overlayPath)
			}
			args = append(args, "./cmd/gh")
			build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), args...)
			build.Dir, build.Env = repo, buildEnv
			output, err := build.CombinedOutput()
			require.NoError(t, err, "%s", output)

			command := exec.CommandContext(ctx, binary, "api", server.URL)
			command.Dir, command.Env = work, commandEnv
			output, err = command.CombinedOutput()
			if tt.denied {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
				assert.Equal(t, 1, exitErr.ExitCode())
				assert.Contains(t, string(output), "certificate chain uses a distrusted CA")
			} else {
				require.NoError(t, err, "%s", output)
				assert.Equal(t, "ok", strings.TrimSpace(string(output)))
			}
		}))
	}
}
