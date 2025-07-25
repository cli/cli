#!/usr/bin/env -S python3 -u
"""
Run a local instance of Boulder for testing purposes.

Boulder always runs as a collection of services. This script will
start them all on their own ports (see test/startservers.py)

Keeps servers alive until ^C. Exit non-zero if any servers fail to
start, or die before ^C.
"""

import errno
import os
import sys
import time

sys.path.append('./test')
import startservers

if not startservers.install(race_detection=False):
    raise(Exception("failed to build"))

if not startservers.start(fakeclock=None):
    sys.exit(1)
try:
    os.wait()

    # If we reach here, a child died early. Log what died:
    startservers.check()
    sys.exit(1)
except KeyboardInterrupt:
    print("\nstopping servers.")
except OSError as v:
    # Ignore EINTR, which happens when we get SIGTERM or SIGINT (i.e. when
    # someone hits Ctrl-C after running `docker compose up` or start.py.
    if v.errno != errno.EINTR:
        raise
