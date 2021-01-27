#!/bin/bash
set -e
# This script:

# - creates a dockerfile
# - prepares a docker image that can run `createrepo` that has the latest release rpms
# - "runs" the image by creating a throwaay container
# - copies the result of createrepo out of the throwaway container
# - destroys the throwaway container
mkdir -p createrepo/dist
cat > createrepo/Dockerfile << EOF
FROM fedora:32
RUN yum install -y createrepo_c
RUN mkdir /packages
CMD touch /tmp/foo
COPY dist/*.rpm /packages/
RUN createrepo /packages
EOF

cp dist/*.rpm createrepo/dist/
docker build -t createrepo createrepo/
docker create -ti --name runcreaterepo createrepo bash
docker cp runcreaterepo:/packages/repodata .
docker rm -f runcreaterepo
rm -rf createrepo
