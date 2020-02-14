# Building with Bazel

Bazel enables a nice developer experience as well as fast, correct, reproducible release builds.

# Status

Currently Bazel functionality in this project is limited to local dev & test. Release build support will be added later if there's any interest in merging Bazel capabilities into the project.

# Setup

Ensure Bazel is configured on your system (`bazelisk` is recommended).

# Building

The first build will download all of the project dependencies and run the build, so it may take a minute or two. Subsequent builds will be cached and extremely fast.

```
bazel build //...
```

Now the built binary can be invoked with the following (macOS):

```
bazel-bin/cmd/gh/darwin_amd64_stripped/gh
```

# Testing

```
bazel test //...
bazel test //api:go_default_test
```

# Add or update BUILD files

Go is a language with first-class support from Bazel and there is an official tool available which enables automated generation of BUILD files. To add new BUILD files (when adding or source files), invoke the following:

```
bazel run //:gazelle
```

This will create or update BUILD files as needed to compile, run, and test the project.

# Update packages

During development, if dependencies have changed you'll want to run `gazelle` to update the WORKSPACE with the changes to `go.mod`. That can be done with the following:

```
bazel run //:gazelle -- update-repos -from_file=go.mod
```

# Lint BUILD files

In the event that you need to manually edit BUILD files or have made changes to the WORKSPACE, there are targets available to lint these files.

To view linter errors:

```
bazel run //:buildifier-lint-warn
```

To fix all lint issues silently:

```
bazel run //:buildifier-lint-fix
```