#!/bin/bash -ex

apt-get update

# Install system deps
apt-get install -y --no-install-recommends \
  mariadb-client-core-10.3 \
  rsyslog \
  build-essential \
  opensc \
  unzip \
  python3-pip \
  gcc \
  ca-certificates \
  softhsm2

PROTO_ARCH=x86_64
if [ "${TARGETPLATFORM}" = linux/arm64 ]
then
  PROTO_ARCH=aarch_64
fi

curl -L https://github.com/google/protobuf/releases/download/v3.20.1/protoc-3.20.1-linux-"${PROTO_ARCH}".zip -o /tmp/protoc.zip
unzip /tmp/protoc.zip -d /usr/local/protoc

pip3 install -r /tmp/requirements.txt

apt-get clean -y

# Tell git to trust the directory where the boulder repo volume is mounted
# by `docker compose`.
git config --global --add safe.directory /boulder

rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
