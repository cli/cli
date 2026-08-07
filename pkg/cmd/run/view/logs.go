package view

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
)

type logFetcher interface {
	GetLog() (io.ReadCloser, error)
}

type zipLogFetcher struct {
	File *zip.File
}

func (f *zipLogFetcher) GetLog() (io.ReadCloser, error) {
	return f.File.Open()
}

type apiLogFetcher struct {
	httpClient *http.Client

	repo  ghrepo.Interface
	jobID int64
}

func (f *apiLogFetcher) GetLog() (io.ReadCloser, error) {
	logPath, err := safeurl.JoinPath("repos", f.repo.RepoOwner(), f.repo.RepoName(), "actions", "jobs", strconv.FormatInt(f.jobID, 10), "logs")
	if err != nil {
		return nil, err
	}

	// The response body is the log stream, which is handed to the caller rather than decoded.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(f.httpClient).Request(f.repo.RepoHost(), http.MethodGet, logPath.String(), nil)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("log not found: %v", f.jobID)
		}
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, api.UnexpectedStatusError(resp)
	}

	return resp.Body, nil
}

// logSegment represents a segment of a log trail, which can be either an entire
// job log or an individual step log.
type logSegment struct {
	job     *shared.Job
	step    *shared.Step
	fetcher logFetcher
}

// maxAPILogFetchers is the maximum allowed number of API log fetchers that can
// be assigned to log segments. This is a heuristic limit to avoid overwhelming
// the API with too many requests when fetching logs for a run with many jobs or
// steps.
const maxAPILogFetchers = 25

var errTooManyAPILogFetchers = errors.New("too many missing logs")

// populateLogSegments populates log segments from the provided jobs and data
// available in the given ZIP archive map. Any missing logs will be assigned a
// log fetcher that retrieves logs from the API.
//
// For example, if there's no step log available in the ZIP archive, the entire
// job log will be selected as a log segment.
//
// Note that, as heuristic approach, we only allow a limited number of API log
// fetchers to be assigned. This is to avoid overwhelming the API with too many
// requests.
func populateLogSegments(httpClient *http.Client, repo ghrepo.Interface, jobs []shared.Job, zlm *zipLogMap, onlyFailed bool) ([]logSegment, error) {
	segments := make([]logSegment, 0, len(jobs))

	apiLogFetcherCount := 0
	for _, job := range jobs {
		if shared.IsSkipped(job.Conclusion) {
			continue
		}

		if onlyFailed && !shared.IsFailureState(job.Conclusion) {
			continue
		}

		stepLogAvailable := slices.ContainsFunc(job.Steps, func(step shared.Step) bool {
			_, ok := zlm.forStep(job.ID, step.Number)
			return ok
		})

		// If at least one step log is available, we populate the segments with
		// them and don't use the entire job log.
		if stepLogAvailable {
			steps := slices.Clone(job.Steps)
			sort.Sort(steps)
			for _, step := range steps {
				if onlyFailed && !shared.IsFailureState(step.Conclusion) {
					continue
				}

				zf, ok := zlm.forStep(job.ID, step.Number)
				if !ok {
					// We have no step log in the zip archive, but there's nothing we can do
					// about that because there is no API endpoint to fetch step logs.
					continue
				}

				segments = append(segments, logSegment{
					job:     &job,
					step:    &step,
					fetcher: &zipLogFetcher{File: zf},
				})
			}
			continue
		}

		segment := logSegment{job: &job}
		if zf, ok := zlm.forJob(job.ID); ok {
			segment.fetcher = &zipLogFetcher{File: zf}
		} else {
			segment.fetcher = &apiLogFetcher{
				httpClient: httpClient,
				repo:       repo,
				jobID:      job.ID,
			}
			apiLogFetcherCount++
		}
		segments = append(segments, segment)

		if apiLogFetcherCount > maxAPILogFetchers {
			return nil, errTooManyAPILogFetchers
		}
	}

	return segments, nil
}

// zipLogMap is a map of job and step logs available in a ZIP archive.
type zipLogMap struct {
	jobs  map[int64]*zip.File
	steps map[string]*zip.File
}

func newZipLogMap() *zipLogMap {
	return &zipLogMap{
		jobs:  make(map[int64]*zip.File),
		steps: make(map[string]*zip.File),
	}
}

