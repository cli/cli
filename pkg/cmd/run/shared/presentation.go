package shared

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
)

func RenderRunHeader(cs *iostreams.ColorScheme, run Run, ago, prNumber string, attempt uint64) string {
	title := fmt.Sprintf("%s %s%s",
		cs.Bold(run.HeadBranch), run.WorkflowName(), prNumber)
	symbol, symbolColor := Symbol(cs, run.Status, run.Conclusion)
	id := cs.Cyanf("%d", run.ID)

	attemptLabel := ""
	if attempt > 0 {
		attemptLabel = fmt.Sprintf(" (Attempt #%d)", attempt)
	}

	header := ""
	header += fmt.Sprintf("%s %s · %s%s\n", symbolColor(symbol), title, id, attemptLabel)
	header += fmt.Sprintf("Triggered via %s %s", run.Event, ago)

	return header
}

func RenderJobs(cs *iostreams.ColorScheme, jobs []Job, verbose bool) string {
	lines := []string{}
	for _, job := range jobs {
		elapsed := job.CompletedAt.Sub(job.StartedAt)
		elapsedStr := fmt.Sprintf(" in %s", elapsed)
		if elapsed < 0 {
			elapsedStr = ""
		}
		symbol, symbolColor := Symbol(cs, job.Status, job.Conclusion)
		id := cs.Cyanf("%d", job.ID)
		lines = append(lines, fmt.Sprintf("%s %s%s (ID %s)", symbolColor(symbol), cs.Bold(job.Name), elapsedStr, id))
		if verbose || IsFailureState(job.Conclusion) {
			for _, step := range job.Steps {
				stepSymbol, stepSymColor := Symbol(cs, step.Status, step.Conclusion)
				lines = append(lines, fmt.Sprintf("  %s %s", stepSymColor(stepSymbol), step.Name))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func RenderAnnotations(cs *iostreams.ColorScheme, annotations []Annotation) string {
	lines := []string{}

	for _, a := range annotations {
		lines = append(lines, fmt.Sprintf("%s %s", AnnotationSymbol(cs, a), a.Message))
		lines = append(lines, cs.Grayf("%s: %s#%d\n", a.JobName, a.Path, a.StartLine))
	}

	return strings.Join(lines, "\n")
}
