#!/usr/bin/env bash
#
# This script expects to run on Ubuntu. It installs the dependencies necessary
# to build Boulder and produce a Debian Package. The actual build and packaging
# is handled by a call to Make.
#

# -e Stops execution in the instance of a command or pipeline error.
# -u Treat unset variables as an error and exit immediately.
set -eu

#
# Setup Dependencies
#

sudo apt-get install -y --no-install-recommends \
  ruby \
  ruby-dev \
  gcc

# Download and unpack our production go version. Ensure that $GO_VERSION is
# already set in the environment (e.g. by the github actions release workflow).
$(dirname -- "${0}")/fetch-and-verify-go.sh "${GO_VERSION}"
sudo tar -C /usr/local -xzf go.tar.gz
export PATH=/usr/local/go/bin:$PATH

# Install fpm, this is used in our Makefile to package Boulder as a deb or rpm.
sudo gem install --no-document -v 1.14.0 fpm

#
# Build
#

# Set $ARCHIVEDIR to our current directory. If left unset our Makefile will set
# it to /tmp.
export ARCHIVEDIR="${PWD}"

# Set $VERSION to be a simulacrum of what is set in other build environments.
export VERSION="${GO_VERSION}.$(date +%s)"

# Build Boulder and produce an RPM, a .deb, and a tar.gz file in $PWD.
make rpm deb tar
