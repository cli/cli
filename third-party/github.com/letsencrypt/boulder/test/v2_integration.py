# -*- coding: utf-8 -*-
"""
Integration test cases for ACMEv2 as implemented by boulder-wfe2.
"""
import subprocess
import requests
import datetime
import time
import os
import json
import re

from cryptography import x509
from cryptography.hazmat.backends import default_backend
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.hazmat.primitives import serialization

import chisel2
from helpers import *

from acme import errors as acme_errors

from acme.messages import Status, CertificateRequest, Directory, NewRegistration
from acme import crypto_util as acme_crypto_util
from acme import client as acme_client
from acme import messages
from acme import challenges
from acme import errors

import josepy

import tempfile
import shutil
import atexit
import random
import string

import threading
from http.server import HTTPServer, BaseHTTPRequestHandler
import socketserver
import socket

import challtestsrv
challSrv = challtestsrv.ChallTestServer()

def test_multidomain():
    chisel2.auth_and_issue([random_domain(), random_domain()])

def test_wildcardmultidomain():
    """
    Test issuance for a random domain and a random wildcard domain using DNS-01.
    """
    chisel2.auth_and_issue([random_domain(), "*."+random_domain()], chall_type="dns-01")

def test_http_challenge():
    chisel2.auth_and_issue([random_domain(), random_domain()], chall_type="http-01")

def rand_http_chall(client):
    d = random_domain()
    csr_pem = chisel2.make_csr([d])
    order = client.new_order(csr_pem)
    authzs = order.authorizations
    for a in authzs:
        for c in a.body.challenges:
            if isinstance(c.chall, challenges.HTTP01):
                return d, c.chall
    raise(Exception("No HTTP-01 challenge found for random domain authz"))

def check_challenge_dns_err(chalType):
    """
    check_challenge_dns_err tests that performing an ACME challenge of the
    specified type to a hostname that is configured to return SERVFAIL for all
    queries produces the correct problem type and detail message.
    """
    client = chisel2.make_client()

    # Create a random domains.
    d = random_domain()

    # Configure the chall srv to SERVFAIL all queries for that domain.
    challSrv.add_servfail_response(d)

    # Expect a DNS problem with a detail that matches a regex
    expectedProbType = "dns"
    expectedProbRegex = re.compile(r"SERVFAIL looking up (A|AAAA|TXT|CAA) for {0}".format(d))

    # Try and issue for the domain with the given challenge type.
    failed = False
    try:
        chisel2.auth_and_issue([d], client=client, chall_type=chalType)
    except acme_errors.ValidationError as e:
        # Mark that the auth_and_issue failed
        failed = True
        # Extract the failed challenge from each failed authorization
        for authzr in e.failed_authzrs:
            c = None
            if chalType == "http-01":
                c = chisel2.get_chall(authzr, challenges.HTTP01)
            elif chalType == "dns-01":
                c = chisel2.get_chall(authzr, challenges.DNS01)
            elif chalType == "tls-alpn-01":
                c = chisel2.get_chall(authzr, challenges.TLSALPN01)
            else:
                raise(Exception("Invalid challenge type requested: {0}".format(challType)))

            # The failed challenge's error should match expected
            error = c.error
            if error is None or error.typ != "urn:ietf:params:acme:error:{0}".format(expectedProbType):
                raise(Exception("Expected {0} prob, got {1}".format(expectedProbType, error.typ)))
            if not expectedProbRegex.search(error.detail):
                raise(Exception("Prob detail did not match expectedProbRegex, got \"{0}\"".format(error.detail)))
    finally:
        challSrv.remove_servfail_response(d)

    # If there was no exception that means something went wrong. The test should fail.
    if failed is False:
        raise(Exception("No problem generated issuing for broken DNS identifier"))

def test_http_challenge_dns_err():
    """
    test_http_challenge_dns_err tests that a HTTP-01 challenge for a domain
    with broken DNS produces the correct problem response.
    """
    check_challenge_dns_err("http-01")

def test_dns_challenge_dns_err():
    """
    test_dns_challenge_dns_err tests that a DNS-01 challenge for a domain
    with broken DNS produces the correct problem response.
    """
    check_challenge_dns_err("dns-01")

def test_tls_alpn_challenge_dns_err():
    """
    test_tls_alpn_challenge_dns_err tests that a TLS-ALPN-01 challenge for a domain
    with broken DNS produces the correct problem response.
    """
    check_challenge_dns_err("tls-alpn-01")

def test_http_challenge_broken_redirect():
    """
    test_http_challenge_broken_redirect tests that a common webserver
    misconfiguration receives the correct specialized error message when attempting
    an HTTP-01 challenge.
    """
    client = chisel2.make_client()

    # Create an authz for a random domain and get its HTTP-01 challenge token
    d, chall = rand_http_chall(client)
    token = chall.encode("token")

    # Create a broken HTTP redirect similar to a sort we see frequently "in the wild"
    challengePath = "/.well-known/acme-challenge/{0}".format(token)
    redirect = "http://{0}.well-known/acme-challenge/bad-bad-bad".format(d)
    challSrv.add_http_redirect(
        challengePath,
        redirect)

    # Expect the specialized error message
    expectedError = "64.112.117.122: Fetching {0}: Invalid host in redirect target \"{1}.well-known\". Check webserver config for missing '/' in redirect target.".format(redirect, d)

    # NOTE(@cpu): Can't use chisel2.expect_problem here because it doesn't let
    # us interrogate the detail message easily.
    try:
        chisel2.auth_and_issue([d], client=client, chall_type="http-01")
    except acme_errors.ValidationError as e:
        for authzr in e.failed_authzrs:
            c = chisel2.get_chall(authzr, challenges.HTTP01)
            error = c.error
            if error is None or error.typ != "urn:ietf:params:acme:error:connection":
                raise(Exception("Expected connection prob, got %s" % (error.__str__())))
            if error.detail != expectedError:
                raise(Exception("Expected prob detail %s, got %s" % (expectedError, error.detail)))

    challSrv.remove_http_redirect(challengePath)

def test_failed_validation_limit():
    """
    Fail a challenge repeatedly for the same domain, with the same account. Once
    we reach the rate limit we should get a rateLimitedError. Note that this
    depends on the specific threshold configured.

    This also incidentally tests a fix for
    https://github.com/letsencrypt/boulder/issues/4329. We expect to get
    ValidationErrors, eventually followed by a rate limit error.
    """
    domain = "fail." + random_domain()
    csr_pem = chisel2.make_csr([domain])
    client = chisel2.make_client()
    threshold = 3
    for _ in range(threshold):
        order = client.new_order(csr_pem)
        chall = order.authorizations[0].body.challenges[0]
        client.answer_challenge(chall, chall.response(client.net.key))
        try:
            client.poll_and_finalize(order)
        except errors.ValidationError as e:
            pass
    chisel2.expect_problem("urn:ietf:params:acme:error:rateLimited",
        lambda: chisel2.auth_and_issue([domain], client=client))


