BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

GH_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
LDFLAGS := -X github.com/github/gh-cli/command.Version=$(GH_VERSION) $(LDFLAGS)
LDFLAGS := -X github.com/github/gh-cli/command.BuildDate=$(shell date +%Y-%m-%d) $(LDFLAGS)
ifdef GH_OAUTH_CLIENT_SECRET
	LDFLAGS := -X github.com/github/gh-cli/context.oauthClientID=$(GH_OAUTH_CLIENT_ID) $(LDFLAGS)
	LDFLAGS := -X github.com/github/gh-cli/context.oauthClientSecret=$(GH_OAUTH_CLIENT_SECRET) $(LDFLAGS)
endif

bin/gh: $(BUILD_FILES)
	@go build -ldflags "$(LDFLAGS)" -o "$@"

test:
	go test ./...
.PHONY: test

site:
	git worktree add site gh-pages

site-docs: site
	git -C site pull
	git -C site rm 'gh*.md' 2>/dev/null || true
	go run ./cmd/gen-docs site
	git -C site add 'gh*.md'
	git -C site commit -m 'update help docs'
	git -C site push
.PHONY: site-docs
