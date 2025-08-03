import atexit
import base64
import errno
import glob
import os
import random
import re
import requests
import shutil
import socket
import subprocess
import tempfile
import time
import urllib

import challtestsrv

challSrv = challtestsrv.ChallTestServer()
tempdir = tempfile.mkdtemp()

@atexit.register
def stop():
    shutil.rmtree(tempdir)

config_dir = os.environ.get('BOULDER_CONFIG_DIR', '')
if config_dir == '':
    raise Exception("BOULDER_CONFIG_DIR was not set")
CONFIG_NEXT = config_dir.startswith("test/config-next")

def temppath(name):
    """Creates and returns a closed file inside the tempdir."""
    f = tempfile.NamedTemporaryFile(
        dir=tempdir,
        suffix='.{0}'.format(name),
        mode='w+',
        delete=False
    )
    f.close()
    return f

def fakeclock(date):
    return date.strftime("%a %b %d %H:%M:%S UTC %Y")

def get_future_output(cmd, date):
    return subprocess.check_output(cmd, stderr=subprocess.STDOUT,
        env={'FAKECLOCK': fakeclock(date)}).decode()

def random_domain():
    """Generate a random domain for testing (to avoid rate limiting)."""
    return "rand.%x.xyz" % random.randrange(2**32)

def run(cmd, **kwargs):
    return subprocess.check_call(cmd, stderr=subprocess.STDOUT, **kwargs)

def fetch_ocsp(request_bytes, url):
    """Fetch an OCSP response using POST, GET, and GET with URL encoding.

    Returns a tuple of the responses.
    """
    ocsp_req_b64 = base64.b64encode(request_bytes).decode()

    # Make the OCSP request three different ways: by POST, by GET, and by GET with
    # URL-encoded parameters. All three should have an identical response.
    get_response = requests.get("%s/%s" % (url, ocsp_req_b64)).content
    get_encoded_response = requests.get("%s/%s" % (url, urllib.parse.quote(ocsp_req_b64, safe = ""))).content
    post_response = requests.post("%s/" % (url), data=request_bytes).content

    return (post_response, get_response, get_encoded_response)

def make_ocsp_req(cert_file, issuer_file):
    """Return the bytes of an OCSP request for the given certificate file."""
    with tempfile.NamedTemporaryFile(dir=tempdir) as f:
        run(["openssl", "ocsp", "-no_nonce",
            "-issuer", issuer_file,
            "-cert", cert_file,
            "-reqout", f.name])
        ocsp_req = f.read()
    return ocsp_req

def ocsp_verify(cert_file, issuer_file, ocsp_response):
    with tempfile.NamedTemporaryFile(dir=tempdir, delete=False) as f:
        f.write(ocsp_response)
        f.close()
        output = subprocess.check_output([
            'openssl', 'ocsp', '-no_nonce',
            '-issuer', issuer_file,
            '-cert', cert_file,
            '-verify_other', issuer_file,
            '-CAfile', 'test/certs/webpki/root-rsa.cert.pem',
            '-respin', f.name], stderr=subprocess.STDOUT).decode()
    # OpenSSL doesn't always return non-zero when response verify fails, so we
    # also look for the string "Response Verify Failure"
    verify_failure = "Response Verify Failure"
    if re.search(verify_failure, output):
        print(output)
        raise(Exception("OCSP verify failure"))
    return output

def verify_ocsp(cert_file, issuer_glob, url, status="revoked", reason=None):
    # Try to verify the OCSP response using every issuer identified by the glob.
    # If one works, great. If none work, re-raise the exception produced by the
    # last attempt
    lastException = None
    for issuer_file in glob.glob(issuer_glob):
        try:
            output = try_verify_ocsp(cert_file, issuer_file, url, status, reason)
            return output
        except Exception as e:
            lastException = e
            continue
    raise(lastException)

def try_verify_ocsp(cert_file, issuer_file, url, status="revoked", reason=None):
    ocsp_request = make_ocsp_req(cert_file, issuer_file)
    responses = fetch_ocsp(ocsp_request, url)

    # Verify all responses are the same
    for resp in responses:
        if resp != responses[0]:
            raise(Exception("OCSP responses differed: %s vs %s" %(
                base64.b64encode(responses[0]), base64.b64encode(resp))))

    # Check response is for the correct certificate and is correct
    # status
    resp = responses[0]
    verify_output = ocsp_verify(cert_file, issuer_file, resp)
    if status is not None:
        if not re.search("%s: %s" % (cert_file, status), verify_output):
            print(verify_output)
            raise(Exception("OCSP response wasn't '%s'" % status))
    if reason == "unspecified":
        if re.search("Reason:", verify_output):
            print(verify_output)
            raise(Exception("OCSP response contained unexpected reason"))
    elif reason is not None:
        if not re.search("Reason: %s" % reason, verify_output):
            print(verify_output)
            raise(Exception("OCSP response wasn't '%s'" % reason))
    return verify_output

def reset_akamai_purges():
    requests.post("http://localhost:6789/debug/reset-purges", data="{}")

def verify_akamai_purge():
    deadline = time.time() + .4
    while True:
        time.sleep(0.05)
        if time.time() > deadline:
            raise(Exception("Timed out waiting for Akamai purge"))
        response = requests.get("http://localhost:6789/debug/get-purges")
        purgeData = response.json()
        if len(purgeData["V3"]) == 0:
            continue
        break
    reset_akamai_purges()

twenty_days_ago_functions = [ ]

def register_twenty_days_ago(f):
    """Register a function to be run during "setup_twenty_days_ago." This allows
       test cases to define their own custom setup.
    """
    twenty_days_ago_functions.append(f)

def setup_twenty_days_ago():
    """Do any setup that needs to happen 20 day in the past, for tests that
       will run in the 'present'.
    """
    for f in twenty_days_ago_functions:
        f()

six_months_ago_functions = []

def register_six_months_ago(f):
    six_months_ago_functions.append(f)

def setup_six_months_ago():
    [f() for f in six_months_ago_functions]

def waitport(port, prog, perTickCheck=None):
    """Wait until a port on localhost is open."""
    for _ in range(1000):
        try:
            time.sleep(0.1)
            if perTickCheck is not None and not perTickCheck():
                return False
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            s.connect(('localhost', port))
            s.close()
            return True
        except socket.error as e:
            if e.errno == errno.ECONNREFUSED:
                print("Waiting for debug port %d (%s)" % (port, prog))
            else:
                raise
    raise(Exception("timed out waiting for debug port %d (%s)" % (port, prog)))

def waithealth(prog, port, host_override):
    subprocess.check_call([
        './bin/health-checker',
        '-addr', ("localhost:%d" % (port)),
        '-host-override', host_override,
        '-config', os.path.join(config_dir, 'health-checker.json')])