def test_http_challenge_loop_redirect():
    client = chisel2.make_client()

    # Create an authz for a random domain and get its HTTP-01 challenge token
    d, chall = rand_http_chall(client)
    token = chall.encode("token")

    # Create a HTTP redirect from the challenge's validation path to itself
    challengePath = "/.well-known/acme-challenge/{0}".format(token)
    challSrv.add_http_redirect(
        challengePath,
        "http://{0}{1}".format(d, challengePath))

    # Issuing for the name should fail because of the challenge domains's
    # redirect loop.
    chisel2.expect_problem("urn:ietf:params:acme:error:connection",
        lambda: chisel2.auth_and_issue([d], client=client, chall_type="http-01"))

    challSrv.remove_http_redirect(challengePath)

def test_http_challenge_badport_redirect():
    client = chisel2.make_client()

    # Create an authz for a random domain and get its HTTP-01 challenge token
    d, chall = rand_http_chall(client)
    token = chall.encode("token")

    # Create a HTTP redirect from the challenge's validation path to a host with
    # an invalid port.
    challengePath = "/.well-known/acme-challenge/{0}".format(token)
    challSrv.add_http_redirect(
        challengePath,
        "http://{0}:1337{1}".format(d, challengePath))

    # Issuing for the name should fail because of the challenge domain's
    # invalid port redirect.
    chisel2.expect_problem("urn:ietf:params:acme:error:connection",
        lambda: chisel2.auth_and_issue([d], client=client, chall_type="http-01"))

    challSrv.remove_http_redirect(challengePath)

def test_http_challenge_badhost_redirect():
    client = chisel2.make_client()

    # Create an authz for a random domain and get its HTTP-01 challenge token
    d, chall = rand_http_chall(client)
    token = chall.encode("token")

    # Create a HTTP redirect from the challenge's validation path to a bare IP
    # hostname.
    challengePath = "/.well-known/acme-challenge/{0}".format(token)
    challSrv.add_http_redirect(
        challengePath,
        "https://127.0.0.1{0}".format(challengePath))

    # Issuing for the name should cause a connection error because the redirect
    # domain name is an IP address.
    chisel2.expect_problem("urn:ietf:params:acme:error:connection",
        lambda: chisel2.auth_and_issue([d], client=client, chall_type="http-01"))

    challSrv.remove_http_redirect(challengePath)

def test_http_challenge_badproto_redirect():
    client = chisel2.make_client()

    # Create an authz for a random domain and get its HTTP-01 challenge token
    d, chall = rand_http_chall(client)
    token = chall.encode("token")

    # Create a HTTP redirect from the challenge's validation path to whacky
    # non-http/https protocol URL.
    challengePath = "/.well-known/acme-challenge/{0}".format(token)
    challSrv.add_http_redirect(
        challengePath,
        "gopher://{0}{1}".format(d, challengePath))

    # Issuing for the name should cause a connection error because the redirect
    # domain name is an IP address.
    chisel2.expect_problem("urn:ietf:params:acme:error:connection",
        lambda: chisel2.auth_and_issue([d], client=client, chall_type="http-01"))

    challSrv.remove_http_redirect(challengePath)

def test_http_challenge_http_redirect():
    client = chisel2.make_client()

    # Create an authz for a random domain and get its HTTP-01 challenge token
    d, chall = rand_http_chall(client)
    token = chall.encode("token")
    # Calculate its keyauth so we can add it in a special non-standard location
    # for the redirect result
    resp = chall.response(client.net.key)
    keyauth = resp.key_authorization
    challSrv.add_http01_response("http-redirect", keyauth)

    # Create a HTTP redirect from the challenge's validation path to some other
    # token path where we have registered the key authorization.
    challengePath = "/.well-known/acme-challenge/{0}".format(token)
    redirectPath = "/.well-known/acme-challenge/http-redirect?params=are&important=to&not=lose"
    challSrv.add_http_redirect(
        challengePath,
        "http://{0}{1}".format(d, redirectPath))

    chisel2.auth_and_issue([d], client=client, chall_type="http-01")

    challSrv.remove_http_redirect(challengePath)
    challSrv.remove_http01_response("http-redirect")

    history = challSrv.http_request_history(d)
    challSrv.clear_http_request_history(d)

    # There should have been at least two GET requests made to the
    # challtestsrv. There may have been more if remote VAs were configured.
    if len(history) < 2:
        raise(Exception("Expected at least 2 HTTP request events on challtestsrv, found {1}".format(len(history))))

    initialRequests = []
    redirectedRequests = []

    for request in history:
      # All requests should have been over HTTP
      if request['HTTPS'] is True:
        raise(Exception("Expected all requests to be HTTP"))
      # Initial requests should have the expected initial HTTP-01 URL for the challenge
      if request['URL'] == challengePath:
        initialRequests.append(request)
      # Redirected requests should have the expected redirect path URL with all
      # its parameters
      elif request['URL'] == redirectPath:
        redirectedRequests.append(request)
      else:
        raise(Exception("Unexpected request URL {0} in challtestsrv history: {1}".format(request['URL'], request)))

    # There should have been at least 1 initial HTTP-01 validation request.
    if len(initialRequests) < 1:
        raise(Exception("Expected {0} initial HTTP-01 request events on challtestsrv, found {1}".format(validation_attempts, len(initialRequests))))

    # There should have been at least 1 redirected HTTP request for each VA
    if len(redirectedRequests) < 1:
        raise(Exception("Expected {0} redirected HTTP-01 request events on challtestsrv, found {1}".format(validation_attempts, len(redirectedRequests))))

