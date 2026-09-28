package evidence

import (
	"fmt"
	"path/filepath"
	"time"
)

func validateCaseSequence(manifest sessionManifest) error {
	phases := map[string]int{"setup": 0, "exercise": 1, "validation": 2, "cleanup": 3}
	captures := map[string]string{}
	for _, item := range manifest.Cases {
		previous, exercises := -1, 0
		for _, run := range item.Runs {
			label := fmt.Sprintf("case %q run %q", item.ID, run.ID)
			phase, valid := phases[run.Phase]
			if !valid {
				return fmt.Errorf("%s has invalid phase %q; expected setup, exercise, validation, or cleanup", label, run.Phase)
			}
			if run.Phase == "exercise" {
				exercises++
				if exercises > 1 {
					return fmt.Errorf("case %q must contain at most one exercise invocation", item.ID)
				}
			}
			if phase < previous {
				return fmt.Errorf("%s violates phase order: setup -> exercise -> validation -> cleanup", label)
			}
			previous = phase
			path, err := filepath.Abs(sessionPath(manifest.Workspace, run.RunDirectory))
			if err != nil {
				return fmt.Errorf("%s has an invalid capture path: %w", label, err)
			}
			if prior, duplicate := captures[path]; duplicate {
				return fmt.Errorf("%s reuses capture %q already used by %s", label, run.RunDirectory, prior)
			}
			captures[path] = label
		}
	}
	return nil
}

func verifySessionChronology(chapters []sessionChapter) (bool, []string, error) {
	var warnings []string
	var previousFinish time.Time
	previousLabel := ""
	contracts := map[string]string{}
	for _, chapter := range chapters {
		if chapter.Kind == "overview" {
			continue
		}
		label := fmt.Sprintf("case %q run %q", chapter.CaseID, chapter.RunID)
		if hash := chapter.Evidence.ContractSHA256; hash != "" {
			if prior, duplicate := contracts[hash]; duplicate {
				return false, warnings, fmt.Errorf("%s reuses captured contract SHA256 %q already used by %s", label, hash, prior)
			}
			contracts[hash] = label
		}
		start, finish := chapter.Evidence.StartedAt, chapter.Evidence.FinishedAt
		if start == "" && finish == "" {
			if len(warnings) == 0 {
				warnings = append(warnings, "execution order cannot be verified: one or more legacy captures lack both startedAt and finishedAt timestamps.")
			}
			continue
		}
		if start == "" || finish == "" {
			return false, warnings, fmt.Errorf("%s must provide both startedAt and finishedAt timestamps", label)
		}
		startedAt, err := time.Parse(time.RFC3339Nano, start)
		if err != nil {
			return false, warnings, fmt.Errorf("%s has invalid startedAt: %w", label, err)
		}
		finishedAt, err := time.Parse(time.RFC3339Nano, finish)
		if err != nil {
			return false, warnings, fmt.Errorf("%s has invalid finishedAt: %w", label, err)
		}
		if !finishedAt.After(startedAt) {
			return false, warnings, fmt.Errorf("%s finishedAt must be after startedAt", label)
		}
		if previousLabel != "" && startedAt.Before(previousFinish) {
			return false, warnings, fmt.Errorf("%s starts before %s finished; executions overlap or are not in the requested order", label, previousLabel)
		}
		previousFinish, previousLabel = finishedAt, label
	}
	return previousLabel != "" && len(warnings) == 0, warnings, nil
}
