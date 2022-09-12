package unwrap

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUnwrap(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "it wraps text at 72 chars, ignoring anything indented, handling lists nicely",
			input: `Alice was beginning to get very tired of sitting by her sister on the
bank, and of having nothing to do: once or twice she had peeped into the
book her sister was reading, but it had no pictures or conversations in
it, "and what is the use of a book," thought Alice "without pictures or
conversations?"

  two-indent multi-line
  here

Some more words

    four-indent multi-line
    here

Even more words

	indent using tab chars

- some thing
- some other thing
- also this

Some more words

1. thing the first
2. thing the second!
3. third

Even more words

1 abc
2 dfg
3 dfgaskhsakjdhaskjdhasjdh

Short lines not that don't follow a long line
don't get unwrapped
because that would look weird.
They must be short for some reason.

So she was considering in her own mind (as well as she could, for the
hot day made her feel very sleepy and stupid), whether the pleasure of
making a daisy-chain would be worth the trouble of getting up and
picking the daisies, when suddenly a White Rabbit with pink eyes ran
close by her.

	indent using tab chars
	that spans
	multiple lines

Some really long numbered paragraphs

1. Alice was beginning to get very tired of sitting by her sister on the
    bank, and of having nothing to do: once or twice she had peeped into
    the book her sister was reading, but it had no pictures or
    conversations in it, "and what is the use of a book," thought alice
    "without pictures or conversations?"
2. So she was considering in her own mind (as well as she could, for the
    hot day made her feel very sleepy and stupid), whether the pleasure
    of making a daisy-chain would be worth the trouble of getting up and
    picking the daisies, when suddenly a White Rabbit with pink eyes ran
    close by her.

Numbering with indents

  1. Alice was beginning to get very tired of sitting by her sister on the
      bank, and of having nothing to do: once or twice she had peeped into
      the book her sister was reading, but it had no pictures or
      conversations in it, "and what is the use of a book," thought alice
      "without pictures or conversations?"
  2. So she was considering in her own mind (as well as she could, for the
      hot day made her feel very sleepy and stupid), whether the pleasure
      of making a daisy-chain would be worth the trouble of getting up and
      picking the daisies, when suddenly a White Rabbit with pink eyes ran
      close by her.

Numbering with indents and line separators

  1. Alice was beginning to get very tired of sitting by her sister on the
      bank, and of having nothing to do: once or twice she had peeped into
      the book her sister was reading, but it had no pictures or
      conversations in it, "and what is the use of a book," thought alice
      "without pictures or conversations?"

  2. So she was considering in her own mind (as well as she could, for the
      hot day made her feel very sleepy and stupid), whether the pleasure
      of making a daisy-chain would be worth the trouble of getting up and
      picking the daisies, when suddenly a White Rabbit with pink eyes ran
      close by her.

unordered lists

  - Alice was beginning to get very tired of sitting by her sister on the
    bank, and of having nothing to do: once or twice she had peeped into
    the book her sister was reading, but it had no pictures or
    conversations in it, "and what is the use of a book," thought alice
    "without pictures or conversations?"
  - So she was considering in her own mind (as well as she could, for the
    hot day made her feel very sleepy and stupid), whether the pleasure
    of making a daisy-chain would be worth the trouble of getting up and
    picking the daisies, when suddenly a White Rabbit with pink eyes ran
    close by her.
`,
			want: `Alice was beginning to get very tired of sitting by her sister on the bank, and of having nothing to do: once or twice she had peeped into the book her sister was reading, but it had no pictures or conversations in it, "and what is the use of a book," thought Alice "without pictures or conversations?"

  two-indent multi-line
  here

Some more words

    four-indent multi-line
    here

Even more words

	indent using tab chars

- some thing
- some other thing
- also this

Some more words

1. thing the first
2. thing the second!
3. third

Even more words

1 abc
2 dfg
3 dfgaskhsakjdhaskjdhasjdh

Short lines not that don't follow a long line
don't get unwrapped
because that would look weird.
They must be short for some reason.

So she was considering in her own mind (as well as she could, for the hot day made her feel very sleepy and stupid), whether the pleasure of making a daisy-chain would be worth the trouble of getting up and picking the daisies, when suddenly a White Rabbit with pink eyes ran close by her.

	indent using tab chars
	that spans
	multiple lines

Some really long numbered paragraphs

1. Alice was beginning to get very tired of sitting by her sister on the bank, and of having nothing to do: once or twice she had peeped into the book her sister was reading, but it had no pictures or conversations in it, "and what is the use of a book," thought alice "without pictures or conversations?"
2. So she was considering in her own mind (as well as she could, for the hot day made her feel very sleepy and stupid), whether the pleasure of making a daisy-chain would be worth the trouble of getting up and picking the daisies, when suddenly a White Rabbit with pink eyes ran close by her.

Numbering with indents

  1. Alice was beginning to get very tired of sitting by her sister on the bank, and of having nothing to do: once or twice she had peeped into the book her sister was reading, but it had no pictures or conversations in it, "and what is the use of a book," thought alice "without pictures or conversations?"
  2. So she was considering in her own mind (as well as she could, for the hot day made her feel very sleepy and stupid), whether the pleasure of making a daisy-chain would be worth the trouble of getting up and picking the daisies, when suddenly a White Rabbit with pink eyes ran close by her.

Numbering with indents and line separators

  1. Alice was beginning to get very tired of sitting by her sister on the bank, and of having nothing to do: once or twice she had peeped into the book her sister was reading, but it had no pictures or conversations in it, "and what is the use of a book," thought alice "without pictures or conversations?"

  2. So she was considering in her own mind (as well as she could, for the hot day made her feel very sleepy and stupid), whether the pleasure of making a daisy-chain would be worth the trouble of getting up and picking the daisies, when suddenly a White Rabbit with pink eyes ran close by her.

unordered lists

  - Alice was beginning to get very tired of sitting by her sister on the bank, and of having nothing to do: once or twice she had peeped into the book her sister was reading, but it had no pictures or conversations in it, "and what is the use of a book," thought alice "without pictures or conversations?"
  - So she was considering in her own mind (as well as she could, for the hot day made her feel very sleepy and stupid), whether the pleasure of making a daisy-chain would be worth the trouble of getting up and picking the daisies, when suddenly a White Rabbit with pink eyes ran close by her.
`,
		},
		{
			name: "handles email-ish trailers appropriately",
			input: `"git p4" working on UTF-16 files on Windows did not implement
CRLF-to-LF conversion correctly, which has been corrected.
source: <pull.1294.v2.git.git.1658341065221.gitgitgadget@gmail.com>

Signed-off-by: Foo Bar <foobar@longemailongemailongemail.com>
Reviewed-by: Baz Qux <baz@qux123123123123123123123.com>
Short-trailer-by: yep
Co-authored-by: Someone <someone123123123123123123@gmail.com>
`,
			want: `"git p4" working on UTF-16 files on Windows did not implement CRLF-to-LF conversion correctly, which has been corrected.
source: <pull.1294.v2.git.git.1658341065221.gitgitgadget@gmail.com>

Signed-off-by: Foo Bar <foobar@longemailongemailongemail.com>
Reviewed-by: Baz Qux <baz@qux123123123123123123123.com>
Short-trailer-by: yep
Co-authored-by: Someone <someone123123123123123123@gmail.com>
`,
		},
		{
			name: "handles true trailers properly",
			input: `There was a bug in the codepath to upgrade generation information
in commit-graph from v1 to v2 format, which has been corrected.
source: <cover.1657667404.git.me@ttaylorr.com>

* tb/commit-graph-genv2-upgrade-fix:
  commit-graph: fix corrupt upgrade from generation v1 to v2
  commit-graph: introduce ''repo_find_commit_pos_in_graph()''
  t5318: demonstrate commit-graph generation v2 corruption
`,
			want: `There was a bug in the codepath to upgrade generation information in commit-graph from v1 to v2 format, which has been corrected.
source: <cover.1657667404.git.me@ttaylorr.com>

* tb/commit-graph-genv2-upgrade-fix:
  commit-graph: fix corrupt upgrade from generation v1 to v2
  commit-graph: introduce ''repo_find_commit_pos_in_graph()''
  t5318: demonstrate commit-graph generation v2 corruption
`,
		},
		{
			name: "it doesn't detect line separators as list chars",
			input: `The ''p2000'' tests demonstrate a ~92% execution time reduction for
''git rm'' using a sparse index.

----
Also, normalize a behavioral difference of ''git-rm'' under sparse-index.
See related discussion [1].
`,
			want: `The ''p2000'' tests demonstrate a ~92% execution time reduction for ''git rm'' using a sparse index.

----
Also, normalize a behavioral difference of ''git-rm'' under sparse-index. See related discussion [1].
`,
		},
		{
			name: "list item edge case",
			input: `The name of the variable is wrong, and it can be set to anything, like
1.
`,
			want: `The name of the variable is wrong, and it can be set to anything, like 1.
`,
		},
		{
			name: "multi-paragraph list items",
			input: `Why do we miss these leaks? Because:

  * We have leaks inside "test_expect_failure" blocks, which by design
    will not distinguish a "normal" failure from an abort() or
    segfault. See [1] for a discussion of it shortcomings.

  * Our tests will otherwise catch segfaults and abort(), but if we
    invoke a command that invokes another command it needs to ferry the
    exit code up to us.

    Notably a command that e.g. might invoke "git pack-objects" might
    itself exit with status 128 if that "pack-objects" segfaults or
    abort()'s. If the test invoking the parent command(s) is using
    "test_must_fail" we'll consider it an expected "ok" failure.

  * run-command.c doesn't (but probably should) ferry up such exit
    codes, so for e.g. "git push" tests where we expect a failure and an
    underlying "git" command fails we won't ferry up the segfault or
    abort exit code.

A few notes:

  - We use REFNAME_ALLOW_ONELEVEL here, which lets:

        git update-ref refs/heads/foo FETCH_HEAD

    continue to work. It's unclear whether anybody wants to do something
    so odd, but it does work now, so this is erring on the conservative
    side. There's a test to make sure we didn't accidentally break this,
    but don't take that test as an endorsement that it's a good idea, or
    something we might not change in the future.

  - The test in t4202-log.sh checks how we handle such an invalid ref on
    the reading side, so it has to be updated to touch the HEAD file
    directly.
`,
			want: `Why do we miss these leaks? Because:

  * We have leaks inside "test_expect_failure" blocks, which by design will not distinguish a "normal" failure from an abort() or segfault. See [1] for a discussion of it shortcomings.

  * Our tests will otherwise catch segfaults and abort(), but if we invoke a command that invokes another command it needs to ferry the exit code up to us.

    Notably a command that e.g. might invoke "git pack-objects" might itself exit with status 128 if that "pack-objects" segfaults or abort()'s. If the test invoking the parent command(s) is using "test_must_fail" we'll consider it an expected "ok" failure.

  * run-command.c doesn't (but probably should) ferry up such exit codes, so for e.g. "git push" tests where we expect a failure and an underlying "git" command fails we won't ferry up the segfault or abort exit code.

A few notes:

  - We use REFNAME_ALLOW_ONELEVEL here, which lets:

        git update-ref refs/heads/foo FETCH_HEAD

    continue to work. It's unclear whether anybody wants to do something so odd, but it does work now, so this is erring on the conservative side. There's a test to make sure we didn't accidentally break this, but don't take that test as an endorsement that it's a good idea, or something we might not change in the future.

  - The test in t4202-log.sh checks how we handle such an invalid ref on the reading side, so it has to be updated to touch the HEAD file directly.
`,
		},
		{
			name: "list item nesting (no spacing)",
			input: `Disadvantages:
- No visible organisation of the file contents. This means
  - Hard to tell which functions are utility functions and which are
    available to you in a debugging session
  - Lots of code duplication within lldb functions Lots of code
    duplication within lldb functions Lots of code duplication within
    lldb functions Lots of code duplication within lldb functions
    - Yard to tell which functions are utility functions and which are
      available to you in a debugging session
    - Xard to tell which functions are utility functions and which are
      available to you in a debugging session
  - Lots of code duplication within lldb functions Lots of code
    duplication within lldb functions Lots of code duplication within
    lldb functions Lots of code duplication within lldb functions
- Large files quickly become intimidating to work with
  - for example, ''lldb_disasm.py'' was implemented as a seperate
    Python module because it was easier to start with a clean slate
    than add significant amounts of code to ''lldb_cruby.py''
`,
			want: `Disadvantages:
- No visible organisation of the file contents. This means
  - Hard to tell which functions are utility functions and which are available to you in a debugging session
  - Lots of code duplication within lldb functions Lots of code duplication within lldb functions Lots of code duplication within lldb functions Lots of code duplication within lldb functions
    - Yard to tell which functions are utility functions and which are available to you in a debugging session
    - Xard to tell which functions are utility functions and which are available to you in a debugging session
  - Lots of code duplication within lldb functions Lots of code duplication within lldb functions Lots of code duplication within lldb functions Lots of code duplication within lldb functions
- Large files quickly become intimidating to work with
  - for example, ''lldb_disasm.py'' was implemented as a seperate Python module because it was easier to start with a clean slate than add significant amounts of code to ''lldb_cruby.py''
`,
		},
		{
			name: "list item nesting (with spacing)",
			input: `Disadvantages:
- No visible organisation of the file contents. This means

  - Hard to tell which functions are utility functions and which are
    available to you in a debugging session

  - Lots of code duplication within lldb functions Lots of code
    duplication within lldb functions Lots of code duplication within
    lldb functions Lots of code duplication within lldb functions

    - Yard to tell which functions are utility functions and which are
      available to you in a debugging session

    - Xard to tell which functions are utility functions and which are
      available to you in a debugging session

  - Lots of code duplication within lldb functions Lots of code
    duplication within lldb functions Lots of code duplication within
    lldb functions Lots of code duplication within lldb functions

- Large files quickly become intimidating to work with

  - for example, ''lldb_disasm.py'' was implemented as a seperate
    Python module because it was easier to start with a clean slate
    than add significant amounts of code to ''lldb_cruby.py''
`,
			want: `Disadvantages:
- No visible organisation of the file contents. This means

  - Hard to tell which functions are utility functions and which are available to you in a debugging session

  - Lots of code duplication within lldb functions Lots of code duplication within lldb functions Lots of code duplication within lldb functions Lots of code duplication within lldb functions

    - Yard to tell which functions are utility functions and which are available to you in a debugging session

    - Xard to tell which functions are utility functions and which are available to you in a debugging session

  - Lots of code duplication within lldb functions Lots of code duplication within lldb functions Lots of code duplication within lldb functions Lots of code duplication within lldb functions

- Large files quickly become intimidating to work with

  - for example, ''lldb_disasm.py'' was implemented as a seperate Python module because it was easier to start with a clean slate than add significant amounts of code to ''lldb_cruby.py''
`,
		},
		{
			name: "tricky code block that is not a list",
			input: `This sort of thing could be detected automatically with a rule similar
to the unused.cocci merged in 7fa60d2a5b6 (Merge branch
'ab/cocci-unused' into next, 2022-07-11). The following rule on top
would catch the case being fixed here:

        @@
        type T;
        identifier I;
        identifier REL1 =~ "^[a-z_]*_(release|reset|clear|free)$";
        identifier REL2 =~ "^(release|clear|free)_[a-z_]*$";
        @@

        - memset(\( I \| &I \), 0, ...);
          ... when != \( I \| &I \)
        (
          \( REL1 \| REL2 \)( \( I \| &I \), ...);
        |
          \( REL1 \| REL2 \)( \( &I \| I \) );
        )
          ... when != \( I \| &I \)

That rule should arguably use only &I, not I (as we might be passed a
pointer). The distinction would matter if anyone cared about the
side-effects of a memset() followed by release() of a pointer to a
variable passed into the function.
`,
			want: `This sort of thing could be detected automatically with a rule similar to the unused.cocci merged in 7fa60d2a5b6 (Merge branch 'ab/cocci-unused' into next, 2022-07-11). The following rule on top would catch the case being fixed here:

        @@
        type T;
        identifier I;
        identifier REL1 =~ "^[a-z_]*_(release|reset|clear|free)$";
        identifier REL2 =~ "^(release|clear|free)_[a-z_]*$";
        @@

        - memset(\( I \| &I \), 0, ...);
          ... when != \( I \| &I \)
        (
          \( REL1 \| REL2 \)( \( I \| &I \), ...);
        |
          \( REL1 \| REL2 \)( \( &I \| I \) );
        )
          ... when != \( I \| &I \)

That rule should arguably use only &I, not I (as we might be passed a pointer). The distinction would matter if anyone cared about the side-effects of a memset() followed by release() of a pointer to a variable passed into the function.
`,
		},
		{
			name:  "",
			input: ``,
			want:  ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// replace a double single quote with a backtick since we're unable to embed literal backticks in Go string literals
			got := Unwrap(strings.ReplaceAll(tt.input, "''", "`"))
			want := strings.ReplaceAll(tt.want, "''", "`")
			if want != got {
				t.Errorf("output mismatch:\n%s", cmp.Diff(want, got))
			}
		})
	}
}