def test_http_challenge_https_redirect():
    client = chisel2.make_client()

    # Create an authz for a random domain and get its HTTP-01 challenge token
    d, chall = rand_http_chall(client)
    token = chall.encode("token")
    # Calculate its keyauth so we can add it in a special non-standard location
    # for the redirect result
    resp = chall.response(client.net.key)
    keyauth = resp.key_authorization
    challSrv.add_http01_response("https-redirect", keyauth)

    # Create a HTTP redirect from the challenge's validation path to an HTTPS
    # path with some parameters
    challengePath = "/.well-known/acme-challenge/{0}".format(token)
    redirectPath = "/.well-known/acme-challenge/https-redirect?params=are&important=to&not=lose"
    challSrv.add_http_redirect(
        challengePath,
        "https://{0}{1}".format(d, redirectPath))

    # Also add an A record for the domain pointing to the interface that the
    # HTTPS HTTP-01 challtestsrv is bound.
    challSrv.add_a_record(d, ["64.112.117.122"])

    try:
        chisel2.auth_and_issue([d], client=client, chall_type="http-01")
    except errors.ValidationError as e:
        problems = []
        for authzr in e.failed_authzrs:
            for chall in authzr.body.challenges:
                error = chall.error
                if error:
                    problems.append(error.__str__())
        raise(Exception("validation problem: %s" % "; ".join(problems)))

    challSrv.remove_http_redirect(challengePath)
    challSrv.remove_a_record(d)

    history = challSrv.http_request_history(d)
    challSrv.clear_http_request_history(d)

    # There should have been at least two GET requests made to the challtestsrv by the VA
    if len(history) < 2:
        raise(Exception("Expected 2 HTTP request events on challtestsrv, found {0}".format(len(history))))

    initialRequests = []
    redirectedRequests = []

    for request in history:
      # Initial requests should have the expected initial HTTP-01 URL for the challenge
      if request['URL'] == challengePath:
        initialRequests.append(request)
      # Redirected requests should have the expected redirect path URL with all
      # its parameters
      elif request['URL'] == redirectPath:
        redirectedRequests.append(request)
      else:
        raise(Exception("Unexpected request URL {0} in challtestsrv history: {1}".format(request['URL'], request)))

    # There should have been at least 1 initial HTTP-01 validation request.
    if len(initialRequests) < 1:
        raise(Exception("Expected {0} initial HTTP-01 request events on challtestsrv, found {1}".format(validation_attempts, len(initialRequests))))
     # All initial requests should have been over HTTP
    for r in initialRequests:
      if r['HTTPS'] is True:
        raise(Exception("Expected all initial requests to be HTTP, got %s" % r))

    # There should have been at least 1 redirected HTTP request for each VA
    if len(redirectedRequests) < 1:
        raise(Exception("Expected {0} redirected HTTP-01 request events on challtestsrv, found {1}".format(validation_attempts, len(redirectedRequests))))
    # All the redirected requests should have been over HTTPS with the correct
    # SNI value
    for r in redirectedRequests:
      if r['HTTPS'] is False:
        raise(Exception("Expected all redirected requests to be HTTPS"))
      if r['ServerName'] != d:
        raise(Exception("Expected all redirected requests to have ServerName {0} got \"{1}\"".format(d, r['ServerName'])))

class SlowHTTPRequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        try:
            # Sleeptime needs to be larger than the RA->VA timeout (20s at the
            # time of writing)
            sleeptime = 22
            print("SlowHTTPRequestHandler: sleeping for {0}s\n".format(sleeptime))
            time.sleep(sleeptime)
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"this is not an ACME key authorization")
        except:
            pass

class SlowHTTPServer(HTTPServer):
    # Override handle_error so we don't print a misleading stack trace when the
    # VA terminates the connection due to timeout.
    def handle_error(self, request, client_address):
        pass

def test_http_challenge_timeout():
    """
    test_http_challenge_timeout tests that the VA times out challenge requests
    to a slow HTTP server appropriately.
    """
    # Start a simple python HTTP server on port 80 in its own thread.
    # NOTE(@cpu): The chall-test-srv binds 64.112.117.122:80 for HTTP-01
    # challenges so we must use the 64.112.117.134 address for the throw away
    # server for this test and add a mock DNS entry that directs the VA to it.
    httpd = SlowHTTPServer(("64.112.117.134", 80), SlowHTTPRequestHandler)
    thread = threading.Thread(target = httpd.serve_forever)
    thread.daemon = False
    thread.start()

    # Pick a random domain
    hostname = random_domain()

    # Add A record for the domains to ensure the VA's requests are directed
    # to the interface that we bound the HTTPServer to.
    challSrv.add_a_record(hostname, ["64.112.117.134"])

    start = datetime.datetime.utcnow()
    end = 0

    try:
        # We expect a connection timeout error to occur
        chisel2.expect_problem("urn:ietf:params:acme:error:connection",
            lambda: chisel2.auth_and_issue([hostname], chall_type="http-01"))
        end = datetime.datetime.utcnow()
    finally:
        # Shut down the HTTP server gracefully and join on its thread.
        httpd.shutdown()
        httpd.server_close()
        thread.join()

    delta = end - start
    # Expected duration should be the RA->VA timeout plus some padding (At
    # present the timeout is 20s so adding 2s of padding = 22s)
    expectedDuration = 22
    if delta.total_seconds() == 0 or delta.total_seconds() > expectedDuration:
        raise(Exception("expected timeout to occur in under {0} seconds. Took {1}".format(expectedDuration, delta.total_seconds())))


def test_tls_alpn_challenge():
    # Pick two random domains
    domains = [random_domain(),random_domain()]

    # Add A records for these domains to ensure the VA's requests are directed
    # to the interface that the challtestsrv has bound for TLS-ALPN-01 challenge
    # responses
    for host in domains:
        challSrv.add_a_record(host, ["64.112.117.134"])
    chisel2.auth_and_issue(domains, chall_type="tls-alpn-01")

    for host in domains:
        challSrv.remove_a_record(host)

def test_overlapping_wildcard():
    """
    Test issuance for a random domain and a wildcard version of the same domain
    using DNS-01. This should result in *two* distinct authorizations.
    """
    domain = random_domain()
    domains = [ domain, "*."+domain ]
    client = chisel2.make_client(None)
    csr_pem = chisel2.make_csr(domains)
    order = client.new_order(csr_pem)
    authzs = order.authorizations

    if len(authzs) != 2:
        raise(Exception("order for %s had %d authorizations, expected 2" %
                (domains, len(authzs))))

    cleanup = chisel2.do_dns_challenges(client, authzs)
    try:
        order = client.poll_and_finalize(order)
    finally:
        cleanup()

def test_highrisk_blocklist():
    """
    Test issuance for a subdomain of a HighRiskBlockedNames entry. It should
    fail with a policy error.
    """

    # We include "example.org" in `test/hostname-policy.yaml` in the
    # HighRiskBlockedNames list so issuing for "foo.example.org" should be
    # blocked.
    domain = "foo.example.org"
    # We expect this to produce a policy problem
    chisel2.expect_problem("urn:ietf:params:acme:error:rejectedIdentifier",
        lambda: chisel2.auth_and_issue([domain], chall_type="dns-01"))

