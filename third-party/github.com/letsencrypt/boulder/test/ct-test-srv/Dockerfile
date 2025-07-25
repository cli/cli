# syntax=docker/dockerfile:1
ARG GO_VERSION

FROM golang:${GO_VERSION} AS build

WORKDIR /app

COPY go.mod go.sum vendor ./

COPY . .

RUN go build -o /bin/ct-test-srv ./test/ct-test-srv/main.go

FROM ubuntu:24.04

RUN useradd -r -u 10001 cttest

COPY --from=build /bin/ct-test-srv /bin/ct-test-srv

COPY test/ct-test-srv/ct-test-srv.json /etc/ct-test-srv.json

ENTRYPOINT ["/bin/ct-test-srv"]

USER cttest

CMD ["-config", "/etc/ct-test-srv.json"]
