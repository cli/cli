BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

GH_VERSION = $(shell go describe --tags 2>/dev/null || git rev-parse --short HEAD)
LDFLAGS := -X github.com/github/gh-cli/command.Version=$(GH_VERSION) $(LDFLAGS)
LDFLAGS := -X github.com/github/gh-cli/command.BuildDate=$(shell date +%Y-%m-%d) $(LDFLAGS)

bin/gh: $(BUILD_FILES)
	@go build -ldflags "$(LDFLAGS)" -o "$@"

test:
	go test ./...

.PHONY: test