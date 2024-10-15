CGO_CPPFLAGS ?= ${CPPFLAGS}
export CGO_CPPFLAGS
CGO_CFLAGS ?= ${CFLAGS}
export CGO_CFLAGS
CGO_LDFLAGS ?= $(filter -g -L% -l% -O%,${LDFLAGS})
export CGO_LDFLAGS

EXE =
ifeq ($(shell go env GOOS),windows)
EXE = .exe
endif

## The following tasks delegate to `script/build.go` so they can be run cross-platform.

.PHONY: bin/gh$(EXE)
bin/gh$(EXE): script/build$(EXE)
	@script/build$(EXE) $@

script/build$(EXE): script/build.go
ifeq ($(EXE),)
	GOOS= GOARCH= GOARM= GOFLAGS= CGO_ENABLED= go build -o $@ $<
else
	go build -o $@ $<
endif

.PHONY: clean
clean: script/build$(EXE)
	@$< $@

.PHONY: manpages
manpages: script/build$(EXE)
	@$< $@

.PHONY: completions
completions: bin/gh$(EXE)
	mkdir -p ./share/bash-completion/completions ./share/fish/vendor_completions.d ./share/zsh/site-functions
	bin/gh$(EXE) completion -s bash > ./share/bash-completion/completions/gh
	bin/gh$(EXE) completion -s fish > ./share/fish/vendor_completions.d/gh.fish
	bin/gh$(EXE) completion -s zsh > ./share/zsh/site-functions/_gh

# just convenience tasks around `go test`
.PHONY: test
test:
	go test ./...

# For more information, see https://github.com/cli/cli/blob/trunk/acceptance/README.md
.PHONY: acceptance
acceptance:
	go test -tags acceptance ./acceptance

## Site-related tasks are exclusively intended for use by the GitHub CLI team and for our release automation.

site:
	git clone https://github.com/github/cli.github.com.git "$@"

.PHONY: site-docs
site-docs: site
	git -C site pull
	git -C site rm 'manual/gh*.md' 2>/dev/null || true
	go run ./cmd/gen-docs --website --doc-path site/manual
	rm -f site/manual/*.bak
	git -C site add 'manual/gh*.md'
	git -C site commit -m 'update help docs' || true

.PHONY: site-bump
site-bump: site-docs
ifndef GITHUB_REF
	$(error GITHUB_REF is not set)
endif
	sed -i.bak -E 's/(assign version = )".+"/\1"$(GITHUB_REF:refs/tags/v%=%)"/' site/index.html
	rm -f site/index.html.bak
	git -C site commit -m '$(GITHUB_REF:refs/tags/v%=%)' index.html

## Install/uninstall tasks are here for use on *nix platform. On Windows, there is no equivalent.

DESTDIR :=
prefix  := /usr/local
bindir  := ${prefix}/bin
datadir := ${prefix}/share
mandir  := ${datadir}/man

.PHONY: install
install: bin/gh manpages completions
	install -d ${DESTDIR}${bindir}
	install -m755 bin/gh ${DESTDIR}${bindir}/
	install -d ${DESTDIR}${mandir}/man1
	install -m644 ./share/man/man1/* ${DESTDIR}${mandir}/man1/
	install -d ${DESTDIR}${datadir}/bash-completion/completions
	install -m644 ./share/bash-completion/completions/gh ${DESTDIR}${datadir}/bash-completion/completions/gh
	install -d ${DESTDIR}${datadir}/fish/vendor_completions.d
	install -m644 ./share/fish/vendor_completions.d/gh.fish ${DESTDIR}${datadir}/fish/vendor_completions.d/gh.fish
	install -d ${DESTDIR}${datadir}/zsh/site-functions
	install -m644 ./share/zsh/site-functions/_gh ${DESTDIR}${datadir}/zsh/site-functions/_gh

.PHONY: uninstall
uninstall:
	rm -f ${DESTDIR}${bindir}/gh ${DESTDIR}${mandir}/man1/gh.1 ${DESTDIR}${mandir}/man1/gh-*.1
	rm -f ${DESTDIR}${datadir}/bash-completion/completions/gh
	rm -f ${DESTDIR}${datadir}/fish/vendor_completions.d/gh.fish
	rm -f ${DESTDIR}${datadir}/zsh/site-functions/_gh

.PHONY: macospkg
macospkg: manpages completions
ifndef VERSION
	$(error VERSION is not set. Use `make macospkg VERSION=vX.Y.Z`)
endif
	./script/release --local "$(VERSION)" --platform macos
	./script/pkgmacos $(VERSION)
