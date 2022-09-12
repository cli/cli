# it detects tables and figures and disables unwrapping for them
message = <<~MESSAGE
The ''p2000'' tests demonstrate a ~92% execution time reduction for
'git rm' using a sparse index.

Test                              HEAD~1            HEAD
--------------------------------------------------------------------------
2000.74: git rm ... (full-v3)     0.41(0.37+0.05)   0.43(0.36+0.07) +4.9%
2000.75: git rm ... (full-v4)     0.38(0.34+0.05)   0.39(0.35+0.05) +2.6%
2000.76: git rm ... (sparse-v3)   0.57(0.56+0.01)   0.05(0.05+0.00) -91.2%
2000.77: git rm ... (sparse-v4)   0.57(0.55+0.02)   0.03(0.03+0.00) -94.7%

Also, normalize a behavioral difference of ''git-rm'' under sparse-index.
See related discussion [1].

| Input     | Output (env A) | Output (env B)   | same/different |
|-----------+----------------+------------------+----------------|
| \{<foo>\} | {&lt;foo&gt;}  | \{&lt;foo&gt;}^M | different      |
| {<foo>}   | {&lt;foo&gt;}  | {&lt;foo&gt;}    | same           |
| \{<foo>}  | {&lt;foo&gt;}  | \{&lt;foo&gt;}^M | different      |
| \{foo\}   | {foo}          | {foo}            | same           |
| \{\}      | {}             | \{}^M            | different      |
| \{}       | {}             | {}               | same           |
| {\}       | {}             | {}               | same           |

Also, normalize a behavioral difference of ''git-rm'' under sparse-index.
See related discussion [1].
MESSAGE

assert_unwrap <<~EXPECTED, message
The ''p2000'' tests demonstrate a ~92% execution time reduction for 'git rm' using a sparse index.

Test                              HEAD~1            HEAD
--------------------------------------------------------------------------
2000.74: git rm ... (full-v3)     0.41(0.37+0.05)   0.43(0.36+0.07) +4.9%
2000.75: git rm ... (full-v4)     0.38(0.34+0.05)   0.39(0.35+0.05) +2.6%
2000.76: git rm ... (sparse-v3)   0.57(0.56+0.01)   0.05(0.05+0.00) -91.2%
2000.77: git rm ... (sparse-v4)   0.57(0.55+0.02)   0.03(0.03+0.00) -94.7%

Also, normalize a behavioral difference of ''git-rm'' under sparse-index. See related discussion [1].

| Input     | Output (env A) | Output (env B)   | same/different |
|-----------+----------------+------------------+----------------|
| \{<foo>\} | {&lt;foo&gt;}  | \{&lt;foo&gt;}^M | different      |
| {<foo>}   | {&lt;foo&gt;}  | {&lt;foo&gt;}    | same           |
| \{<foo>}  | {&lt;foo&gt;}  | \{&lt;foo&gt;}^M | different      |
| \{foo\}   | {foo}          | {foo}            | same           |
| \{\}      | {}             | \{}^M            | different      |
| \{}       | {}             | {}               | same           |
| {\}       | {}             | {}               | same           |

Also, normalize a behavioral difference of ''git-rm'' under sparse-index. See related discussion [1].
EXPECTED

# disables unwrapping inside fences
message = <<~MESSAGE
While creating an object, we will be able to fetch the uploader just
fine:

''''''
> c = CodeqlVariantAnalysisRepoTask.create(...., uploader: @owner)
> c.uploader
<User ...>
''''''

When ''safe-ruby'' is executed outside of the RAILS_ROOT directory it fails to
find the ''config/ruby-version'' script, e.g.

''''''
$ bin/vendor-gem https://github.com/github/serviceowners
Cloning https://github.com/github/serviceowners for gem build
Building serviceowners
HEAD is now at 3c5a3fe Merge pull request #67 from github/match-no-reviews
commit 3c5a3fe4aa352bc5a3557c7ff94aca0415a08f3f (HEAD -> main, tag: v0.11.0)
Merge: f562294 f90bcc2
Author: Matt Clark <44023+mclark@users.noreply.github.com>
Date:   Tue Aug 16 08:41:16 2022 -0400

    Merge pull request #67 from github/match-no-reviews

    Output just path with no owners for pattern specs with ''no_review'' specified
/workspaces/github/tmp/gems/serviceowners /workspaces/github/tmp/gems/serviceowners
Building serviceowners.gemspec
/workspaces/github/bin/safe-ruby-quick: line 14: config/ruby-version: No such file or directory
''''''

This commit resolves the problem by using an absolute path when invoking the
''config/ruby-version'' script.

''''''
PATCH /repos/{owner}/{repo}/code-scanning/codeql/variant-analyses/{variant_analysis_id}
''''''
MESSAGE

assert_unwrap <<~EXPECTED, message
While creating an object, we will be able to fetch the uploader just fine:

''''''
> c = CodeqlVariantAnalysisRepoTask.create(...., uploader: @owner)
> c.uploader
<User ...>
''''''

When ''safe-ruby'' is executed outside of the RAILS_ROOT directory it fails to find the ''config/ruby-version'' script, e.g.

''''''
$ bin/vendor-gem https://github.com/github/serviceowners
Cloning https://github.com/github/serviceowners for gem build
Building serviceowners
HEAD is now at 3c5a3fe Merge pull request #67 from github/match-no-reviews
commit 3c5a3fe4aa352bc5a3557c7ff94aca0415a08f3f (HEAD -> main, tag: v0.11.0)
Merge: f562294 f90bcc2
Author: Matt Clark <44023+mclark@users.noreply.github.com>
Date:   Tue Aug 16 08:41:16 2022 -0400

    Merge pull request #67 from github/match-no-reviews

    Output just path with no owners for pattern specs with ''no_review'' specified
/workspaces/github/tmp/gems/serviceowners /workspaces/github/tmp/gems/serviceowners
Building serviceowners.gemspec
/workspaces/github/bin/safe-ruby-quick: line 14: config/ruby-version: No such file or directory
''''''

This commit resolves the problem by using an absolute path when invoking the ''config/ruby-version'' script.

''''''
PATCH /repos/{owner}/{repo}/code-scanning/codeql/variant-analyses/{variant_analysis_id}
''''''
EXPECTED

# quote chars are ignored
message = <<~MESSAGE
test: add stuff
As @wincent said [here](http://example.com/):
> Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do
> eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut
> enim ad minim veniam, quis nostrud exercitation ullamco laboris
> nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in
>
> reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla
> pariatur. Excepteur sint occaecat cupidatat non proident, sunt in
> culpa qui officia deserunt mollit anim id est laborum.
Anyway. That is all.
MESSAGE

assert_unwrap <<~EXPECTED, message
test: add stuff
As @wincent said [here](http://example.com/):
> Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do
> eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut
> enim ad minim veniam, quis nostrud exercitation ullamco laboris
> nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in
>
> reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla
> pariatur. Excepteur sint occaecat cupidatat non proident, sunt in
> culpa qui officia deserunt mollit anim id est laborum.
Anyway. That is all.
EXPECTED