func (l *zipLogMap) forJob(jobID int64) (*zip.File, bool) {
	f, ok := l.jobs[jobID]
	return f, ok
}

func (l *zipLogMap) forStep(jobID int64, stepNumber int) (*zip.File, bool) {
	logFetcherKey := fmt.Sprintf("%d/%d", jobID, stepNumber)
	f, ok := l.steps[logFetcherKey]
	return f, ok
}

func (l *zipLogMap) addStep(jobID int64, stepNumber int, zf *zip.File) {
	logFetcherKey := fmt.Sprintf("%d/%d", jobID, stepNumber)
	l.steps[logFetcherKey] = zf
}

func (l *zipLogMap) addJob(jobID int64, zf *zip.File) {
	l.jobs[jobID] = zf
}

// getZipLogMap populates a logs struct with appropriate log fetchers based on
// the provided zip file and list of jobs.
//
// The structure of zip file is expected to be as:
//
//	zip/
//	├── jobname1/
//	│   ├── 1_stepname.txt
//	│   ├── 2_anotherstepname.txt
//	│   ├── 3_stepstepname.txt
//	│   └── 4_laststepname.txt
//	├── jobname2/
//	|   ├── 1_stepname.txt
//	|   └── 2_somestepname.txt
//	├── 0_jobname1.txt
//	├── 1_jobname2.txt
//	└── -9999999999_jobname3.txt
//
// The function iterates through the list of jobs and tries to find the matching
// log file in the ZIP archive.
//
// The top-level .txt files include the logs for an entire job run. Note that
// the prefixed number is either:
//   - An ordinal and cannot be mapped to the corresponding job's ID.
//   - A negative integer which is the ID of the job in the old Actions service.
//     The service right now tries to get logs and use an ordinal in a loop.
//     However, if it doesn't get the logs, it falls back to an old service
//     where the ID can apparently be negative.
func getZipLogMap(rlz *zip.Reader, jobs []shared.Job) *zipLogMap {
	zlm := newZipLogMap()

	for _, job := range jobs {
		// So far we haven't yet encountered a ZIP containing both top-level job
		// logs (i.e. the normal and the legacy .txt files). However, it's still
		// possible. Therefore, we prioritise the normal log over the legacy one.
		if zf := matchFileInZIPArchive(rlz, jobLogFilenameRegexp(job)); zf != nil {
			zlm.addJob(job.ID, zf)
		} else if zf := matchFileInZIPArchive(rlz, legacyJobLogFilenameRegexp(job)); zf != nil {
			zlm.addJob(job.ID, zf)
		}

		for _, step := range job.Steps {
			if zf := matchFileInZIPArchive(rlz, stepLogFilenameRegexp(job, step)); zf != nil {
				zlm.addStep(job.ID, step.Number, zf)
			}
		}
	}

	return zlm
}

const JOB_NAME_MAX_LENGTH = 90

func getJobNameForLogFilename(name string) string {
	// As described in https://github.com/cli/cli/issues/5011#issuecomment-1570713070, there are a number of steps
	// the server can take when producing the downloaded zip file that can result in a mismatch between the job name
	// and the filename in the zip including:
	//  * Removing characters in the job name that aren't allowed in file paths
	//  * Truncating names that are too long for zip files
	//  * Adding collision deduplicating numbers for jobs with the same name
	//
	// We are hesitant to duplicate all the server logic due to the fragility but it may be unavoidable. Currently, we:
	// * Strip `/` which occur when composite action job names are constructed of the form `<JOB_NAME`> / <ACTION_NAME>`
	// * Truncate long job names
	//
	sanitizedJobName := strings.ReplaceAll(name, "/", "")
	sanitizedJobName = strings.ReplaceAll(sanitizedJobName, ":", "")
	sanitizedJobName = truncateAsUTF16(sanitizedJobName, JOB_NAME_MAX_LENGTH)
	return sanitizedJobName
}

// A job run log file is a top-level .txt file whose name starts with an ordinal
// number; e.g., "0_jobname.txt".
func jobLogFilenameRegexp(job shared.Job) *regexp.Regexp {
	sanitizedJobName := getJobNameForLogFilename(job.Name)
	re := fmt.Sprintf(`^\d+_%s\.txt$`, regexp.QuoteMeta(sanitizedJobName))
	return regexp.MustCompile(re)
}

