FROM buildpack-deps:focal-scm as godeps
ARG GO_VERSION
# Provided automatically by docker build.
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ENV TARGETPLATFORM=${TARGETPLATFORM:-$BUILDPLATFORM}
ENV GO_VERSION=$GO_VERSION
ENV PATH /usr/local/go/bin:/usr/local/protoc/bin:$PATH
ENV GOBIN /usr/local/bin/
RUN curl "https://dl.google.com/go/go${GO_VERSION}.$(echo $TARGETPLATFORM | sed 's|\/|-|').tar.gz" |\
    tar -C /usr/local -xz
RUN go install github.com/rubenv/sql-migrate/sql-migrate@v1.1.2
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.1
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@bb9882e6ae58f0a80a6390b50a5ec3bd63e46a3c
RUN go install github.com/letsencrypt/pebble/v2/cmd/pebble-challtestsrv@66511d8
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.57.2
RUN go install honnef.co/go/tools/cmd/staticcheck@2023.1.7
RUN go install github.com/jsha/minica@v1.1.0

FROM rust:bullseye as rustdeps
# Provided automatically by docker build.
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ENV TARGETPLATFORM=${TARGETPLATFORM:-$BUILDPLATFORM}
COPY build-rust-deps.sh /tmp/build-rust-deps.sh
RUN /tmp/build-rust-deps.sh

# When the version of Ubuntu (focal, jammy, etc) changes, ensure that the
# version of libc6 is compatible with the rustdeps container above. See
# https://github.com/letsencrypt/boulder/pull/7248#issuecomment-1896612920 for
# more information. 
#
# Run this command in each container: dpkg -l libc6
FROM buildpack-deps:focal-scm
# Provided automatically by docker build.
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ENV TARGETPLATFORM=${TARGETPLATFORM:-$BUILDPLATFORM}
COPY requirements.txt /tmp/requirements.txt
COPY boulder.rsyslog.conf /etc/rsyslog.d/
COPY build.sh /tmp/build.sh
RUN /tmp/build.sh

RUN sed -i '/imklog/s/^/#/' /etc/rsyslog.conf
RUN sed -i '/$ActionFileDefaultTemplate/s/^/#/' /etc/rsyslog.conf
RUN sed -i '/$RepeatedMsgReduction on/s/^/#/' /etc/rsyslog.conf

COPY --from=godeps /usr/local/bin/* /usr/local/bin/
COPY --from=godeps /usr/local/go/ /usr/local/go/
COPY --from=rustdeps /usr/local/cargo/bin/typos /usr/local/bin/typos

ENV PATH /usr/local/go/bin:/usr/local/protoc/bin:$PATH
