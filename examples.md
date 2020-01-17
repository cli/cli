## Examples in use

### Creating interactively

### Creating with flags

### Creating in the browser

### Viewing a filtered list in a repository

### Checking out a pull request locally

### Viewing the status of your relevant work

### Opening a pull request or issue in the browser

<div class="width-full">
  <pre class="terminal"><code><span class='gray'> // Create a pull request interactively</span>
  <span class='red'>~/Projects/my-project</span>$ gh pr create
  Creating pull request for <span class="cyan">different-stuff</span> into <span class="cyan">master</span> in ampinsk/test
  <b>? Title</b> My new pull request
  <b>? Body</b> Lorem ipsum sit dolor.
  http://github.com/owner/repo/pull/1
  <span class='red'>~/Projects/my-project</span>$</code></pre>
</div>

<div class="width-full">
  <pre class="terminal"><code><span class='gray'> // Create a pull request using flags</span>
  <span class='red'>~/Projects/my-project</span>$ gh pr create -t "Pull request title" -b "Pull request body"
  http://github.com/owner/repo/pull/1
  <span class='red'>~/Projects/my-project</span>$</code></pre>
</div>
