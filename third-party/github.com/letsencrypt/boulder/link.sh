#!/usr/bin/env bash
#
# Symlink the various boulder subcommands into place.
#
BINDIR="$PWD/bin"
for n in `"${BINDIR}/boulder" --list` ; do
  ln -sf boulder "${BINDIR}/$n"
done
