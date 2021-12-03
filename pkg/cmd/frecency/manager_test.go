package frecency

// import (
// 	"testing"

// 	"github.com/cli/cli/v2/internal/config"
// 	"github.com/cli/cli/v2/pkg/iostreams"
// )

// func newTestManager(dir string, io *iostreams.IOStreams) *Manager {
// 	m := &Manager{
// 		config: config.NewBlankConfig(),
// 		io:     io,
// 	}
// 	return m
// }

// func TestManager_update(t *testing.T) {
// 	tempDir := t.TempDir()
// 	m := newTestManager(tempDir, nil)
// 	m.initFrecentFile(tempDir)
// 	m.RecordAccess("issue", Identifier{Repository: "cli/cli", Number: 4567})

// }
