## Examples in use

### Creating issues and pull requests

#### Interactively

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Create a pull request interactively</span>
<span class='magenta'>~/Projects/my-project</span>$ gh pr create
Creating pull request for <span class="cyan">different-stuff</span> into <span class="cyan">master</span> in ampinsk/test
<b>? Title</b> My new pull request
<b>? Body</b> Lorem ipsum sit dolor.
http://github.com/owner/repo/pull/1
<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

#### With flags

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Create an issue using flags</span>
<span class='magenta'>~/Projects/my-project</span>$ gh issue create -t "Pull request title" -b "Pull request body"
http://github.com/owner/repo/issue/1
<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

#### In the browser

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Quickly navigate to the pull request creation page</span>
<span class='magenta'>~/Projects/my-project</span>$ gh pr create -w
Opening https://github.com/owner/repo/pull/branch in your browser.
<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

### Viewing a list of issues or pull requests in a repository

#### Default behavior

You will see the most recent 20 open items

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Viewing a list of open issues</span>
<span class='magenta'>~/Projects/my-project</span>$ gh issue list
Issues for owner/repo

<span class='green'>#16</span>  testing cli
<span class='green'>#14</span>  bleep
<span class='green'>#13</span>  Testing!      <span class='gray'>(enhancement)</span>
<span class='green'>#8</span>   bleep bloop   <span class='gray'>(bug)</span>

<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

#### Filtering with flags
You can use flags to filter the list for your specific use cases.

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Viewing a list of closed pull requests assigned to a user</span>
<span class='magenta'>~/Projects/my-project</span>$ gh pr list --state closed --assignee user
Pull requests for owner/repo

<span class='red'>#13</span>  Testing!      <span class='cyan'>update-items</span>
<span class='red'>#8</span>   bleep bloop   <span class='cyan'>add-feature</span>

<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

### Checking out a pull request locally

#### Using pull request number

You can check out any pull request, including from forks, in a repository using its pull request number

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Checking out a pull request locally</span>
<span class='magenta'>~/Projects/my-project</span>$ gh pr checkout 12
remote: Enumerating objects: 66, done.
remote: Counting objects: 100% (66/66), done.
remote: Total 83 (delta 66), reused 66 (delta 66), pack-reused 17
Unpacking objects: 100% (83/83), done.
From https://github.com/owner/repo
 * [new ref]             refs/pull/8896/head -> patch-2
M       README.md
Switched to branch 'patch-2'

<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

#### Using other selectors

You can also use URLs and branch names to checkout pull requests.

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Checking out a pull request locally</span>
<span class='magenta'>~/Projects/my-project</span>$ gh pr checkout branch-name
Switched to branch 'branch-name'
Your branch is up to date with 'origin/branch-name'.
Already up to date.

<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

### Viewing the status of your relevant work

#### Pull requests

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Viewing the status of your relevant pull requests</span>
<span class='magenta'>~/Projects/my-project</span>$ gh pr status
<b>Current branch</b>
  <span class='green'>#12</span> Remove the test feature <span class='cyan'>[Rexogamer:patch-2]</span>
   - <span class='red'>All checks failing</span> - <span class='yellow'>review required</span>

<b>Created by you</b>
  <span class='gray'>You have no open pull requests</span>

<b>Requesting a code review from you</b>
  <span class='green'>#13</span> Fix tests <span class='cyan'>[branch]</span>
  - <span class='red'>3/4 checks failing</span> - <span class='yellow'>review required</span>
  <span class='green'>#15</span> New feature <span class='cyan'>[branch]</span>
   - <span class='green'>Checks passing</span> - <span class='green'>approved</span>

<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

#### Issues

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Viewing issues relevant to you</span>
<span class='magenta'>~/Projects/my-project</span>$ gh issue status
<b>Issues assigned to you</b>
  <span class='green'>#8509</span> [Fork] Improve how Desktop handles forks  <span class='gray'>(epic:fork, meta)</span>

<b>Issues mentioning you</b>
  <span class='green'>#8938</span> [Fork] Add create fork flow entry point at commit warning  <span class='gray'>(epic:fork)</span>
  <span class='green'>#8509</span> [Fork] Improve how Desktop handles forks  <span class='gray'>(epic:fork, meta)</span>

<b>Issues opened by you</b>
  <span class='green'>#8936</span> [Fork] Hide PR number badges on branches that have an upstream PR  <span class='gray'>(epic:fork)</span>
  <span class='green'>#6386</span> Improve no editor detected state on conflicts modal  <span class='gray'>(enhancement)</span>

<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>


### Viewing a pull request or issue

#### In the browser

Quickly open a pull request or issue in the browser.

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Viewing a pull request in the browser</span>
<span class='magenta'>~/Projects/my-project</span>$ gh pr view 21
Opening https://github.com/owner/repo/pull/21 in your browser.
<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>

#### In terminal

Use `--preview` or `-p` to view a preview of the title and body.

<div class="width-full">
<pre class="terminal"><code><span class='gray'>// Viewing an issue in terminal</span>
<span class='magenta'>~/Projects/my-project</span>$ gh issue view 21 --preview
<b>[Fork] Hide PR number badges on branches that have an upstream PR</b>
<span class='gray'>opened by ampinsk. 0 comments. (epic:fork)</span>

  While we're in a state where we're not displaying upstream pull requests in the PR list, we should
  hide the `#XX` badge to avoid confusion.

  cc @outofambit

<span class='gray'>View this issue on GitHub: https://github.com/owner/repo/issues/21</span>
<span class='magenta'>~/Projects/my-project</span>$</code></pre>
</div>
