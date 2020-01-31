BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

GH_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
LDFLAGS := -X github.com/cli/cli/command.Version=$(GH_VERSION) $(LDFLAGS)
LDFLAGS := -X github.com/cli/cli/command.BuildDate=$(shell date +%Y-%m-%d) $(LDFLAGS)
ifdef GH_OAUTH_CLIENT_SECRET
	LDFLAGS := -X github.com/cli/cli/context.oauthClientID=$(GH_OAUTH_CLIENT_ID) $(LDFLAGS)
	LDFLAGS := -X github.com/cli/cli/context.oauthClientSecret=$(GH_OAUTH_CLIENT_SECRET) $(LDFLAGS)
endif

bin/gh: $(BUILD_FILES)
	@go build -ldflags "$(LDFLAGS)" -o "$@" ./cmd/gh

test:
	go test ./...
.PHONY: test

site:
	git worktree add site gh-pages

site-docs: site
	git -C site pull
	git -C site rm 'manual/gh*.md' 2>/dev/null || true
	go run ./cmd/gen-docs site/manual
	for f in site/manual/gh*.md; do sed -i.bak -e '/^### SEE ALSO/,$$d' "$$f"; done
	rm -f site/manual/*.bak
	git -C site add 'manual/gh*.md'
	git -C site commit -m 'update help docs'
	git -C site push
.PHONY: site-docs
