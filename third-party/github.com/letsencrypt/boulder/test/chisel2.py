"""
A simple client that uses the Python ACME library to run a test issuance against
a local Boulder server.
Usage:

$ virtualenv venv
$ . venv/bin/activate
$ pip install -r requirements.txt
$ python chisel2.py foo.com bar.com
"""
import json
import logging
import os
import sys
import signal
import threading
import time

from cryptography.hazmat.backends import default_backend
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography import x509
from cryptography.hazmat.primitives import hashes

import OpenSSL
import josepy

from acme import challenges
from acme import client as acme_client
from acme import crypto_util as acme_crypto_util
from acme import errors as acme_errors
from acme import messages
from acme import standalone

logging.basicConfig()
logger = logging.getLogger()
logger.setLevel(int(os.getenv('LOGLEVEL', 20)))

DIRECTORY_V2 = os.getenv('DIRECTORY_V2', 'http://boulder.service.consul:4001/directory')
ACCEPTABLE_TOS = os.getenv('ACCEPTABLE_TOS',"https://boulder.service.consul:4431/terms/v7")
PORT = os.getenv('PORT', '80')

os.environ.setdefault('REQUESTS_CA_BUNDLE', 'test/certs/ipki/minica.pem')

import challtestsrv
challSrv = challtestsrv.ChallTestServer()

def uninitialized_client(key=None):
    if key is None:
        key = josepy.JWKRSA(key=rsa.generate_private_key(65537, 2048, default_backend()))
    net = acme_client.ClientNetwork(key, user_agent="Boulder integration tester")
    directory = messages.Directory.from_json(net.get(DIRECTORY_V2).json())
    return acme_client.ClientV2(directory, net)

def make_client(email=None):
    """Build an acme.Client and register a new account with a random key."""
    client = uninitialized_client()
    tos = client.directory.meta.terms_of_service
    if tos == ACCEPTABLE_TOS:
        client.net.account = client.new_account(messages.NewRegistration.from_data(email=email,
            terms_of_service_agreed=True))
    else:
        raise Exception("Unrecognized terms of service URL %s" % tos)
    return client

class NoClientError(ValueError):
    """
    An error that occurs when no acme.Client is provided to a function that
    requires one.
    """
    pass

class EmailRequiredError(ValueError):
    """
    An error that occurs when a None email is provided to update_email.
    """

def update_email(client, email):
    """
    Use a provided acme.Client to update the client's account to the specified
    email.
    """
    if client is None:
        raise(NoClientError("update_email requires a valid acme.Client argument"))
    if email is None:
        raise(EmailRequiredError("update_email requires an email argument"))
    if not email.startswith("mailto:"):
        email = "mailto:"+ email
    acct = client.net.account
    updatedAcct = acct.update(body=acct.body.update(contact=(email,)))
    return client.update_registration(updatedAcct)


def get_chall(authz, typ):
    for chall_body in authz.body.challenges:
        if isinstance(chall_body.chall, typ):
            return chall_body
    raise Exception("No %s challenge found" % typ.typ)

def make_csr(domains):
    key = OpenSSL.crypto.PKey()
    key.generate_key(OpenSSL.crypto.TYPE_RSA, 2048)
    pem = OpenSSL.crypto.dump_privatekey(OpenSSL.crypto.FILETYPE_PEM, key)
    return acme_crypto_util.make_csr(pem, domains, False)

def http_01_answer(client, chall_body):
    """Return an HTTP01Resource to server in response to the given challenge."""
    response, validation = chall_body.response_and_validation(client.net.key)
    return standalone.HTTP01RequestHandler.HTTP01Resource(
          chall=chall_body.chall, response=response,
          validation=validation)

def auth_and_issue(domains, chall_type="dns-01", email=None, cert_output=None, client=None):
    """Make authzs for each of the given domains, set up a server to answer the
       challenges in those authzs, tell the ACME server to validate the challenges,
       then poll for the authzs to be ready and issue a cert."""
    if client is None:
        client = make_client(email)

    csr_pem = make_csr(domains)
    order = client.new_order(csr_pem)
    authzs = order.authorizations

    if chall_type == "http-01":
        cleanup = do_http_challenges(client, authzs)
    elif chall_type == "dns-01":
        cleanup = do_dns_challenges(client, authzs)
    elif chall_type == "tls-alpn-01":
        cleanup = do_tlsalpn_challenges(client, authzs)
    else:
        raise Exception("invalid challenge type %s" % chall_type)

    try:
        order = client.poll_and_finalize(order)
        if cert_output is not None:
            with open(cert_output, "w") as f:
                f.write(order.fullchain_pem)
    finally:
        cleanup()

    return order

def do_dns_challenges(client, authzs):
    cleanup_hosts = []
    for a in authzs:
        c = get_chall(a, challenges.DNS01)
        name, value = (c.validation_domain_name(a.body.identifier.value),
            c.validation(client.net.key))
        cleanup_hosts.append(name)
        challSrv.add_dns01_response(name, value)
        client.answer_challenge(c, c.response(client.net.key))
    def cleanup():
        for host in cleanup_hosts:
            challSrv.remove_dns01_response(host)
    return cleanup

def do_http_challenges(client, authzs):
    cleanup_tokens = []
    challs = [get_chall(a, challenges.HTTP01) for a in authzs]

    for chall_body in challs:
        # Determine the token and key auth for the challenge
        token = chall_body.chall.encode("token")
        resp = chall_body.response(client.net.key)
        keyauth = resp.key_authorization

        # Add the HTTP-01 challenge response for this token/key auth to the
        # challtestsrv
        challSrv.add_http01_response(token, keyauth)
        cleanup_tokens.append(token)

        # Then proceed initiating the challenges with the ACME server
        client.answer_challenge(chall_body, chall_body.response(client.net.key))

    def cleanup():
        # Cleanup requires removing each of the HTTP-01 challenge responses for
        # the tokens we added.
        for token in cleanup_tokens:
            challSrv.remove_http01_response(token)
    return cleanup

def do_tlsalpn_challenges(client, authzs):
    cleanup_hosts = []
    for a in authzs:
        c = get_chall(a, challenges.TLSALPN01)
        name, value = (a.body.identifier.value, c.key_authorization(client.net.key))
        cleanup_hosts.append(name)
        challSrv.add_tlsalpn01_response(name, value)
        client.answer_challenge(c, c.response(client.net.key))
    def cleanup():
        for host in cleanup_hosts:
            challSrv.remove_tlsalpn01_response(host)
    return cleanup

def expect_problem(problem_type, func):
    """Run a function. If it raises an acme_errors.ValidationError or messages.Error that
       contains the given problem_type, return. If it raises no error or the wrong
       error, raise an exception."""
    ok = False
    try:
        func()
    except messages.Error as e:
        if e.typ == problem_type:
            ok = True
        else:
            raise Exception("Expected %s, got %s" % (problem_type, e.__str__()))
    except acme_errors.ValidationError as e:
        for authzr in e.failed_authzrs:
            for chall in authzr.body.challenges:
                error = chall.error
                if error and error.typ == problem_type:
                    ok = True
                elif error:
                    raise Exception("Expected %s, got %s" % (problem_type, error.__str__()))
    if not ok:
        raise Exception('Expected %s, got no error' % problem_type)

if __name__ == "__main__":
    # Die on SIGINT
    signal.signal(signal.SIGINT, signal.SIG_DFL)
    domains = sys.argv[1:]
    if len(domains) == 0:
        print(__doc__)
        sys.exit(0)
    try:
        auth_and_issue(domains)
    except messages.Error as e:
        print(e)
        sys.exit(1)
