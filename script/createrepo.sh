#!/bin/bash
set -e
mkdir -p createrepo
cat > createrepo/Dockerfile << EOF
FROM fedora:40
RUN dnf install -y createrepo_c && dnf clean all
ENTRYPOINT ["createrepo", "/packages"]
EOF

docker build -t createrepo createrepo/
docker run --rm --volume "$PWD/dist":/packages createrepo
rm -rf createrepo
