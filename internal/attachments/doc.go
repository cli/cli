// Package attachments implements the --attach flag.
// It uploads local user assets to GitHub and optionally rewrites user provided
// markdown references to point at the remote asset path rather than the local
// path. In the absence of a markdown reference, a new reference is appended to
// the user provided markdown.
//
// An upload cannot be undone. Callers must ensure that the returned markdown
// is written to a resource (issue, PR, etc.).
//
// A caller must build the Uploader before it prompts for anything, so a token
// or permission that cannot upload stops the command before an editor opens.
// It must call UploadAndAttach after every possible prompt has been exercised,
// not before, because nothing that can cancel may follow an upload.
//
//	attachFlag := attachments.AddFlag(cmd)
//	...
//	attachmentArgs, err := attachFlag.UserAssets()
//	...
//
//	// repositoryID and viewerPermission come from the lookup the command
//	// already makes.
//	uploader, err := attachments.NewUploader(
//		httpClient, token, host, repositoryID, viewerPermission)
//	...
//
//	// Every reasonable cancellation possible belongs here.
//
//	md, uploaded, err := uploader.UploadAndAttach(ctx, md, attachmentArgs)
//	if uploaded == 0 {
//		return err
//	}
//
//	// Write md to target, then report err alongside whatever the write
//	// returned.
//
// UploadAndAttach reports how many assets were successfully uploaded. The
// caller must write the markdown when that count is above zero, including after
// a partial failure, because what did upload must be referenced by something.
// At zero nothing is stranded and nothing is written.
package attachments