// A legacy job run log file is a top-level .txt file whose name starts with a
// negative number which is the ID of the run; e.g., "-2147483648_jobname.txt".
func legacyJobLogFilenameRegexp(job shared.Job) *regexp.Regexp {
	sanitizedJobName := getJobNameForLogFilename(job.Name)
	re := fmt.Sprintf(`^-\d+_%s\.txt$`, regexp.QuoteMeta(sanitizedJobName))
	return regexp.MustCompile(re)
}

func stepLogFilenameRegexp(job shared.Job, step shared.Step) *regexp.Regexp {
	sanitizedJobName := getJobNameForLogFilename(job.Name)
	re := fmt.Sprintf(`^%s\/%d_.*\.txt$`, regexp.QuoteMeta(sanitizedJobName), step.Number)
	return regexp.MustCompile(re)
}

/*
If you're reading this comment by necessity, I'm sorry and if you're reading it for fun, you're welcome, you weirdo.

What is the length of this string "a😅😅"? If you said 9 you'd be right. If you said 3 or 5 you might also be right!

Here's a summary:

	"a" takes 1 byte (`\x61`)
	"😅" takes 4 `bytes` (`\xF0\x9F\x98\x85`)
	"a😅😅" therefore takes 9 `bytes`
	In Go `len("a😅😅")` is 9 because the `len` builtin counts `bytes`
	In Go `len([]rune("a😅😅"))` is 3 because each `rune` is 4 `bytes` so each character fits within a `rune`
	In C# `"a😅😅".Length` is 5 because `.Length` counts `Char` objects, `Chars` hold 2 bytes, and "😅" takes 2 Chars.

But wait, what does C# have to do with anything? Well the server is running C#. Which server? The one that serves log
files to us in `.zip` format of course! When the server is constructing the zip file to avoid running afoul of a 260
byte zip file path length limitation, it applies transformations to various strings in order to limit their length.
In C#, the server truncates strings with this function:

	public static string TruncateAfter(string str, int max)
	{
		string result = str.Length > max ? str.Substring(0, max) : str;
		result = result.Trim();
		return result;
	}

This seems like it would be easy enough to replicate in Go but as we already discovered, the length of a string isn't
as obvious as it might seem. Since C# uses UTF-16 encoding for strings, and Go uses UTF-8 encoding and represents
characters by runes (which are an alias of int32) we cannot simply slice the string without any further consideration.
Instead, we need to encode the string as UTF-16 bytes, slice it and then decode it back to UTF-8.

Interestingly, in C# length and substring both act on the Char type so it's possible to slice into the middle of
a visual, "representable" character. For example we know `"a😅😅".Length` = 5 (1+2+2) and therefore Substring(0,4)
results in the final character being cleaved in two, resulting in "a😅�". Since our int32 runes are being encoded as
2 uint16 elements, we also mimic this behaviour by slicing into the UTF-16 encoded string.

Here's a program you can put into a dotnet playground to see how C# works:

	using System;
	public class Program {
	  public static void Main() {
	    string s = "a😅😅";
	    Console.WriteLine("{0} {1}", s.Length, s);
	    string t = TruncateAfter(s, 4);
	    Console.WriteLine("{0} {1}", t.Length, t);
	  }
	  public static string TruncateAfter(string str, int max) {
	    string result = str.Length > max ? str.Substring(0, max) : str;
	    return result.Trim();
	  }
	}

This will output:
5 a😅😅
4 a😅�
*/
func truncateAsUTF16(str string, max int) string {
	// Encode the string to UTF-16 to count code units
	utf16Encoded := utf16.Encode([]rune(str))
	if len(utf16Encoded) > max {
		// Decode back to UTF-8 up to the max length
		str = string(utf16.Decode(utf16Encoded[:max]))
	}
	return strings.TrimSpace(str)
}

func matchFileInZIPArchive(zr *zip.Reader, re *regexp.Regexp) *zip.File {
	for _, file := range zr.File {
		if re.MatchString(file.Name) {
			return file
		}
	}
	return nil
}
