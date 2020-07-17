FROM golang:alpine AS builder

# Get build dependencies
RUN apk --no-cache add git make gcc musl-dev

# Build the cli
WORKDIR /go/gh-cli
COPY .git .git
COPY api api
COPY auth auth
COPY cmd cmd
COPY command command
COPY context context
COPY git git
COPY internal internal
COPY pkg pkg
COPY test test
COPY update update
COPY utils utils
COPY Makefile .
COPY go.* .
RUN ["make"]

FROM alpine:3.12 AS runner
COPY --from=builder /go/gh-cli/bin/gh /usr/local/bin
ENTRYPOINT [ "gh" ]
CMD [ "help" ]