def test_wildcard_exactblacklist():
    """
    Test issuance for a wildcard that would cover an exact blacklist entry. It
    should fail with a policy error.
    """

    # We include "highrisk.le-test.hoffman-andrews.com" in `test/hostname-policy.yaml`
    # Issuing for "*.le-test.hoffman-andrews.com" should be blocked
    domain = "*.le-test.hoffman-andrews.com"
    # We expect this to produce a policy problem
    chisel2.expect_problem("urn:ietf:params:acme:error:rejectedIdentifier",
        lambda: chisel2.auth_and_issue([domain], chall_type="dns-01"))

def test_wildcard_authz_reuse():
    """
    Test that an authorization for a base domain obtained via HTTP-01 isn't
    reused when issuing a wildcard for that base domain later on.
    """

    # Create one client to reuse across multiple issuances
    client = chisel2.make_client(None)

    # Pick a random domain to issue for
    domains = [ random_domain() ]
    csr_pem = chisel2.make_csr(domains)

    # Submit an order for the name
    order = client.new_order(csr_pem)
    # Complete the order via an HTTP-01 challenge
    cleanup = chisel2.do_http_challenges(client, order.authorizations)
    try:
        order = client.poll_and_finalize(order)
    finally:
        cleanup()

    # Now try to issue a wildcard for the random domain
    domains[0] = "*." + domains[0]
    csr_pem = chisel2.make_csr(domains)
    order = client.new_order(csr_pem)

    # We expect all of the returned authorizations to be pending status
    for authz in order.authorizations:
        if authz.body.status != Status("pending"):
            raise(Exception("order for %s included a non-pending authorization (status: %s) from a previous HTTP-01 order" %
                    ((domains), str(authz.body.status))))

def test_bad_overlap_wildcard():
    chisel2.expect_problem("urn:ietf:params:acme:error:malformed",
        lambda: chisel2.auth_and_issue(["*.example.com", "www.example.com"]))

def test_duplicate_orders():
    """
    Test that the same client issuing for the same domain names twice in a row
    works without error.
    """
    client = chisel2.make_client(None)
    domains = [ random_domain() ]
    chisel2.auth_and_issue(domains, client=client)
    chisel2.auth_and_issue(domains, client=client)

def test_order_reuse_failed_authz():
    """
    Test that creating an order for a domain name, failing an authorization in
    that order, and submitting another new order request for the same name
    doesn't reuse a failed authorization in the new order.
    """

    client = chisel2.make_client(None)
    domains = [ random_domain() ]
    csr_pem = chisel2.make_csr(domains)

    order = client.new_order(csr_pem)
    firstOrderURI = order.uri

    # Pick the first authz's first challenge, doesn't matter what type it is
    chall_body = order.authorizations[0].body.challenges[0]
    # Answer it, but with nothing set up to solve the challenge request
    client.answer_challenge(chall_body, chall_body.response(client.net.key))

    deadline = datetime.datetime.now() + datetime.timedelta(seconds=60)
    authzFailed = False
    try:
        # Poll the order's authorizations until they are non-pending, a timeout
        # occurs, or there is an invalid authorization status.
        client.poll_authorizations(order, deadline)
    except acme_errors.ValidationError as e:
        # We expect there to be a ValidationError from one of the authorizations
        # being invalid.
        authzFailed = True

    # If the poll ended and an authz's status isn't invalid then we reached the
    # deadline, fail the test
    if not authzFailed:
        raise(Exception("timed out waiting for order %s to become invalid" % firstOrderURI))

    # Make another order with the same domains
    order = client.new_order(csr_pem)

    # It should not be the same order as before
    if order.uri == firstOrderURI:
        raise(Exception("new-order for %s returned a , now-invalid, order" % domains))

    # We expect all of the returned authorizations to be pending status
    for authz in order.authorizations:
        if authz.body.status != Status("pending"):
            raise(Exception("order for %s included a non-pending authorization (status: %s) from a previous order" %
                    ((domains), str(authz.body.status))))

    # We expect the new order can be fulfilled
    cleanup = chisel2.do_http_challenges(client, order.authorizations)
    try:
        order = client.poll_and_finalize(order)
    finally:
        cleanup()

def test_only_return_existing_reg():
    client = chisel2.uninitialized_client()
    email = "test@not-example.com"
    client.new_account(messages.NewRegistration.from_data(email=email,
            terms_of_service_agreed=True))

    client = chisel2.uninitialized_client(key=client.net.key)
    class extendedAcct(dict):
        def json_dumps(self, indent=None):
            return json.dumps(self)
    acct = extendedAcct({
        "termsOfServiceAgreed": True,
        "contact": [email],
        "onlyReturnExisting": True
    })
    resp = client.net.post(client.directory['newAccount'], acct)
    if resp.status_code != 200:
        raise(Exception("incorrect response returned for onlyReturnExisting"))

    other_client = chisel2.uninitialized_client()
    newAcct = extendedAcct({
        "termsOfServiceAgreed": True,
        "contact": [email],
        "onlyReturnExisting": True
    })
    chisel2.expect_problem("urn:ietf:params:acme:error:accountDoesNotExist",
        lambda: other_client.net.post(other_client.directory['newAccount'], newAcct))

def BouncerHTTPRequestHandler(redirect, guestlist):
    """
    BouncerHTTPRequestHandler returns a BouncerHandler class that acts like
    a club bouncer in front of another server. The bouncer will respond to
    GET requests by looking up the allowed number of requests in the guestlist
    for the User-Agent making the request. If there is at least one guestlist
    spot for that UA it will be redirected to the real server and the
    guestlist will be decremented. Once the guestlist spots for a UA are
    expended requests will get a bogus result and have to stand outside in the
    cold
    """
    class BouncerHandler(BaseHTTPRequestHandler):
        def __init__(self, *args, **kwargs):
            BaseHTTPRequestHandler.__init__(self, *args, **kwargs)

        def do_HEAD(self):
            # This is used by wait_for_server
            self.send_response(200)
            self.end_headers()

        def do_GET(self):
            ua = self.headers['User-Agent']
            guestlistAllows = BouncerHandler.guestlist.get(ua, 0)
            # If there is still space on the guestlist for this UA then redirect
            # the request and decrement the guestlist.
            if guestlistAllows > 0:
                BouncerHandler.guestlist[ua] -= 1
                self.log_message("BouncerHandler UA {0} is on the Guestlist. {1} requests remaining.".format(ua, BouncerHandler.guestlist[ua]))
                self.send_response(302)
                self.send_header("Location", BouncerHandler.redirect)
                self.end_headers()
            # Otherwise return a bogus result
            else:
                self.log_message("BouncerHandler UA {0} has no requests on the Guestlist. Sending request to the curb".format(ua))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(u"(• ◡ •) <( VIPs only! )".encode())

    BouncerHandler.guestlist = guestlist
    BouncerHandler.redirect = redirect
    return BouncerHandler

