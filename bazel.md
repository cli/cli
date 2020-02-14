# Building with Bazel

Bazel enables a nice developer experience as well as correct, reproducable release builds.

# Status

Currently Bazel functionality in this project is limited to local dev & test. Release build support will be added later if there's any interest in merging Bazel capabilites into the project.

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
