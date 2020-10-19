FROM golang:1.15-buster as builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN make bin/gh

FROM debian:buster-slim

ARG VERSION=1.1.0
ARG BUILD_DATE=2020-10-19

LABEL \
  org.opencontainers.image.created="$BUILD_DATE" \
  org.opencontainers.image.authors="noreply@github.comi" \
  org.opencontainers.image.homepage="https://cli.github.com" \
  org.opencontainers.image.documentation="https://cli.github.com/manual" \
  org.opencontainers.image.source="https://github.com/cli/cli" \
  org.opencontainers.image.version="$VERSION" \
  org.opencontainers.image.vendor="GitHub" \
  org.opencontainers.image.licenses="MIT" \
  summary="gh is GitHub on the command line" \
  description="gh is GitHub on the command line. It brings pull requests, issues to the terminal." \
  name="gh"

USER 1001

WORKDIR /app

COPY --from=builder /app/bin/gh .

CMD ["./gh"]
