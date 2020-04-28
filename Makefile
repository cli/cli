BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

GH_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
DATE_FMT = +%Y-%m-%d
ifdef SOURCE_DATE_EPOCH
    BUILD_DATE ?= $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u -r "$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u "$(DATE_FMT)")
else
    BUILD_DATE ?= $(shell date "$(DATE_FMT)")
endif
LDFLAGS := -X github.com/cli/cli/command.Version=$(GH_VERSION) $(LDFLAGS)
LDFLAGS := -X github.com/cli/cli/command.BuildDate=$(BUILD_DATE) $(LDFLAGS)
ifdef GH_OAUTH_CLIENT_SECRET
	LDFLAGS := -X github.com/cli/cli/context.oauthClientID=$(GH_OAUTH_CLIENT_ID) $(LDFLAGS)
	LDFLAGS := -X github.com/cli/cli/context.oauthClientSecret=$(GH_OAUTH_CLIENT_SECRET) $(LDFLAGS)
endif

bin/gh: $(BUILD_FILES)
	@go build -trimpath -ldflags "$(LDFLAGS)" -o "$@" ./cmd/gh

test:
	go test ./...
.PHONY: test

site:
	git clone https://github.com/github/cli.github.com.git "$@"

site-docs: site
	git -C site pull
	git -C site rm 'manual/gh*.md' 2>/dev/null || true
	go run ./cmd/gen-docs site/manual
	for f in site/manual/gh*.md; do sed -i.bak -e '/^### SEE ALSO/,$$d' "$$f"; done
	rm -f site/manual/*.bak
	git -C site add 'manual/gh*.md'
	git -C site commit -m 'update help docs' || true
.PHONY: site-docs

site-publish: site-docs
ifndef GITHUB_REF
	$(error GITHUB_REF is not set)
endif
	sed -i.bak -E 's/(assign version = )".+"/\1"$(GITHUB_REF:refs/tags/v%=%)"/' site/index.html
	rm -f site/index.html.bak
	git -C site commit -m '$(GITHUB_REF:refs/tags/v%=%)' index.html
	git -C site push
.PHONY: site-publish