def wait_for_server(addr):
    while True:
        try:
            # NOTE(@cpu): Using HEAD here instead of GET because the
            # BouncerHandler modifies its state for GET requests.
            status = requests.head(addr).status_code
            if status == 200:
                return
        except requests.exceptions.ConnectionError:
            pass
        time.sleep(0.5)

def multiva_setup(client, guestlist):
    """
    Setup a testing domain and backing multiva server setup. This will block
    until the server is ready. The returned cleanup function should be used to
    stop the server. The first bounceFirst requests to the server will be sent
    to the real challtestsrv for a good answer, the rest will get a bad
    answer. Domain name is randomly chosen with random_domain().
    """
    hostname = random_domain()

    csr_pem = chisel2.make_csr([hostname])
    order = client.new_order(csr_pem)
    authz = order.authorizations[0]
    chall = None
    for c in authz.body.challenges:
        if isinstance(c.chall, challenges.HTTP01):
            chall = c.chall
    if chall is None:
        raise(Exception("No HTTP-01 challenge found for random domain authz"))

    token = chall.encode("token")

    # Calculate the challenge's keyauth so we can add a good keyauth response on
    # the real challtestsrv that we redirect VIP requests to.
    resp = chall.response(client.net.key)
    keyauth = resp.key_authorization
    challSrv.add_http01_response(token, keyauth)

    # Add an A record for the domains to ensure the VA's requests are directed
    # to the interface that we bound the HTTPServer to.
    challSrv.add_a_record(hostname, ["64.112.117.134"])

    # Add an A record for the redirect target that sends it to the real chall
    # test srv for a valid HTTP-01 response.
    redirHostname = "chall-test-srv.example.com"
    challSrv.add_a_record(redirHostname, ["64.112.117.122"])

    # Start a simple python HTTP server on port 80 in its own thread.
    # NOTE(@cpu): The chall-test-srv binds 64.112.117.122:80 for HTTP-01
    # challenges so we must use the 64.112.117.134 address for the throw away
    # server for this test and add a mock DNS entry that directs the VA to it.
    redirect = "http://{0}/.well-known/acme-challenge/{1}".format(
            redirHostname, token)
    httpd = HTTPServer(("64.112.117.134", 80), BouncerHTTPRequestHandler(redirect, guestlist))
    thread = threading.Thread(target = httpd.serve_forever)
    thread.daemon = False
    thread.start()

    def cleanup():
        # Remove the challtestsrv mocks
        challSrv.remove_a_record(hostname)
        challSrv.remove_a_record(redirHostname)
        challSrv.remove_http01_response(token)
        # Shut down the HTTP server gracefully and join on its thread.
        httpd.shutdown()
        httpd.server_close()
        thread.join()

    return hostname, cleanup

def test_http_multiva_threshold_pass():
    client = chisel2.make_client()

    # Configure a guestlist that will pass the multiVA threshold test by
    # allowing the primary VA at some, but not all, remotes.
    # In particular, remoteva-c is missing.
    guestlist = {"boulder": 1, "remoteva-a": 1, "remoteva-b": 1}

    hostname, cleanup = multiva_setup(client, guestlist)

    try:
        # With the maximum number of allowed remote VA failures the overall
        # challenge should still succeed.
        chisel2.auth_and_issue([hostname], client=client, chall_type="http-01")
    finally:
        cleanup()

def test_http_multiva_primary_fail_remote_pass():
    client = chisel2.make_client()

    # Configure a guestlist that will fail the primary VA check but allow all of
    # the remote VAs.
    guestlist = {"boulder": 0, "remoteva-a": 1, "remoteva-b": 1}

    hostname, cleanup = multiva_setup(client, guestlist)

    foundException = False

    try:
        # The overall validation should fail even if the remotes are allowed
        # because the primary VA result cannot be overridden.
        chisel2.auth_and_issue([hostname], client=client, chall_type="http-01")
    except acme_errors.ValidationError as e:
        # NOTE(@cpu): Chisel2's expect_problem doesn't work in this case so this
        # test needs to unpack an `acme_errors.ValidationError` on its own. It
        # might be possible to clean this up in the future.
        if len(e.failed_authzrs) != 1:
            raise(Exception("expected one failed authz, found {0}".format(len(e.failed_authzrs))))
        challs = e.failed_authzrs[0].body.challenges
        httpChall = None
        for chall_body in challs:
            if isinstance(chall_body.chall, challenges.HTTP01):
                httpChall = chall_body
        if httpChall is None:
            raise(Exception("no HTTP-01 challenge in failed authz"))
        if httpChall.error.typ != "urn:ietf:params:acme:error:unauthorized":
            raise(Exception("expected unauthorized prob, found {0}".format(httpChall.error.typ)))
        foundException = True
    finally:
        cleanup()
        if foundException is False:
            raise(Exception("Overall validation did not fail"))

def test_http_multiva_threshold_fail():
    client = chisel2.make_client()

    # Configure a guestlist that will fail the multiVA threshold test by
    # only allowing the primary VA.
    guestlist = {"boulder": 1}

    hostname, cleanup = multiva_setup(client, guestlist)

    failed_authzrs = []
    try:
        chisel2.auth_and_issue([hostname], client=client, chall_type="http-01")
    except acme_errors.ValidationError as e:
        # NOTE(@cpu): Chisel2's expect_problem doesn't work in this case so this
        # test needs to unpack an `acme_errors.ValidationError` on its own. It
        # might be possible to clean this up in the future.
        failed_authzrs = e.failed_authzrs
    finally:
        cleanup()
    if len(failed_authzrs) != 1:
        raise(Exception("expected one failed authz, found {0}".format(len(failed_authzrs))))
    challs = failed_authzrs[0].body.challenges
    httpChall = None
    for chall_body in challs:
        if isinstance(chall_body.chall, challenges.HTTP01):
            httpChall = chall_body
    if httpChall is None:
        raise(Exception("no HTTP-01 challenge in failed authz"))
    if httpChall.error.typ != "urn:ietf:params:acme:error:unauthorized":
        raise(Exception("expected unauthorized prob, found {0}".format(httpChall.error.typ)))
    if not httpChall.error.detail.startswith("During secondary validation: "):
        raise(Exception("expected 'During secondary validation' problem detail, found {0}".format(httpChall.error.detail)))

