#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
This file contains basic infrastructure for running the integration test cases.
Most test cases are in v2_integration.py. There are a few exceptions: Test cases
that don't test either the v1 or v2 API are in this file, and test cases that
have to run at a specific point in the cycle (e.g. after all other test cases)
are also in this file.
"""
import argparse
import datetime
import inspect
import json
import os
import random
import re
import requests
import subprocess
import shlex
import signal
import time

import startservers

import v2_integration
from helpers import *

from acme import challenges

# Set the environment variable RACE to anything other than 'true' to disable
# race detection. This significantly speeds up integration testing cycles
# locally.
race_detection = True
if os.environ.get('RACE', 'true') != 'true':
    race_detection = False

def run_go_tests(filterPattern=None,verbose=False):
    """
    run_go_tests launches the Go integration tests. The go test command must
    return zero or an exception will be raised. If the filterPattern is provided
    it is used as the value of the `--test.run` argument to the go test command.
    """
    cmdLine = ["go", "test"]
    if filterPattern is not None and filterPattern != "":
        cmdLine = cmdLine + ["--test.run", filterPattern]
    cmdLine = cmdLine + ["-tags", "integration", "-count=1", "-race"]
    if verbose:
        cmdLine = cmdLine + ["-v"]
    cmdLine = cmdLine +  ["./test/integration"]
    subprocess.check_call(cmdLine, stderr=subprocess.STDOUT)

exit_status = 1

def main():
    parser = argparse.ArgumentParser(description='Run integration tests')
    parser.add_argument('--chisel', dest="run_chisel", action="store_true",
                        help="run integration tests using chisel")
    parser.add_argument('--gotest', dest="run_go", action="store_true",
                        help="run Go integration tests")
    parser.add_argument('--gotestverbose', dest="run_go_verbose", action="store_true",
                        help="run Go integration tests with verbose output")
    parser.add_argument('--filter', dest="test_case_filter", action="store",
                        help="Regex filter for test cases")
    # allow any ACME client to run custom command for integration
    # testing (without having to implement its own busy-wait loop)
    parser.add_argument('--custom', metavar="CMD", help="run custom command")
    parser.set_defaults(run_chisel=False, test_case_filter="", skip_setup=False)
    args = parser.parse_args()

    if not (args.run_chisel or args.custom  or args.run_go is not None):
        raise(Exception("must run at least one of the letsencrypt or chisel tests with --chisel, --gotest, or --custom"))

    if not startservers.install(race_detection=race_detection):
        raise(Exception("failed to build"))

    if not args.test_case_filter:
        now = datetime.datetime.utcnow()

        six_months_ago = now+datetime.timedelta(days=-30*6)
        if not startservers.start(fakeclock=fakeclock(six_months_ago)):
            raise(Exception("startservers failed (mocking six months ago)"))
        setup_six_months_ago()
        startservers.stop()

        twenty_days_ago = now+datetime.timedelta(days=-20)
        if not startservers.start(fakeclock=fakeclock(twenty_days_ago)):
            raise(Exception("startservers failed (mocking twenty days ago)"))
        setup_twenty_days_ago()
        startservers.stop()

    if not startservers.start(fakeclock=None):
        raise(Exception("startservers failed"))

    if args.run_chisel:
        run_chisel(args.test_case_filter)

    if args.run_go:
        run_go_tests(args.test_case_filter, False)
    
    if args.run_go_verbose:
        run_go_tests(args.test_case_filter, True)

    if args.custom:
        run(args.custom.split())

    # Skip the last-phase checks when the test case filter is one, because that
    # means we want to quickly iterate on a single test case.
    if not args.test_case_filter:
        run_cert_checker()
        check_balance()

    if not startservers.check():
        raise(Exception("startservers.check failed"))

    global exit_status
    exit_status = 0

def run_chisel(test_case_filter):
    for key, value in inspect.getmembers(v2_integration):
      if callable(value) and key.startswith('test_') and re.search(test_case_filter, key):
        value()
    for key, value in globals().items():
      if callable(value) and key.startswith('test_') and re.search(test_case_filter, key):
        value()

def check_balance():
    """Verify that gRPC load balancing across backends is working correctly.

    Fetch metrics from each backend and ensure the grpc_server_handled_total
    metric is present, which means that backend handled at least one request.
    """
    addresses = [
        "localhost:8003", # SA
        "localhost:8103", # SA
        "localhost:8009", # publisher
        "localhost:8109", # publisher
        "localhost:8004", # VA
        "localhost:8104", # VA
        "localhost:8001", # CA
        "localhost:8101", # CA
        "localhost:8002", # RA
        "localhost:8102", # RA
    ]
    for address in addresses:
        metrics = requests.get("http://%s/metrics" % address)
        if not "grpc_server_handled_total" in metrics.text:
            raise(Exception("no gRPC traffic processed by %s; load balancing problem?")
                % address)

def run_cert_checker():
    run(["./bin/boulder", "cert-checker", "-config", "%s/cert-checker.json" % config_dir])

if __name__ == "__main__":
    main()
