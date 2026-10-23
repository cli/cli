package evidence

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sessionOrderCases(t *testing.T) {
	t.Run("atomic case sequence", func(t *testing.T) {
		workspace, err := filepath.Abs("uncreated-session-order-workspace")
		require.NoError(t, err)
		runs := func(caseID string, phases ...string) []sessionRun {
			result := make([]sessionRun, 0, len(phases))
			for index, phase := range phases {
				id := fmt.Sprintf("%s-%d", phase, index)
				result = append(result, sessionRun{ID: id, Phase: phase,
					Title: "  Requested " + id + " title  ", RunDirectory: caseID + "/" + id})
			}
			return result
		}
		for _, tc := range []struct {
			name   string
			modify func(*sessionManifest)
			err    string
		}{
			{name: "multiple cases stay in requested order"},
			{name: "multiple preparations validations and cleanups", modify: func(m *sessionManifest) {
				m.Cases[0].Runs = runs(m.Cases[0].ID, "setup", "setup", "exercise", "validation", "validation", "cleanup", "cleanup")
			}},
			{name: "exercise-only coverage", modify: func(m *sessionManifest) {
				m.Cases = m.Cases[:1]
				m.Cases[0].Runs = runs(m.Cases[0].ID, "exercise")
			}},
			{name: "validation-only coverage", modify: func(m *sessionManifest) {
				m.Cases = m.Cases[:1]
				m.Cases[0].Runs = runs(m.Cases[0].ID, "validation", "validation")
			}},
			{name: "setup-only coverage", modify: func(m *sessionManifest) {
				m.Cases = m.Cases[:1]
				m.Cases[0].Runs = runs(m.Cases[0].ID, "setup", "setup")
			}},
			{name: "cleanup-only coverage", modify: func(m *sessionManifest) {
				m.Cases = m.Cases[:1]
				m.Cases[0].Runs = runs(m.Cases[0].ID, "cleanup")
			}},
			{name: "validation and cleanup without an unrequested exercise", modify: func(m *sessionManifest) {
				m.Cases = m.Cases[:1]
				m.Cases[0].Runs = runs(m.Cases[0].ID, "validation", "cleanup")
			}},
			{name: "no unrequested validation is inserted", modify: func(m *sessionManifest) {
				m.Cases[0].Runs = runs(m.Cases[0].ID, "exercise", "cleanup")
			}},
			{name: "absolute capture path need not exist", modify: func(m *sessionManifest) {
				m.Cases[0].Runs[0].RunDirectory = filepath.Join(workspace, "separate-capture")
			}},
			{name: "reused capture within one case", modify: func(m *sessionManifest) {
				m.Cases[0].Runs[1].RunDirectory = m.Cases[0].Runs[0].RunDirectory
			}, err: "reuses capture"},
			{name: "reused capture across cases", modify: func(m *sessionManifest) {
				m.Cases[1].Runs[0].RunDirectory = m.Cases[0].Runs[0].RunDirectory
			}, err: "reuses capture"},
			{name: "relative lexical aliases cannot relabel a capture", modify: func(m *sessionManifest) {
				m.Cases[0].Runs[1].RunDirectory = "./z-case/unused/../setup-0/."
			}, err: "reuses capture"},
			{name: "relative and absolute paths cannot relabel a capture", modify: func(m *sessionManifest) {
				m.Cases[1].Runs[0].RunDirectory = filepath.Join(workspace, "z-case", "setup-0")
			}, err: "reuses capture"},
			{name: "absolute lexical aliases cannot relabel a capture", modify: func(m *sessionManifest) {
				m.Cases[1].Runs[0].RunDirectory = workspace + string(filepath.Separator) + filepath.FromSlash("z-case/unused/../setup-0/.")
			}, err: "reuses capture"},
			{name: "workspace is lexically canonicalized too", modify: func(m *sessionManifest) {
				m.Workspace += string(filepath.Separator) + filepath.FromSlash("unused/..")
				m.Cases[1].Runs[0].RunDirectory = filepath.Join(workspace, "z-case", "setup-0")
			}, err: "reuses capture"},
			{name: "multiple exercises are separate behaviors", modify: func(m *sessionManifest) {
				m.Cases[0].Runs = runs(m.Cases[0].ID, "setup", "exercise", "exercise", "validation")
			}, err: "at most one exercise"},
			{name: "setup after run", modify: func(m *sessionManifest) {
				m.Cases[0].Runs = runs(m.Cases[0].ID, "exercise", "setup")
			}, err: "phase order"},
			{name: "run after validation", modify: func(m *sessionManifest) {
				m.Cases[0].Runs = runs(m.Cases[0].ID, "validation", "exercise")
			}, err: "phase order"},
			{name: "validation after cleanup", modify: func(m *sessionManifest) {
				m.Cases[0].Runs = runs(m.Cases[0].ID, "cleanup", "validation")
			}, err: "phase order"},
			{name: "invalid phase", modify: func(m *sessionManifest) {
				m.Cases[0].Runs[0].Phase = "run"
			}, err: "invalid phase"},
			{name: "missing phase", modify: func(m *sessionManifest) {
				m.Cases[0].Runs[0].Phase = ""
			}, err: "invalid phase"},
			{name: "phase names are not rewritten", modify: func(m *sessionManifest) {
				m.Cases[0].Runs[0].Phase = " Setup "
			}, err: "invalid phase"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				manifest := sessionManifest{SchemaVersion: 1, ID: "sequence", Title: "Original session title",
					Source: "Preserve the requested coverage.", Workspace: workspace}
				for _, id := range []string{"z-case", "a-case"} {
					manifest.Cases = append(manifest.Cases, sessionCase{ID: id, Title: "  Original " + id + " title  ",
						Runs: runs(id, "setup", "exercise", "validation", "cleanup")})
				}
				if tc.modify != nil {
					tc.modify(&manifest)
				}
				original, err := json.MarshalIndent(manifest, "", "\t")
				require.NoError(t, err)
				err = validateCaseSequence(manifest)
				if tc.err != "" {
					require.ErrorContains(t, err, tc.err)
				} else {
					require.NoError(t, err)
				}
				after, err := json.MarshalIndent(manifest, "", "\t")
				require.NoError(t, err)
				require.Equal(t, original, after, "validation must not sort, rename, or add requested runs")
			})
		}
	})
	t.Run("execution chronology", func(t *testing.T) {
		origin := time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC)
		stamp := func(offset time.Duration) string { return origin.Add(offset).Format(time.RFC3339Nano) }
		interval := func(chapter *sessionChapter, start, finish time.Duration) {
			chapter.Evidence.StartedAt, chapter.Evidence.FinishedAt = stamp(start), stamp(finish)
		}
		legacy := func(chapter *sessionChapter) {
			chapter.Evidence.StartedAt, chapter.Evidence.FinishedAt = "", ""
		}
		for _, tc := range []struct {
			name     string
			modify   func(*[]sessionChapter)
			verified bool
			warning  bool
			err      string
		}{
			{name: "fresh monotonic runs across cases", verified: true},
			{name: "empty contract hashes do not imply reused captures", verified: true, modify: func(chapters *[]sessionChapter) {
				for index := range *chapters {
					(*chapters)[index].Evidence.ContractSHA256 = ""
				}
			}},
			{name: "copied capture under distinct paths", modify: func(chapters *[]sessionChapter) {
				(*chapters)[1].Evidence.ContractSHA256 = (*chapters)[0].Evidence.ContractSHA256
			}, err: "reuses captured contract SHA256"},
			{name: "copied capture across cases", modify: func(chapters *[]sessionChapter) {
				(*chapters)[3].Evidence.ContractSHA256 = (*chapters)[0].Evidence.ContractSHA256
			}, err: "reuses captured contract SHA256"},
			{name: "copied legacy capture under distinct paths", warning: true, modify: func(chapters *[]sessionChapter) {
				legacy(&(*chapters)[0])
				legacy(&(*chapters)[1])
				(*chapters)[1].Evidence.ContractSHA256 = (*chapters)[0].Evidence.ContractSHA256
			}, err: "reuses captured contract SHA256"},
			{name: "copied legacy capture across cases", warning: true, modify: func(chapters *[]sessionChapter) {
				legacy(&(*chapters)[0])
				legacy(&(*chapters)[3])
				(*chapters)[3].Evidence.ContractSHA256 = (*chapters)[0].Evidence.ContractSHA256
			}, err: "reuses captured contract SHA256"},
			{name: "legacy gap does not conceal a copied capture", warning: true, modify: func(chapters *[]sessionChapter) {
				legacy(&(*chapters)[1])
				(*chapters)[2].Evidence.ContractSHA256 = (*chapters)[0].Evidence.ContractSHA256
			}, err: "reuses captured contract SHA256"},
			{name: "known capture cannot reuse a legacy contract", warning: true, modify: func(chapters *[]sessionChapter) {
				legacy(&(*chapters)[0])
				(*chapters)[3].Evidence.ContractSHA256 = (*chapters)[0].Evidence.ContractSHA256
			}, err: "reuses captured contract SHA256"},
			{name: "legacy capture cannot reuse a known contract", modify: func(chapters *[]sessionChapter) {
				legacy(&(*chapters)[3])
				(*chapters)[3].Evidence.ContractSHA256 = (*chapters)[0].Evidence.ContractSHA256
			}, err: "reuses captured contract SHA256"},
			{name: "empty legacy hashes only warn", warning: true, modify: func(chapters *[]sessionChapter) {
				for index := range *chapters {
					legacy(&(*chapters)[index])
					(*chapters)[index].Evidence.ContractSHA256 = ""
				}
			}},
			{name: "overview hashes are not execution identities", verified: true, modify: func(chapters *[]sessionChapter) {
				overview := (*chapters)[0]
				overview.Kind = "overview"
				*chapters = append([]sessionChapter{overview}, (*chapters)...)
				*chapters = append(*chapters, overview)
			}},
			{name: "nanosecond intervals and adjacent boundaries", verified: true, modify: func(chapters *[]sessionChapter) {
				for index := range *chapters {
					interval(&(*chapters)[index], time.Duration(index), time.Duration(index+1))
				}
			}},
			{name: "gaps between real executions are allowed", verified: true, modify: func(chapters *[]sessionChapter) {
				for index := range *chapters {
					start := time.Duration(index*2) * time.Second
					interval(&(*chapters)[index], start, start+time.Second)
				}
			}},
			{name: "offset timestamps compare as instants", verified: true, modify: func(chapters *[]sessionChapter) {
				zone := time.FixedZone("fixture", -7*60*60)
				(*chapters)[1].Evidence.StartedAt = origin.Add(time.Second).In(zone).Format(time.RFC3339Nano)
				(*chapters)[1].Evidence.FinishedAt = origin.Add(2 * time.Second).In(zone).Format(time.RFC3339Nano)
			}},
			{name: "untyped chapters still represent executions", verified: true, modify: func(chapters *[]sessionChapter) {
				for index := range *chapters {
					(*chapters)[index].Kind = ""
				}
			}},
			{name: "legacy captures have only one explicit warning", warning: true, modify: func(chapters *[]sessionChapter) {
				for index := range *chapters {
					legacy(&(*chapters)[index])
				}
			}},
			{name: "mixed legacy captures are not verified", warning: true, modify: func(chapters *[]sessionChapter) {
				legacy(&(*chapters)[0])
				legacy(&(*chapters)[2])
			}},
			{name: "missing start is not legacy", modify: func(chapters *[]sessionChapter) {
				(*chapters)[0].Evidence.StartedAt = ""
			}, err: "both startedAt and finishedAt"},
			{name: "missing finish is not legacy", modify: func(chapters *[]sessionChapter) {
				(*chapters)[0].Evidence.FinishedAt = ""
			}, err: "both startedAt and finishedAt"},
			{name: "malformed start", modify: func(chapters *[]sessionChapter) {
				(*chapters)[0].Evidence.StartedAt = "not a timestamp"
			}, err: "invalid startedAt"},
			{name: "malformed finish", modify: func(chapters *[]sessionChapter) {
				(*chapters)[0].Evidence.FinishedAt = "2026-01-02 03:04:06"
			}, err: "invalid finishedAt"},
			{name: "whitespace is not a legacy timestamp", modify: func(chapters *[]sessionChapter) {
				(*chapters)[0].Evidence.StartedAt, (*chapters)[0].Evidence.FinishedAt = " ", " "
			}, err: "invalid startedAt"},
			{name: "zero interval", modify: func(chapters *[]sessionChapter) {
				interval(&(*chapters)[0], 0, 0)
			}, err: "finishedAt must be after startedAt"},
			{name: "negative interval", modify: func(chapters *[]sessionChapter) {
				interval(&(*chapters)[0], 0, -time.Second)
			}, err: "finishedAt must be after startedAt"},
			{name: "overlap within a case", modify: func(chapters *[]sessionChapter) {
				(*chapters)[1].Evidence.StartedAt = stamp(time.Second / 2)
			}, err: "executions overlap"},
			{name: "overlap across cases", modify: func(chapters *[]sessionChapter) {
				(*chapters)[3].Evidence.StartedAt = stamp(5 * time.Second / 2)
			}, err: "executions overlap"},
			{name: "disjoint intervals in reverse order", modify: func(chapters *[]sessionChapter) {
				(*chapters)[0], (*chapters)[1] = (*chapters)[1], (*chapters)[0]
			}, err: "not in the requested order"},
			{name: "batched execution cannot become sequential presentation", modify: func(chapters *[]sessionChapter) {
				for index, slot := range []int{0, 2, 4, 1, 3, 5} {
					start := time.Duration(slot) * time.Second
					interval(&(*chapters)[index], start, start+time.Second)
				}
			}, err: "not in the requested order"},
			{name: "overlap across a legacy gap", warning: true, modify: func(chapters *[]sessionChapter) {
				legacy(&(*chapters)[1])
				(*chapters)[2].Evidence.StartedAt = stamp(time.Second / 2)
			}, err: "executions overlap"},
			{name: "reordering across a legacy gap", warning: true, modify: func(chapters *[]sessionChapter) {
				interval(&(*chapters)[0], 4*time.Second, 5*time.Second)
				legacy(&(*chapters)[1])
			}, err: "not in the requested order"},
			{name: "legacy prefix does not conceal known overlap", warning: true, modify: func(chapters *[]sessionChapter) {
				legacy(&(*chapters)[0])
				(*chapters)[2].Evidence.StartedAt = stamp(3 * time.Second / 2)
			}, err: "executions overlap"},
			{name: "overview chapters are not execution evidence", verified: true, modify: func(chapters *[]sessionChapter) {
				*chapters = append([]sessionChapter{
					{Kind: "overview"},
					(*chapters)[0],
					{Kind: "overview", Evidence: report{StartedAt: "not an execution"}},
					{Kind: "overview", Evidence: report{StartedAt: stamp(8 * time.Second), FinishedAt: stamp(-time.Second)}},
				}, (*chapters)[1:]...)
			}},
			{name: "overview alone does not prove execution", modify: func(chapters *[]sessionChapter) {
				*chapters = []sessionChapter{{Kind: "overview"}}
			}},
			{name: "no executions does not prove order", modify: func(chapters *[]sessionChapter) {
				*chapters = nil
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var chapters []sessionChapter
				for _, caseID := range []string{"z-case", "a-case"} {
					for _, phase := range []string{"setup", "exercise", "validation"} {
						start := time.Duration(len(chapters)) * time.Second
						chapter := sessionChapter{Kind: "run", CaseID: caseID, CaseTitle: "Original " + caseID,
							RunID: phase, Phase: phase, Title: "Original " + phase, RunDirectory: caseID + "/" + phase,
							Evidence: report{CaseID: caseID, Mode: "exact", CaseStatus: "passed", CaptureStatus: "complete",
								ContractSHA256: fmt.Sprintf("%064x", len(chapters)+1)}}
						interval(&chapter, start, start+time.Second)
						chapters = append(chapters, chapter)
					}
				}
				chapters[1].Evidence.CaseStatus = "failed"
				chapters[3].Evidence.Mode = "explore"
				if tc.modify != nil {
					tc.modify(&chapters)
				}
				original, err := json.Marshal(chapters)
				require.NoError(t, err)
				verified, warnings, err := verifySessionChronology(chapters)
				if tc.err != "" {
					require.ErrorContains(t, err, tc.err)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, tc.verified, verified)
				if tc.warning {
					require.Len(t, warnings, 1)
					require.Contains(t, warnings[0], "execution order cannot be verified")
					require.Contains(t, warnings[0], "legacy")
				} else {
					require.Empty(t, warnings)
				}
				after, err := json.Marshal(chapters)
				require.NoError(t, err)
				require.Equal(t, original, after, "chronology checks must not rewrite verdicts or rearrange chapters")
			})
		}
	})
}
