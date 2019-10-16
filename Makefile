BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

# export GOFLAGS := -mod=vendor $GOFLAGS

bin/gh: $(BUILD_FILES)
	go build -o "$@"

test:
	go test ./...

.PHONY: test