class FakeH2ServerHandler(socketserver.BaseRequestHandler):
    """
    FakeH2ServerHandler is a TCP socket handler that writes data representing an
    initial HTTP/2 SETTINGS frame as a response to all received data.
    """
    def handle(self):
        # Read whatever the HTTP request was so that the response isn't seen as
        # unsolicited.
        self.data = self.request.recv(1024).strip()
        # Blast some HTTP/2 bytes onto the socket
        # Truncated example data from taken from the community forum:
        # https://community.letsencrypt.org/t/le-validation-error-if-server-is-in-google-infrastructure/51841
        self.request.sendall(b"\x00\x00\x12\x04\x00\x00\x00\x00\x00\x00\x03\x00\x00\x00\x80\x00")

def wait_for_tcp_server(addr, port):
    """
    wait_for_tcp_server attempts to make a TCP connection to the given
    address/port every 0.5s until it succeeds.
    """
    while True:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        try:
            sock.connect((addr, port))
            sock.sendall(b"\n")
            return
        except socket.error:
            time.sleep(0.5)
            pass

def test_http2_http01_challenge():
    """
    test_http2_http01_challenge tests that an HTTP-01 challenge made to a HTTP/2
    server fails with a specific error message for this case.
    """
    client = chisel2.make_client()
    hostname = "fake.h2.example.com"

    # Add an A record for the test server to ensure the VA's requests are directed
    # to the interface that we bind the FakeH2ServerHandler to.
    challSrv.add_a_record(hostname, ["64.112.117.134"])

    # Allow socket address reuse on the base TCPServer class. Failing to do this
    # causes subsequent integration tests to fail with "Address in use" errors even
    # though this test _does_ call shutdown() and server_close(). Even though the
    # server was shut-down Python's socket will be in TIME_WAIT because of prev. client
    # connections. Having the TCPServer set SO_REUSEADDR on the socket solves
    # the problem.
    socketserver.TCPServer.allow_reuse_address = True
    # Create, start, and wait for a fake HTTP/2 server.
    server = socketserver.TCPServer(("64.112.117.134", 80), FakeH2ServerHandler)
    thread = threading.Thread(target = server.serve_forever)
    thread.daemon = False
    thread.start()
    wait_for_tcp_server("64.112.117.134", 80)

    # Issuing an HTTP-01 challenge for this hostname should produce a connection
    # problem with an error specific to the HTTP/2 misconfiguration.
    expectedError = "Server is speaking HTTP/2 over HTTP"
    try:
        chisel2.auth_and_issue([hostname], client=client, chall_type="http-01")
    except acme_errors.ValidationError as e:
        for authzr in e.failed_authzrs:
            c = chisel2.get_chall(authzr, challenges.HTTP01)
            error = c.error
            if error is None or error.typ != "urn:ietf:params:acme:error:connection":
                raise(Exception("Expected connection prob, got %s" % (error.__str__())))
            if not error.detail.endswith(expectedError):
                raise(Exception("Expected prob detail ending in %s, got %s" % (expectedError, error.detail)))
    finally:
        server.shutdown()
        server.server_close()
        thread.join()

def test_new_order_policy_errs():
    """
    Test that creating an order with policy blocked identifiers returns
    a problem with subproblems.
    """
    client = chisel2.make_client(None)

    # 'in-addr.arpa' is present in `test/hostname-policy.yaml`'s
    # HighRiskBlockedNames list.
    csr_pem = chisel2.make_csr(["out-addr.in-addr.arpa", "between-addr.in-addr.arpa"])

    # With two policy blocked names in the order we expect to get back a top
    # level rejectedIdentifier with a detail message that references
    # subproblems.
    #
    # TODO(@cpu): After https://github.com/certbot/certbot/issues/7046 is
    # implemented in the upstream `acme` module this test should also ensure the
    # subproblems are properly represented.
    ok = False
    try:
        order = client.new_order(csr_pem)
    except messages.Error as e:
        ok = True
        if e.typ != "urn:ietf:params:acme:error:rejectedIdentifier":
            raise(Exception("Expected rejectedIdentifier type problem, got {0}".format(e.typ)))
        if e.detail != 'Error creating new order :: Cannot issue for "between-addr.in-addr.arpa": The ACME server refuses to issue a certificate for this domain name, because it is forbidden by policy (and 1 more problems. Refer to sub-problems for more information.)':
            raise(Exception("Order problem detail did not match expected"))
    if not ok:
        raise(Exception("Expected problem, got no error"))

def test_delete_unused_challenges():
    order = chisel2.auth_and_issue([random_domain()], chall_type="dns-01")
    a = order.authorizations[0]
    if len(a.body.challenges) != 1:
        raise(Exception("too many challenges (%d) left after validation" % len(a.body.challenges)))
    if not isinstance(a.body.challenges[0].chall, challenges.DNS01):
        raise(Exception("wrong challenge type left after validation"))

    # intentionally fail a challenge
    client = chisel2.make_client()
    csr_pem = chisel2.make_csr([random_domain()])
    order = client.new_order(csr_pem)
    c = chisel2.get_chall(order.authorizations[0], challenges.DNS01)
    client.answer_challenge(c, c.response(client.net.key))
    for _ in range(5):
        a, _ = client.poll(order.authorizations[0])
        if a.body.status == Status("invalid"):
            break
        time.sleep(1)
    if len(a.body.challenges) != 1:
        raise(Exception("too many challenges (%d) left after failed validation" %
            len(a.body.challenges)))
    if not isinstance(a.body.challenges[0].chall, challenges.DNS01):
        raise(Exception("wrong challenge type left after validation"))

def test_auth_deactivation_v2():
    client = chisel2.make_client(None)
    csr_pem = chisel2.make_csr([random_domain()])
    order = client.new_order(csr_pem)
    resp = client.deactivate_authorization(order.authorizations[0])
    if resp.body.status is not messages.STATUS_DEACTIVATED:
        raise(Exception("unexpected authorization status"))

    order = chisel2.auth_and_issue([random_domain()], client=client)
    resp = client.deactivate_authorization(order.authorizations[0])
    if resp.body.status is not messages.STATUS_DEACTIVATED:
        raise(Exception("unexpected authorization status"))

