package status

import (
	"testing"
)

// BenchmarkShouldExclude_MapLookup benchmarks the O(1) map-based ShouldExclude
// using the post-fix implementation via NewStatusGetter.
func BenchmarkShouldExclude_MapLookup(b *testing.B) {
	excludeList := []string{
		"owner/repo-a", "owner/repo-b", "owner/repo-c",
		"owner/repo-d", "owner/repo-e",
	}
	sg := &StatusGetter{
		Exclude:    excludeList,
		excludeSet: buildExcludeSet(excludeList),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sg.ShouldExclude("owner/repo-e") // worst-case: last element
	}
}

// BenchmarkShouldExclude_LinearScan benchmarks the O(n) linear-scan baseline
// to provide a before/after comparison.
func BenchmarkShouldExclude_LinearScan(b *testing.B) {
	excludeList := []string{
		"owner/repo-a", "owner/repo-b", "owner/repo-c",
		"owner/repo-d", "owner/repo-e",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shouldExcludeLinear(excludeList, "owner/repo-e")
	}
}

// shouldExcludeLinear is the pre-fix O(n) implementation, preserved for
// benchmarking purposes only.
func shouldExcludeLinear(excludes []string, repo string) bool {
	for _, exclude := range excludes {
		if repo == exclude {
			return true
		}
	}
	return false
}

// buildExcludeSet constructs the O(1) map from a slice of repo names.
func buildExcludeSet(excludes []string) map[string]struct{} {
	m := make(map[string]struct{}, len(excludes))
	for _, r := range excludes {
		m[r] = struct{}{}
	}
	return m
}
