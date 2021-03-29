package shared

import (
	"fmt"

	"github.com/cli/cli/pkg/iostreams"
)

func RenderRunHeader(cs *iostreams.ColorScheme, run Run, ago, prNumber string) string {
	title := fmt.Sprintf("%s %s%s",
		cs.Bold(run.HeadBranch), run.Name, prNumber)
	symbol, symbolColor := Symbol(cs, run.Status, run.Conclusion)
	id := cs.Cyanf("%d", run.ID)

	header := ""
	header += fmt.Sprintf("%s %s · %s\n", symbolColor(symbol), title, id)
	header += fmt.Sprintf("Triggered via %s %s", run.Event, ago)

	return header
}
