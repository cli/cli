//go:build updateable

package ghcmd

// `updateable` is a build tag set in the gh formula within homebrew/homebrew-core
// and is used to control whether users are notified of newer GitHub CLI releases.
//
// Currently, updaterEnabled needs to be set to 'cli/cli' as it affects where
// update.CheckForUpdate() checks for releases. It is unclear to what extent
// this updaterEnabled is being used by unofficial forks or builds, so we decided
// to leave it available for injection as a string variable for now.
//
// Development builds do not generate update messages by default.
//
// For more information, see:
// - the Homebrew formula for gh: <https://github.com/Homebrew/homebrew-core/blob/master/Formula/g/gh.rb>.
// - a discussion about adding this build tag: <https://github.com/cli/cli/pull/11024#discussion_r2107597618>.
var updaterEnabled = "cli/cli"