def test_ct_submission():
    hostname = random_domain()

    chisel2.auth_and_issue([hostname])

    # These should correspond to the configured logs in ra.json.
    log_groups = [
        ["http://boulder.service.consul:4600/submissions", "http://boulder.service.consul:4601/submissions", "http://boulder.service.consul:4602/submissions", "http://boulder.service.consul:4603/submissions"],
        ["http://boulder.service.consul:4604/submissions", "http://boulder.service.consul:4605/submissions"],
        ["http://boulder.service.consul:4606/submissions"],
        ["http://boulder.service.consul:4607/submissions"],
        ["http://boulder.service.consul:4608/submissions"],
        ["http://boulder.service.consul:4609/submissions"],
    ]

    # These should correspond to the logs with `submitFinal` in ra.json.
    final_logs = [
        "http://boulder.service.consul:4600/submissions",
        "http://boulder.service.consul:4601/submissions",
        "http://boulder.service.consul:4606/submissions",
        "http://boulder.service.consul:4609/submissions",
     ]

    # We'd like to enforce strict limits here (exactly 1 submission per group,
    # exactly two submissions overall) but the async nature of the race system
    # means we can't -- a slowish submission to one log in a group could trigger
    # a very fast submission to a different log in the same group, and then both
    # submissions could succeed at the same time. Although the Go code will only
    # use one of the SCTs, both logs will still have been submitted to, and it
    # will show up here.
    total_count = 0
    for i in range(len(log_groups)):
        group_count = 0
        for j in range(len(log_groups[i])):
            log = log_groups[i][j]
            count = int(requests.get(log + "?hostnames=%s" % hostname).text)
            threshold = 1
            if log in final_logs:
                threshold += 1
            if count > threshold:
                raise(Exception("Got %d submissions for log %s, expected at most %d" % (count, log, threshold)))
            group_count += count
        total_count += group_count
    if total_count < 2:
        raise(Exception("Got %d total submissions, expected at least 2" % total_count))

def check_ocsp_basic_oid(cert_file, issuer_file, url):
    """
    This function checks if an OCSP response was successful, but doesn't verify
    the signature or timestamp. This is useful when simulating the past, so we
    don't incorrectly reject a response for being in the past.
    """
    ocsp_request = make_ocsp_req(cert_file, issuer_file)
    responses = fetch_ocsp(ocsp_request, url)
    # An unauthorized response (for instance, if the OCSP responder doesn't know
    # about this cert) will just be 30 03 0A 01 06. A "good" or "revoked"
    # response will contain, among other things, the id-pkix-ocsp-basic OID
    # identifying the response type. We look for that OID to confirm we got a
    # successful response.
    expected = bytearray.fromhex("06 09 2B 06 01 05 05 07 30 01 01")
    for resp in responses:
        if not expected in bytearray(resp):
            raise(Exception("Did not receive successful OCSP response: %s doesn't contain %s" %
                (base64.b64encode(resp), base64.b64encode(expected))))

ocsp_exp_unauth_setup_data = {}
@register_six_months_ago
def ocsp_exp_unauth_setup():
    client = chisel2.make_client(None)
    cert_file = temppath('ocsp_exp_unauth_setup.pem')
    chisel2.auth_and_issue([random_domain()], client=client, cert_output=cert_file.name)

    # Since our servers are pretending to be in the past, but the openssl cli
    # isn't, we'll get an expired OCSP response. Just check that it exists;
    # don't do the full verification (which would fail).
    lastException = None
    for issuer_file in glob.glob("test/certs/webpki/int-rsa-*.cert.pem"):
        try:
            check_ocsp_basic_oid(cert_file.name, issuer_file, "http://localhost:4002")
            global ocsp_exp_unauth_setup_data
            ocsp_exp_unauth_setup_data['cert_file'] = cert_file.name
            return
        except Exception as e:
            lastException = e
            continue
    raise(lastException)

def test_ocsp_exp_unauth():
    tries = 0
    if 'cert_file' not in ocsp_exp_unauth_setup_data:
        raise Exception("ocsp_exp_unauth_setup didn't run")
    cert_file = ocsp_exp_unauth_setup_data['cert_file']
    last_error = ""
    while tries < 5:
        try:
            verify_ocsp(cert_file, "test/certs/webpki/int-rsa-*.cert.pem", "http://localhost:4002", "XXX")
            raise(Exception("Unexpected return from verify_ocsp"))
        except subprocess.CalledProcessError as cpe:
            last_error = cpe.output
            if cpe.output == b"Responder Error: unauthorized (6)\n":
                break
        except e:
            last_error = e
            pass
        tries += 1
        time.sleep(0.25)
    else:
        raise(Exception("timed out waiting for unauthorized OCSP response for expired certificate. Last error: {}".format(last_error)))

def test_caa_good():
    domain = random_domain()
    challSrv.add_caa_issue(domain, "happy-hacker-ca.invalid")
    chisel2.auth_and_issue([domain])

def test_caa_reject():
    domain = random_domain()
    challSrv.add_caa_issue(domain, "sad-hacker-ca.invalid")
    chisel2.expect_problem("urn:ietf:params:acme:error:caa",
        lambda: chisel2.auth_and_issue([domain]))

def test_caa_extensions():
    goodCAA = "happy-hacker-ca.invalid"

    client = chisel2.make_client()
    caa_account_uri = client.net.account.uri
    caa_records = [
        {"domain": "accounturi.good-caa-reserved.com", "value":"{0}; accounturi={1}".format(goodCAA, caa_account_uri)},
        {"domain": "dns-01-only.good-caa-reserved.com", "value": "{0}; validationmethods=dns-01".format(goodCAA)},
        {"domain": "http-01-only.good-caa-reserved.com", "value": "{0}; validationmethods=http-01".format(goodCAA)},
        {"domain": "dns-01-or-http01.good-caa-reserved.com", "value": "{0}; validationmethods=dns-01,http-01".format(goodCAA)},
    ]
    for policy in caa_records:
        challSrv.add_caa_issue(policy["domain"], policy["value"])

    chisel2.expect_problem("urn:ietf:params:acme:error:caa",
        lambda: chisel2.auth_and_issue(["dns-01-only.good-caa-reserved.com"], chall_type="http-01"))

    chisel2.expect_problem("urn:ietf:params:acme:error:caa",
        lambda: chisel2.auth_and_issue(["http-01-only.good-caa-reserved.com"], chall_type="dns-01"))

    ## Note: the additional names are to avoid rate limiting...
    chisel2.auth_and_issue(["dns-01-only.good-caa-reserved.com", "www.dns-01-only.good-caa-reserved.com"], chall_type="dns-01")
    chisel2.auth_and_issue(["http-01-only.good-caa-reserved.com", "www.http-01-only.good-caa-reserved.com"], chall_type="http-01")
    chisel2.auth_and_issue(["dns-01-or-http-01.good-caa-reserved.com", "dns-01-only.good-caa-reserved.com"], chall_type="dns-01")
    chisel2.auth_and_issue(["dns-01-or-http-01.good-caa-reserved.com", "http-01-only.good-caa-reserved.com"], chall_type="http-01")

    ## CAA should fail with an arbitrary account, but succeed with the CAA client.
    chisel2.expect_problem("urn:ietf:params:acme:error:caa", lambda: chisel2.auth_and_issue(["accounturi.good-caa-reserved.com"]))
    chisel2.auth_and_issue(["accounturi.good-caa-reserved.com"], client=client)

