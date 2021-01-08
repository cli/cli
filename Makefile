BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

GH_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
DATE_FMT = +%Y-%m-%d
ifdef SOURCE_DATE_EPOCH
    BUILD_DATE ?= $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u -r "$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u "$(DATE_FMT)")
else
    BUILD_DATE ?= $(shell date "$(DATE_FMT)")
endif

CGO_CPPFLAGS ?= ${CPPFLAGS}
export CGO_CPPFLAGS
CGO_CFLAGS ?= ${CFLAGS}
export CGO_CFLAGS
CGO_LDFLAGS ?= $(filter -g -L% -l% -O%,${LDFLAGS})
export CGO_LDFLAGS

GO_LDFLAGS := -X github.com/cli/cli/internal/build.Version=$(GH_VERSION) $(GO_LDFLAGS)
GO_LDFLAGS := -X github.com/cli/cli/internal/build.Date=$(BUILD_DATE) $(GO_LDFLAGS)
ifdef GH_OAUTH_CLIENT_SECRET
	GO_LDFLAGS := -X github.com/cli/cli/internal/authflow.oauthClientID=$(GH_OAUTH_CLIENT_ID) $(GO_LDFLAGS)
	GO_LDFLAGS := -X github.com/cli/cli/internal/authflow.oauthClientSecret=$(GH_OAUTH_CLIENT_SECRET) $(GO_LDFLAGS)
endif

bin/gh: $(BUILD_FILES)
	go build -trimpath -ldflags "${GO_LDFLAGS}" -o "$@" ./cmd/gh

clean:
	rm -rf ./bin ./share
.PHONY: clean

test:
	go test ./...
.PHONY: test

site:
	git clone https://github.com/github/cli.github.com.git "$@"

site-docs: site
	git -C site pull
	git -C site rm 'manual/gh*.md' 2>/dev/null || true
	go run ./cmd/gen-docs --website --doc-path site/manual
	rm -f site/manual/*.bak
	git -C site add 'manual/gh*.md'
	git -C site commit -m 'update help docs' || true
.PHONY: site-docs

site-bump: site-docs
ifndef GITHUB_REF
	$(error GITHUB_REF is not set)
endif
	sed -i.bak -E 's/(assign version = )".+"/\1"$(GITHUB_REF:refs/tags/v%=%)"/' site/index.html
	rm -f site/index.html.bak
	git -C site commit -m '$(GITHUB_REF:refs/tags/v%=%)' index.html
.PHONY: site-bump

.PHONY: manpages
manpages:
	go run ./cmd/gen-docs --man-page --doc-path ./share/man/man1/

DESTDIR :=
prefix  := /usr/local
bindir  := ${prefix}/bin
mandir  := ${prefix}/share/man

.PHONY: install
install: bin/gh manpages
	install -d ${DESTDIR}${bindir}
	install -m755 bin/gh ${DESTDIR}${bindir}/
	install -d ${DESTDIR}${mandir}/man1
	install -m644 ./share/man/man1/* ${DESTDIR}${mandir}/man1/

.PHONY: uninstall
uninstall:
	rm -f ${DESTDIR}${bindir}/gh ${DESTDIR}${mandir}/man1/gh.1 ${DESTDIR}${mandir}/man1/gh-*.1
