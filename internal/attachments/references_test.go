package attachments

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	pngURL = "https://github.com/user-attachments/assets/11111111-1111-1111-1111-111111111111"
	mp4URL = "https://github.com/user-attachments/assets/22222222-2222-2222-2222-222222222222"
)

func pngArg() attachmentArg {
	return attachmentArg{Path: "./login.png", URL: pngURL, Alt: "login"}
}

func mp4Arg() attachmentArg {
	return attachmentArg{Path: "./repro.mp4", URL: mp4URL, Alt: "repro.mp4", RendersAsPlayer: true}
}

func TestAttachAssetsToMarkdown(t *testing.T) {
	absPNG, err := filepath.Abs("./login.png")
	require.NoError(t, err)

	tests := []struct {
		name           string
		markdown       string
		attachmentArgs []attachmentArg
		wantMarkdown   string
		wantToAppend   []attachmentArg
		wantErr        string
	}{
		{
			name:           "image embed on its own line keeps its markdown",
			markdown:       "Before\n\n![the login screen](./login.png)\n\nAfter",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "Before\n\n![the login screen](" + pngURL + ")\n\nAfter",
		},
		{
			name:           "image embed inline keeps its markdown",
			markdown:       "The screen ![the login screen](./login.png) looks wrong.",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "The screen ![the login screen](" + pngURL + ") looks wrong.",
		},
		{
			name:           "image link stays a link",
			markdown:       "See [the screenshot](./login.png) for detail.",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "See [the screenshot](" + pngURL + ") for detail.",
		},
		{
			name:           "image never referenced is reported for appending",
			markdown:       "No references here.",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "No references here.",
			wantToAppend:   []attachmentArg{pngArg()},
		},

		{
			name:           "video embed alone in its own paragraph becomes a bare url",
			markdown:       "Watch:\n\n![screen recording](./repro.mp4)\n\nEnd",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "Watch:\n\n" + mp4URL + "\n\nEnd",
		},
		{
			name:           "video embed alone in a paragraph padded with spaces becomes a bare url",
			markdown:       "  ![screen recording](./repro.mp4)  ",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "  " + mp4URL + "  ",
		},
		{
			name:           "video embed inline degrades to a link",
			markdown:       "The failure ![screen recording](./repro.mp4) is reproducible.",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "The failure [screen recording](" + mp4URL + ") is reproducible.",
		},
		{
			// Dropping the "!" would leave the preceding one touching the
			// "[", re-forming an embed of a video, which renders as a broken
			// image. Escaping it keeps the bang literal.
			name:           "a bang before a degraded video is escaped so no embed re-forms",
			markdown:       "Watch this!![demo](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   `Watch this\![demo](` + mp4URL + ")",
		},
		{
			// The preceding bang is already a literal, so escaping it again
			// would emit a backslash and re-form the embed.
			name:           "a bang that is already escaped is left as it is",
			markdown:       `Watch this\!![demo](./repro.mp4)`,
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   `Watch this\![demo](` + mp4URL + ")",
		},
		{
			name:           "a bang before a degraded video at the very start of the markdown is escaped",
			markdown:       "!![demo](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   `\![demo](` + mp4URL + ")",
		},
		{
			// An image keeps its "!", so its neighbour cannot pair with
			// anything and only the destination moves.
			name:           "a bang before an image embed is left alone",
			markdown:       "Look at this!![the login screen](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "Look at this!![the login screen](" + pngURL + ")",
		},
		{
			// A "]" cannot pair with the following "[" into anything, so the
			// ordinary deletion is correct here.
			name:           "a bracket before a degraded video needs no escaping",
			markdown:       "See ]![demo](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "See ][demo](" + mp4URL + ")",
		},
		{
			name:           "a degraded video at the very start of the markdown keeps the rest of the line",
			markdown:       "![demo](./repro.mp4) is the recording.",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "[demo](" + mp4URL + ") is the recording.",
		},
		{
			name:           "video link stays a link",
			markdown:       "See [the recording](./repro.mp4) for detail.",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "See [the recording](" + mp4URL + ") for detail.",
		},
		{
			name:           "video never referenced is reported for appending",
			markdown:       "No references here.",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "No references here.",
			wantToAppend:   []attachmentArg{mp4Arg()},
		},

		// A path that looks like a reference but is not one.
		{
			name:           "path inside a fenced code block is left alone",
			markdown:       "```\n![example](./login.png)\n```\n\n![the real one](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "```\n![example](./login.png)\n```\n\n![the real one](" + pngURL + ")",
		},
		{
			name:           "path inside an inline code byteRange is left alone",
			markdown:       "Write `![alt](./login.png)` to embed ![the real one](./login.png) here.",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "Write `![alt](./login.png)` to embed ![the real one](" + pngURL + ") here.",
		},
		{
			// The code byteRange holds a "](path)" that a bracket outside it could
			// pair with, so scanning has to skip the byteRange's bytes or it
			// rewrites inside the byteRange and leaves the real reference dead.
			name:           "a code byteRange that could pair with an earlier bracket is left alone",
			markdown:       "[x `](./login.png)` y [the real one](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "[x `](./login.png)` y [the real one](" + pngURL + ")",
		},
		{
			name:           "an indented code block is left alone",
			markdown:       "Example:\n\n    ![example](./login.png)\n\n![the real one](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "Example:\n\n    ![example](./login.png)\n\n![the real one](" + pngURL + ")",
		},
		{
			name:           "a remote url is not touched",
			markdown:       "![hosted](https://example.com/login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![hosted](https://example.com/login.png)",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			name:           "a local path nobody attached is left as written",
			markdown:       "![unattached](./other.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![unattached](./other.png)",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			name:           "an anchor is not treated as a path",
			markdown:       "[jump](#login.png)",
			attachmentArgs: []attachmentArg{{Path: "#login.png", URL: pngURL}},
			wantMarkdown:   "[jump](#login.png)",
			wantToAppend:   []attachmentArg{{Path: "#login.png", URL: pngURL}},
		},
		{
			name:           "a scheme without slashes is not treated as a path",
			markdown:       "[write me](mailto:me@example.com)",
			attachmentArgs: []attachmentArg{{Path: "mailto:me@example.com", URL: pngURL}},
			wantMarkdown:   "[write me](mailto:me@example.com)",
			wantToAppend:   []attachmentArg{{Path: "mailto:me@example.com", URL: pngURL}},
		},
		{
			name:           "a protocol-relative url is not treated as a path",
			markdown:       "![hosted](//example.com/login.png)",
			attachmentArgs: []attachmentArg{{Path: "/example.com/login.png", URL: pngURL}},
			wantMarkdown:   "![hosted](//example.com/login.png)",
			wantToAppend:   []attachmentArg{{Path: "/example.com/login.png", URL: pngURL}},
		},
		{
			// The one letter that a scheme test has to let through, since it is
			// how Windows names a volume.
			name:           "a windows volume path is treated as a path",
			markdown:       `![the login screen](C:/Users/me/login.png)`,
			attachmentArgs: []attachmentArg{{Path: "C:/Users/me/login.png", URL: pngURL}},
			wantMarkdown:   "![the login screen](" + pngURL + ")",
		},
		{
			// goldmark keeps whitespace inside angle brackets, because that is
			// the only way to write a name holding a space. The padded
			// destination and the attached file are two different names.
			name:           "a padded angle destination is not the file it pads",
			markdown:       "![the login screen](< login.png >)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![the login screen](< login.png >)",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			name:           "a padded angle destination is rewritten when it names an attached file",
			markdown:       "![the login screen](< login.png >)",
			attachmentArgs: []attachmentArg{{Path: " login.png ", URL: pngURL, Alt: "login"}},
			wantMarkdown:   "![the login screen](" + pngURL + ")",
		},
		{
			name:           "a padded angle definition is rewritten when it names an attached file",
			markdown:       "![the login screen][shot]\n\n[shot]: < login.png >",
			attachmentArgs: []attachmentArg{{Path: " login.png ", URL: pngURL, Alt: "login"}},
			wantMarkdown:   "![the login screen][shot]\n\n[shot]: " + pngURL,
		},

		// One file, several references.
		{
			name:           "the same file referenced twice is rewritten in both places",
			markdown:       "![first](./login.png)\n\nSome prose.\n\n![second](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![first](" + pngURL + ")\n\nSome prose.\n\n![second](" + pngURL + ")",
		},
		{
			name:           "the same file referenced twice in one paragraph is rewritten in both places",
			markdown:       "Compare ![first](./login.png) with ![second](./login.png).",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "Compare ![first](" + pngURL + ") with ![second](" + pngURL + ").",
		},
		{
			name:           "an embed and a link to the same file are told apart",
			markdown:       "![embed](./login.png) and [link](./login.png).",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![embed](" + pngURL + ") and [link](" + pngURL + ").",
		},

		// Alt text.
		{
			name:           "alt text written in the markdown wins over the flag",
			markdown:       "![the login screen showing an auth error](./login.png)",
			attachmentArgs: []attachmentArg{{Path: "./login.png", URL: pngURL, Alt: "alt from the flag"}},
			wantMarkdown:   "![the login screen showing an auth error](" + pngURL + ")",
		},
		{
			name:           "a video degraded to a link keeps the alt text written in the markdown",
			markdown:       "Here ![the crash, recorded](./repro.mp4) it is.",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "Here [the crash, recorded](" + mp4URL + ") it is.",
		},
		{
			name:           "a video degraded to a link falls back to the file name when the markdown has no alt text",
			markdown:       "Here ![](./repro.mp4) it is.",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "Here [repro.mp4](" + mp4URL + ") it is.",
		},
		{
			name:           "a file name that would end the label early is escaped",
			markdown:       "Here ![](./a]b.mp4) it is.",
			attachmentArgs: []attachmentArg{{Path: "./a]b.mp4", URL: mp4URL, Alt: `a]b.mp4`, RendersAsPlayer: true}},
			wantMarkdown:   `Here [a\]b.mp4](` + mp4URL + ") it is.",
		},
		{
			name:           "formatting inside the alt text survives",
			markdown:       "![the *login* screen](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![the *login* screen](" + pngURL + ")",
		},

		// Ways markdown spells a path.
		{
			name:           "an absolute path in the markdown matches a relative asset",
			markdown:       "![the login screen](" + absPNG + ")",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![the login screen](" + pngURL + ")",
		},
		{
			name:           "an angle bracketed path with spaces is matched and replaced whole",
			markdown:       "![shot](<./Screenshot 2026-08-10 at 5.38.10 PM.png>)",
			attachmentArgs: []attachmentArg{{Path: "./Screenshot 2026-08-10 at 5.38.10 PM.png", URL: pngURL}},
			wantMarkdown:   "![shot](" + pngURL + ")",
		},
		{
			name:           "a percent encoded path with spaces is matched",
			markdown:       "![shot](./Screenshot%202026-08-10%20at%205.38.10%20PM.png)",
			attachmentArgs: []attachmentArg{{Path: "./Screenshot 2026-08-10 at 5.38.10 PM.png", URL: pngURL}},
			wantMarkdown:   "![shot](" + pngURL + ")",
		},
		{
			name:           "a backslash escaped path is matched",
			markdown:       `![shot](./login\(1\).png)`,
			attachmentArgs: []attachmentArg{{Path: "./login(1).png", URL: pngURL}},
			wantMarkdown:   "![shot](" + pngURL + ")",
		},
		{
			// A backslash is itself escapable punctuation, so "\\" in the
			// markdown is one literal backslash in the filename.
			name:           "a path containing an escaped backslash is matched",
			markdown:       `![shot](./a\\b.png)`,
			attachmentArgs: []attachmentArg{{Path: `./a\b.png`, URL: pngURL}},
			wantMarkdown:   "![shot](" + pngURL + ")",
		},
		{
			name:           "a title survives the rewrite",
			markdown:       `![the login screen](./login.png "Login")`,
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   `![the login screen](` + pngURL + ` "Login")`,
		},
		{
			name:           "a single quoted title survives the rewrite",
			markdown:       `![the login screen](./login.png 'Login')`,
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   `![the login screen](` + pngURL + ` 'Login')`,
		},
		{
			name:           "a parenthesised title survives the rewrite",
			markdown:       `![the login screen](./login.png (Login))`,
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   `![the login screen](` + pngURL + ` (Login))`,
		},
		{
			// The escaped quotes are inside the title, so neither closes it.
			name:           "a title containing escaped quotes survives the rewrite",
			markdown:       `![the login screen](./login.png "he said \"hi\"")`,
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   `![the login screen](` + pngURL + ` "he said \"hi\"")`,
		},
		{
			// The backslash escapes the ")", so it belongs to the path
			// rather than closing the destination.
			name:           `an inline path with an escaped ")" is rewritten`,
			markdown:       `![a](./f\).png)`,
			attachmentArgs: []attachmentArg{{Path: `./f).png`, URL: pngURL}},
			wantMarkdown:   `![a](` + pngURL + `)`,
		},
		{
			// The scan walks every "](" in the block, so a malformed one
			// ahead of the real reference must be rejected rather than
			// swallowing it.
			name:           "an unterminated title earlier in the block is skipped",
			markdown:       `[x](./a.png "oops) then ![the login screen](./login.png)`,
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   `[x](./a.png "oops) then ![the login screen](` + pngURL + `)`,
		},
		{
			name:           "a tail with no closing parenthesis earlier in the block is skipped",
			markdown:       `[x](./a.png "t" and ![the login screen](./login.png)`,
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   `[x](./a.png "t" and ![the login screen](` + pngURL + `)`,
		},
		{
			// The first "](" holds the same path but no closing parenthesis
			// after its trailing word, so it is not a link tail at all and
			// must not be claimed in place of the real reference.
			name:           "a tail whose destination matches but does not close is skipped",
			markdown:       `[x](./login.png b) and [the login screen](./login.png)`,
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   `[x](./login.png b) and [the login screen](` + pngURL + `)`,
		},

		// Structure that could confuse the scan.
		{
			name:           "an image nested in a link is told apart from the link",
			markdown:       "[![the badge](./login.png)](./repro.mp4)",
			attachmentArgs: []attachmentArg{pngArg(), mp4Arg()},
			wantMarkdown:   "[![the badge](" + pngURL + ")](" + mp4URL + ")",
		},
		{
			name:           "a thumbnail linking to its own file rewrites both halves",
			markdown:       "[![the login screen](./login.png)](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "[![the login screen](" + pngURL + ")](" + pngURL + ")",
		},
		{
			name:           "a link inside a video alt text loses to the replacement of the whole embed",
			markdown:       "![see [the screenshot](./login.png)](./repro.mp4)",
			attachmentArgs: []attachmentArg{pngArg(), mp4Arg()},
			wantMarkdown:   mp4URL,
			// The video swallowed the reference to the image, so the image is
			// reported for appending rather than quietly dropped.
			wantToAppend: []attachmentArg{pngArg()},
		},
		{
			// The degrade keeps the label, so a reference nested in it has to
			// be rewritten too. Rebuilding the node from the label's source
			// bytes used to leave this local path in the markdown and append the
			// same file again at the end.
			name:           "a link inside the alt text of a degraded video is rewritten in place",
			markdown:       "Here is ![see [the screenshot](./login.png)](./repro.mp4) inline.",
			attachmentArgs: []attachmentArg{pngArg(), mp4Arg()},
			wantMarkdown:   "Here is [see [the screenshot](" + pngURL + ")](" + mp4URL + ") inline.",
		},
		{
			name:           "an image inside the alt text of a degraded video is rewritten in place",
			markdown:       "Here is ![see ![the screenshot](./login.png)](./repro.mp4) inline.",
			attachmentArgs: []attachmentArg{pngArg(), mp4Arg()},
			wantMarkdown:   "Here is [see ![the screenshot](" + pngURL + ")](" + mp4URL + ") inline.",
		},
		{
			name:           "a video embedded inside a link to itself keeps the two apart",
			markdown:       "[![the recording](./repro.mp4)](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "[[the recording](" + mp4URL + ")](" + mp4URL + ")",
		},
		{
			name:           "a bracket inside a code byteRange in the alt text does not confuse the scan",
			markdown:       "![before `[` after](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![before `[` after](" + pngURL + ")",
		},
		{
			name:           "a reference in a list item is rewritten",
			markdown:       "- before\n- ![the login screen](./login.png)\n- after",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "- before\n- ![the login screen](" + pngURL + ")\n- after",
		},
		{
			name:           "a video alone in a tight list item stays a labelled link",
			markdown:       "- ![the recording](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "- [the recording](" + mp4URL + ")",
		},
		{
			// A tight list item holds no paragraph, only a text block, which
			// is why it does not play while the loose one above does.
			name:           "a video alone in a loose list item becomes a bare url",
			markdown:       "- one\n\n- ![the recording](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "- one\n\n- " + mp4URL,
		},
		{
			name:           "a reference in a blockquote is rewritten",
			markdown:       "> quoting\n> ![the login screen](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "> quoting\n> ![the login screen](" + pngURL + ")",
		},
		{
			name:           "a video alone in a blockquote becomes a bare url",
			markdown:       "> ![the recording](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "> " + mp4URL,
		},
		{
			name:           "a video alone in a nested blockquote becomes a bare url",
			markdown:       "> > ![the recording](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "> > " + mp4URL,
		},
		{
			name:           "a video alone in a blockquote inside a list item becomes a bare url",
			markdown:       "- > ![the recording](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "- > " + mp4URL,
		},
		{
			name:           "a reference in a heading is rewritten",
			markdown:       "# ![the login screen](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "# ![the login screen](" + pngURL + ")",
		},
		{
			name:           "a video alone in a heading stays a labelled link",
			markdown:       "# ![the recording](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "# [the recording](" + mp4URL + ")",
		},
		{
			// A bare URL here renders as a plain link showing the raw asset
			// UUID, so the labelled link is the better of the two.
			name:           "a video alone on a soft wrapped line stays a labelled link",
			markdown:       "Watch this:\n![the recording](./repro.mp4)\nand then read on.",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "Watch this:\n[the recording](" + mp4URL + ")\nand then read on.",
		},
		{
			name:           "a video sharing a paragraph inside a blockquote stays a labelled link",
			markdown:       "> Watch this:\n> ![the recording](./repro.mp4)",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "> Watch this:\n> [the recording](" + mp4URL + ")",
		},

		// Several files at once.
		{
			name:           "referenced and unreferenced files are sorted out",
			attachmentArgs: []attachmentArg{pngArg(), mp4Arg()},
			markdown:       "The login screen:\n\n![the login screen](./login.png)\n\nNothing references the video.",
			wantMarkdown: "The login screen:\n\n![the login screen](" + pngURL +
				")\n\nNothing references the video.",
			wantToAppend: []attachmentArg{mp4Arg()},
		},
		{
			name:           "unreferenced files keep the order they were passed",
			markdown:       "Nothing here.",
			attachmentArgs: []attachmentArg{mp4Arg(), pngArg()},
			wantMarkdown:   "Nothing here.",
			wantToAppend:   []attachmentArg{mp4Arg(), pngArg()},
		},
		{
			name:           "an image and a video in one paragraph are rewritten separately",
			markdown:       "See ![the login screen](./login.png) and ![the recording](./repro.mp4).",
			attachmentArgs: []attachmentArg{pngArg(), mp4Arg()},
			wantMarkdown:   "See ![the login screen](" + pngURL + ") and [the recording](" + mp4URL + ").",
		},

		// Nothing to do.
		{
			name:           "empty markdown reports every asset for appending",
			markdown:       "",
			attachmentArgs: []attachmentArg{pngArg(), mp4Arg()},
			wantMarkdown:   "",
			wantToAppend:   []attachmentArg{pngArg(), mp4Arg()},
		},
		{
			name:           "markdown with no assets is returned unchanged",
			markdown:       "Just prose.\n\n![hosted](https://example.com/a.png)",
			attachmentArgs: nil,
			wantMarkdown:   "Just prose.\n\n![hosted](https://example.com/a.png)",
		},
		{
			name:           "a file that produced no asset url is left as the author wrote it",
			markdown:       "![the login screen](./login.png)",
			attachmentArgs: []attachmentArg{{Path: "./login.png", Alt: "login"}}, // no url
			wantMarkdown:   "![the login screen](./login.png)",
		},

		// reference-style links, where the destination lives in a definition
		// rather than at the usage. Rewriting the definition once carries
		// every usage of that label with it.
		{
			name:           "an image written as a reference-style image is rewritten in its definition",
			markdown:       "![the login screen][shot]\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![the login screen][shot]\n\n[shot]: " + pngURL,
		},
		{
			name:           "an image written as a reference-style link is rewritten in its definition",
			markdown:       "See the [screenshot][shot] for details.\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "See the [screenshot][shot] for details.\n\n[shot]: " + pngURL,
		},
		{
			// A link to a video asset is promoted to a player when it stands
			// alone in a paragraph, so this comes out better than a link.
			name:           "a video written as a reference-style link is rewritten in its definition",
			markdown:       "[the recording][clip]\n\n[clip]: ./repro.mp4",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantMarkdown:   "[the recording][clip]\n\n[clip]: " + mp4URL,
		},
		{
			name:           "a video written as a reference-style image is refused",
			markdown:       "![the recording][clip]\n\n[clip]: ./repro.mp4",
			attachmentArgs: []attachmentArg{mp4Arg()},
			wantErr:        "cannot embed a video as a reference-style image: ./repro.mp4",
		},
		{
			name:           "the collapsed reference form is rewritten in its definition",
			markdown:       "![shot][]\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![shot][]\n\n[shot]: " + pngURL,
		},
		{
			name:           "the shortcut reference form is rewritten in its definition",
			markdown:       "![shot]\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![shot]\n\n[shot]: " + pngURL,
		},
		{
			name:           "one definition used by both a link and an image is rewritten once",
			markdown:       "Both [a link][shot] and ![an image][shot].\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "Both [a link][shot] and ![an image][shot].\n\n[shot]: " + pngURL,
		},
		{
			name:           "a definition keeps its title",
			markdown:       "![the login screen][shot]\n\n[shot]: ./login.png \"Login\"",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![the login screen][shot]\n\n[shot]: " + pngURL + " \"Login\"",
		},
		{
			name:           "an angle bracketed definition path is replaced whole",
			markdown:       "![shot][s]\n\n[s]: <./Screenshot 2026-08-10 at 5.38.10 PM.png>",
			attachmentArgs: []attachmentArg{{Path: "./Screenshot 2026-08-10 at 5.38.10 PM.png", URL: pngURL}},
			wantMarkdown:   "![shot][s]\n\n[s]: " + pngURL,
		},
		{
			// The escaped ">" is part of the filename, not the closing
			// bracket, so the scan has to step over it.
			name:           `a definition path with an escaped ">" is replaced whole`,
			markdown:       `![shot][s]` + "\n\n" + `[s]: <./a\>b.png>`,
			attachmentArgs: []attachmentArg{{Path: "./a>b.png", URL: pngURL}},
			wantMarkdown:   "![shot][s]\n\n[s]: " + pngURL,
		},
		{
			name:           "a definition whose angle bracket is never closed is left alone",
			markdown:       "![shot][s]\n\n[s]: <./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![shot][s]\n\n[s]: <./login.png",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			name:           "a tab between a definition path and its title ends the path",
			markdown:       "![shot][s]\n\n[s]: ./login.png\t\"Login\"",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![shot][s]\n\n[s]: " + pngURL + "\t\"Login\"",
		},
		{
			name:           "a definition in markdown with carriage returns is rewritten",
			markdown:       "![shot][s]\r\n\r\n[s]: ./login.png\r\n",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![shot][s]\r\n\r\n[s]: " + pngURL + "\r\n",
		},
		{
			// The block byteRanges a carriage return, so the scan walks one while
			// looking for the reference.
			name:           "references in a wrapped paragraph with carriage returns are rewritten",
			markdown:       "a ![x](./login.png) b\r\nc ![y](./login.png) d",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "a ![x](" + pngURL + ") b\r\nc ![y](" + pngURL + ") d",
		},
		{
			// A destination cannot byteRange a line break, so this is not a link
			// tail and must not be claimed in place of the real reference.
			name:           "a candidate whose destination byteRanges a carriage return is skipped",
			markdown:       "See ](./log\r\nin.png) and ![the login screen](./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "See ](./log\r\nin.png) and ![the login screen](" + pngURL + ")",
		},
		{
			// The parentheses are balanced, so they belong to the path rather
			// than closing the inline destination.
			name:           "an inline path containing balanced parentheses is rewritten",
			markdown:       "![a](./f(1).png)",
			attachmentArgs: []attachmentArg{{Path: "./f(1).png", URL: pngURL}},
			wantMarkdown:   "![a](" + pngURL + ")",
		},
		{
			name:           "an inline path containing nested balanced parentheses is rewritten",
			markdown:       "![a](./f((1)(2)).png)",
			attachmentArgs: []attachmentArg{{Path: "./f((1)(2)).png", URL: pngURL}},
			wantMarkdown:   "![a](" + pngURL + ")",
		},
		{
			// An unbalanced ")" closes the destination, so the path is only
			// "./f" and the rest falls out as text. GitHub renders it the
			// same way.
			name:           "an unbalanced closing parenthesis ends the destination",
			markdown:       "![a](./f).png)",
			attachmentArgs: []attachmentArg{{Path: "./f", URL: pngURL}},
			wantMarkdown:   "![a](" + pngURL + ").png)",
		},
		{
			// An unbalanced "(" makes the whole thing literal text rather
			// than a link, so there is nothing to rewrite.
			name:           "an unbalanced opening parenthesis is not a reference",
			markdown:       "![a](./f(1.png)",
			attachmentArgs: []attachmentArg{{Path: "./f(1.png", URL: pngURL}},
			wantMarkdown:   "![a](./f(1.png)",
			wantToAppend:   []attachmentArg{{Path: "./f(1.png", URL: pngURL}},
		},
		{
			// A space ends an unbracketed destination, so this is text, not
			// an image. Angle brackets or percent encoding are the spellings
			// that work, both covered above.
			name:           "an unbracketed path with a space is not a reference",
			markdown:       "![a](./my file.png)",
			attachmentArgs: []attachmentArg{{Path: "./my file.png", URL: pngURL}},
			wantMarkdown:   "![a](./my file.png)",
			wantToAppend:   []attachmentArg{{Path: "./my file.png", URL: pngURL}},
		},
		{
			name:           "an inline angle bracket that is never closed is left alone",
			markdown:       "![a](<./login.png)",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![a](<./login.png)",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			// Nothing uses the label, so the definition renders nothing.
			// Editing it would leave the file invisible instead of appended.
			name:           "an unused definition is left alone and its file is still appended",
			markdown:       "No references here.\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "No references here.\n\n[shot]: ./login.png",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			name:           "a definition inside a code fence is left alone",
			markdown:       "```\n[shot]: ./login.png\n```\n\n![the login screen][shot]",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "```\n[shot]: ./login.png\n```\n\n![the login screen][shot]",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			name:           "a definition inside an inline code byteRange is left alone",
			markdown:       "Write `[shot]: ./login.png` to define it.\n\n![the login screen][shot]",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "Write `[shot]: ./login.png` to define it.\n\n![the login screen][shot]",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			name:           "an inline usage and a reference usage of one file are both rewritten",
			markdown:       "![inline](./login.png) and [attachmentArg][shot].\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![inline](" + pngURL + ") and [attachmentArg][shot].\n\n[shot]: " + pngURL,
		},
		{
			// A node records no label, so a spare definition carrying the same
			// destination is rewritten too. It renders nothing either way.
			name:           "a second definition of the same file is rewritten alongside the used one",
			markdown:       "![the login screen][a]\n\n[a]: ./login.png\n[b]: ./login.png",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "![the login screen][a]\n\n[a]: " + pngURL + "\n[b]: " + pngURL,
		},
		{
			name:           "a definition for a file nobody attached is left alone",
			markdown:       "![other][o]\n\n[o]: ./other.png",
			attachmentArgs: []attachmentArg{{Path: "./other.png", Alt: "other"}},
			wantMarkdown:   "![other][o]\n\n[o]: ./other.png",
		},
		{
			// The block range of a definition continued onto the next line of
			// a blockquote still carries the ">" prefix, so the destination
			// read from the source is that marker rather than the path.
			// Rewriting it would eat the blockquote, so the definition is
			// left alone and the file is appended instead.
			name:           "a definition continued onto the next line of a blockquote is left alone",
			markdown:       "> [shot]:\n> ./login.png\n\n![the login screen][shot]",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "> [shot]:\n> ./login.png\n\n![the login screen][shot]",
			wantToAppend:   []attachmentArg{pngArg()},
		},
		{
			name:           "a definition inside a blockquote on one line is rewritten",
			markdown:       "> [shot]: ./login.png\n>\n> ![the login screen][shot]",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "> [shot]: " + pngURL + "\n>\n> ![the login screen][shot]",
		},
		{
			name:           "a definition with its title on the next line keeps the title",
			markdown:       "[shot]: ./login.png\n  \"Login\"\n\n![the login screen][shot]",
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   "[shot]: " + pngURL + "\n  \"Login\"\n\n![the login screen][shot]",
		},
		{
			// The escaped "]:" inside the label is not where the destination
			// starts, so the scan has to skip it to find the real one.
			name:           "a definition whose label contains an escaped bracket is rewritten",
			markdown:       `[a\]: b]: ./login.png` + "\n\n" + `![the login screen][a\]: b]`,
			attachmentArgs: []attachmentArg{pngArg()},
			wantMarkdown:   `[a\]: b]: ` + pngURL + "\n\n" + `![the login screen][a\]: b]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The real flow: validate, then upload, then rewrite. Markdown that
			// cannot work is refused here, before anything uploads.
			v, err := newAttachableMarkdown(tt.markdown, tt.attachmentArgs)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			got, err := attachAssetsToMarkdown(v)
			require.NoError(t, err)
			require.Equal(t, tt.wantMarkdown, got.Rewritten)
			require.Equal(t, tt.wantToAppend, got.ToAppend)
		})
	}
}

func TestNewAttachableMarkdown(t *testing.T) {
	// Nothing has uploaded when this runs, so no argument carries a URL.
	png := attachmentArg{Path: "./login.png", Alt: "login"}
	mp4 := attachmentArg{Path: "./repro.mp4", Alt: "repro.mp4", RendersAsPlayer: true}
	clip := attachmentArg{Path: "./clip.mov", Alt: "clip.mov", RendersAsPlayer: true}

	tests := []struct {
		name           string
		markdown       string
		attachmentArgs []attachmentArg
		wantErr        string
	}{
		{
			name:           "an image written as a reference-style image is fine",
			markdown:       "![the login screen][shot]\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{png},
		},
		{
			name:           "an image written as a reference-style link is fine",
			markdown:       "See the [screenshot][shot].\n\n[shot]: ./login.png",
			attachmentArgs: []attachmentArg{png},
		},
		{
			name:           "a video written as a reference-style link is fine",
			markdown:       "[the recording][clip]\n\n[clip]: ./repro.mp4",
			attachmentArgs: []attachmentArg{mp4},
		},
		{
			name:           "a video written as a reference-style image is refused",
			markdown:       "![the recording][clip]\n\n[clip]: ./repro.mp4",
			attachmentArgs: []attachmentArg{mp4},
			wantErr:        "cannot embed a video as a reference-style image: ./repro.mp4",
		},
		{
			name:           "every offending video is named",
			markdown:       "![one][a] and ![two][b]\n\n[a]: ./repro.mp4\n[b]: ./clip.mov",
			attachmentArgs: []attachmentArg{mp4, clip},
			wantErr:        "cannot embed a video as a reference-style image: ./repro.mp4, ./clip.mov",
		},
		{
			name:           "a video named once is reported once",
			markdown:       "![one][a] and ![two][a]\n\n[a]: ./repro.mp4",
			attachmentArgs: []attachmentArg{mp4},
			wantErr:        "cannot embed a video as a reference-style image: ./repro.mp4",
		},
		{
			// Both orderings, because the reported videos are deduplicated by
			// argument. A link is the allowed shape, so marking the argument
			// seen while skipping it would swallow the embed that follows.
			name:           "a video used as both a link and an image is reported (image first)",
			markdown:       "![one][a] and [two][a]\n\n[a]: ./repro.mp4",
			attachmentArgs: []attachmentArg{mp4},
			wantErr:        "cannot embed a video as a reference-style image: ./repro.mp4",
		},
		{
			name:           "a video used as both a link and an image is reported (link first)",
			markdown:       "[one][a] and ![two][a]\n\n[a]: ./repro.mp4",
			attachmentArgs: []attachmentArg{mp4},
			wantErr:        "cannot embed a video as a reference-style image: ./repro.mp4",
		},
		{
			// The degrade rule handles this shape, so it must not be caught
			// here.
			name:           "an inline video embed is not a reference-style image",
			markdown:       "The failure ![screen recording](./repro.mp4) is reproducible.",
			attachmentArgs: []attachmentArg{mp4},
		},
		{
			name:           "an unused definition for a video is fine",
			markdown:       "Nothing uses it.\n\n[clip]: ./repro.mp4",
			attachmentArgs: []attachmentArg{mp4},
		},
		{
			name:           "a video reference image inside a code fence is fine",
			markdown:       "```\n![clip][c]\n\n[c]: ./repro.mp4\n```",
			attachmentArgs: []attachmentArg{mp4},
		},
		{
			name:           "markdown with no attachments is fine",
			markdown:       "Just prose.",
			attachmentArgs: []attachmentArg{png, mp4},
		},
		{
			name:           "no assets at all is fine",
			markdown:       "![the recording][clip]\n\n[clip]: ./repro.mp4",
			attachmentArgs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := newAttachableMarkdown(tt.markdown, tt.attachmentArgs)
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, tt.markdown, v.markdown)
				return
			}
			require.EqualError(t, err, tt.wantErr)
			require.Zero(t, v, "refused markdown must not be usable")
		})
	}
}

func TestIsSingleImage(t *testing.T) {
	const url = "https://example.invalid/probe"

	tests := []struct {
		name     string
		markdown string
		want     bool
	}{
		{
			name:     "one image pointing where it should",
			markdown: "![a caption](" + url + ")",
			want:     true,
		},
		{
			name:     "empty alt text",
			markdown: "![](" + url + ")",
			want:     true,
		},
		{
			name:     "brackets that stay inside because they are escaped",
			markdown: `![a \[bracketed\] caption](` + url + `)`,
			want:     true,
		},
		{
			name:     "alt text that closes the image early takes the destination",
			markdown: "![](https://evil.example.com/x.png)](" + url + ")",
			want:     false,
		},
		{
			name:     "alt text that adds a second image",
			markdown: "![a](" + url + ")![b](" + url + ")",
			want:     false,
		},
		{
			name:     "alt text that leaves trailing prose outside the image",
			markdown: "![a](" + url + ") and more",
			want:     false,
		},
		{
			name:     "alt text that opens a second paragraph",
			markdown: "![a](" + url + ")\n\nsecond",
			want:     false,
		},
		{
			name:     "an image pointing somewhere else",
			markdown: "![a](https://evil.example.com/x.png)",
			want:     false,
		},
		{
			name:     "no image at all",
			markdown: "just prose",
			want:     false,
		},
		{
			name:     "nothing",
			markdown: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isSingleImage(tt.markdown, url))
		})
	}
}