def test_renewal_exemption():
    """
    Under a single domain, issue two certificates for different subdomains of
    the same name, then renewals of each of them. Since the certificatesPerName
    rate limit in testing is 2 per 90 days, and the renewals should not be
    counted under the renewal exemption, each of these issuances should succeed.
    Then do one last issuance (for a third subdomain of the same name) that we
    expect to be rate limited, just to check that the rate limit is actually 2,
    and we are testing what we think we are testing. See
    https://letsencrypt.org/docs/rate-limits/ for more details.
    """
    base_domain = random_domain()
    # First issuance
    chisel2.auth_and_issue(["www." + base_domain])
    # First Renewal
    chisel2.auth_and_issue(["www." + base_domain])
    # Issuance of a different cert
    chisel2.auth_and_issue(["blog." + base_domain])
    # Renew that one
    chisel2.auth_and_issue(["blog." + base_domain])
    # Final, failed issuance, for another different cert
    chisel2.expect_problem("urn:ietf:params:acme:error:rateLimited",
        lambda: chisel2.auth_and_issue(["mail." + base_domain]))

def test_oversized_csr():
    # Number of names is chosen to be one greater than the configured RA/CA maxNames
    numNames = 101
    # Generate numNames subdomains of a random domain
    base_domain = random_domain()
    domains = [ "{0}.{1}".format(str(n),base_domain) for n in range(numNames) ]
    # We expect issuing for these domains to produce a malformed error because
    # there are too many names in the request.
    chisel2.expect_problem("urn:ietf:params:acme:error:malformed",
            lambda: chisel2.auth_and_issue(domains))

def parse_cert(order):
    return x509.load_pem_x509_certificate(order.fullchain_pem.encode(), default_backend())

def test_sct_embedding():
    order = chisel2.auth_and_issue([random_domain()])
    cert = parse_cert(order)

    # make sure there is no poison extension
    try:
        cert.extensions.get_extension_for_oid(x509.ObjectIdentifier("1.3.6.1.4.1.11129.2.4.3"))
        raise(Exception("certificate contains CT poison extension"))
    except x509.ExtensionNotFound:
        # do nothing
        pass

    # make sure there is a SCT list extension
    try:
        sctList = cert.extensions.get_extension_for_oid(x509.ObjectIdentifier("1.3.6.1.4.1.11129.2.4.2"))
    except x509.ExtensionNotFound:
        raise(Exception("certificate doesn't contain SCT list extension"))
    if len(sctList.value) != 2:
        raise(Exception("SCT list contains wrong number of SCTs"))
    for sct in sctList.value:
        if sct.version != x509.certificate_transparency.Version.v1:
            raise(Exception("SCT contains wrong version"))
        if sct.entry_type != x509.certificate_transparency.LogEntryType.PRE_CERTIFICATE:
            raise(Exception("SCT contains wrong entry type"))
        delta = sct.timestamp - datetime.datetime.now()
        if abs(delta) > datetime.timedelta(hours=1):
            raise(Exception("Delta between SCT timestamp and now was too great "
                "%s vs %s (%s)" % (sct.timestamp, datetime.datetime.now(), delta)))

def test_auth_deactivation():
    client = chisel2.make_client(None)
    d = random_domain()
    csr_pem = chisel2.make_csr([d])
    order = client.new_order(csr_pem)

    resp = client.deactivate_authorization(order.authorizations[0])
    if resp.body.status is not messages.STATUS_DEACTIVATED:
        raise Exception("unexpected authorization status")

    order = chisel2.auth_and_issue([random_domain()], client=client)
    resp = client.deactivate_authorization(order.authorizations[0])
    if resp.body.status is not messages.STATUS_DEACTIVATED:
        raise Exception("unexpected authorization status")

def get_ocsp_response_and_reason(cert_file, issuer_glob, url):
    """Returns the ocsp response output and revocation reason."""
    output = verify_ocsp(cert_file, issuer_glob, url, None)
    m = re.search(r'Reason: (\w+)', output)
    reason = m.group(1) if m is not None else ""
    return output, reason

ocsp_resigning_setup_data = {}
@register_twenty_days_ago
def ocsp_resigning_setup():
    """Issue and then revoke a cert in the past.

    Useful setup for test_ocsp_resigning, which needs to check that the
    revocation reason is still correctly set after re-signing and old OCSP
    response.
    """
    client = chisel2.make_client(None)
    cert_file = temppath('ocsp_resigning_setup.pem')
    order = chisel2.auth_and_issue([random_domain()], client=client, cert_output=cert_file.name)

    cert = x509.load_pem_x509_certificate(order.fullchain_pem.encode(), default_backend())
    # Revoke for reason 5: cessationOfOperation
    client.revoke(cert, 5)

    ocsp_response, reason = get_ocsp_response_and_reason(
        cert_file.name, "test/certs/webpki/int-rsa-*.cert.pem", "http://localhost:4002")
    global ocsp_resigning_setup_data
    ocsp_resigning_setup_data = {
        'cert_file': cert_file.name,
        'response': ocsp_response,
        'reason': reason
    }

def test_ocsp_resigning():
    """Check that, after re-signing an OCSP, the reason is still set."""
    if 'response' not in ocsp_resigning_setup_data:
        raise Exception("ocsp_resigning_setup didn't run")

    tries = 0
    while tries < 5:
        resp, reason = get_ocsp_response_and_reason(
            ocsp_resigning_setup_data['cert_file'], "test/certs/webpki/int-rsa-*.cert.pem", "http://localhost:4002")
        if resp != ocsp_resigning_setup_data['response']:
            break
        tries += 1
        time.sleep(0.25)
    else:
        raise(Exception("timed out waiting for re-signed OCSP response for certificate"))

    if reason != ocsp_resigning_setup_data['reason']:
        raise(Exception("re-signed ocsp response has different reason %s expected %s" % (
            reason, ocsp_resigning_setup_data['reason'])))
    if reason != "cessationOfOperation":
        raise(Exception("re-signed ocsp response has wrong reason %s" % reason))
