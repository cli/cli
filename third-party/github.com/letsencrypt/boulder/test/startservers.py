import atexit
import collections
import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time

from helpers import waithealth, waitport, config_dir, CONFIG_NEXT

Service = collections.namedtuple('Service', ('name', 'debug_port', 'grpc_port', 'host_override', 'cmd', 'deps'))

# Keep these ports in sync with consul/config.hcl
SERVICES = (
    Service('remoteva-a',
        8011, 9397, 'rva.boulder',
        ('./bin/boulder', 'remoteva', '--config', os.path.join(config_dir, 'remoteva-a.json'), '--addr', ':9397', '--debug-addr', ':8011'),
        None),
    Service('remoteva-b',
        8012, 9498, 'rva.boulder',
        ('./bin/boulder', 'remoteva', '--config', os.path.join(config_dir, 'remoteva-b.json'), '--addr', ':9498', '--debug-addr', ':8012'),
        None),
    Service('remoteva-c',
        8023, 9499, 'rva.boulder',
        ('./bin/boulder', 'remoteva', '--config', os.path.join(config_dir, 'remoteva-c.json'), '--addr', ':9499', '--debug-addr', ':8023'),
        None),
    Service('boulder-sa-1',
        8003, 9395, 'sa.boulder',
        ('./bin/boulder', 'boulder-sa', '--config', os.path.join(config_dir, 'sa.json'), '--addr', ':9395', '--debug-addr', ':8003'),
        None),
    Service('boulder-sa-2',
        8103, 9495, 'sa.boulder',
        ('./bin/boulder', 'boulder-sa', '--config', os.path.join(config_dir, 'sa.json'), '--addr', ':9495', '--debug-addr', ':8103'),
        None),
    Service('aia-test-srv',
        4502, None, None,
        ('./bin/aia-test-srv', '--addr', ':4502', '--hierarchy', 'test/certs/webpki/'), None),
    Service('ct-test-srv',
        4600, None, None,
        ('./bin/ct-test-srv', '--config', 'test/ct-test-srv/ct-test-srv.json'), None),
    Service('boulder-publisher-1',
        8009, 9391, 'publisher.boulder',
        ('./bin/boulder', 'boulder-publisher', '--config', os.path.join(config_dir, 'publisher.json'), '--addr', ':9391', '--debug-addr', ':8009'),
        None),
    Service('boulder-publisher-2',
        8109, 9491, 'publisher.boulder',
        ('./bin/boulder', 'boulder-publisher', '--config', os.path.join(config_dir, 'publisher.json'), '--addr', ':9491', '--debug-addr', ':8109'),
        None),
    Service('mail-test-srv',
        9380, None, None,
        ('./bin/mail-test-srv', '--closeFirst', '5', '--cert', 'test/certs/ipki/localhost/cert.pem', '--key', 'test/certs/ipki/localhost/key.pem'),
        None),
    Service('ocsp-responder',
        8005, None, None,
        ('./bin/boulder', 'ocsp-responder', '--config', os.path.join(config_dir, 'ocsp-responder.json'), '--addr', ':4002', '--debug-addr', ':8005'),
        ('boulder-ra-1', 'boulder-ra-2')),
    Service('boulder-va-1',
        8004, 9392, 'va.boulder',
        ('./bin/boulder', 'boulder-va', '--config', os.path.join(config_dir, 'va.json'), '--addr', ':9392', '--debug-addr', ':8004'),
        ('remoteva-a', 'remoteva-b')),
    Service('boulder-va-2',
        8104, 9492, 'va.boulder',
        ('./bin/boulder', 'boulder-va', '--config', os.path.join(config_dir, 'va.json'), '--addr', ':9492', '--debug-addr', ':8104'),
        ('remoteva-a', 'remoteva-b')),
    Service('boulder-ca-1',
        8001, 9393, 'ca.boulder',
        ('./bin/boulder', 'boulder-ca', '--config', os.path.join(config_dir, 'ca.json'), '--addr', ':9393', '--debug-addr', ':8001'),
        ('boulder-sa-1', 'boulder-sa-2', 'boulder-ra-sct-provider-1', 'boulder-ra-sct-provider-2')),
    Service('boulder-ca-2',
        8101, 9493, 'ca.boulder',
        ('./bin/boulder', 'boulder-ca', '--config', os.path.join(config_dir, 'ca.json'), '--addr', ':9493', '--debug-addr', ':8101'),
        ('boulder-sa-1', 'boulder-sa-2', 'boulder-ra-sct-provider-1', 'boulder-ra-sct-provider-2')),
    Service('akamai-test-srv',
        6789, None, None,
        ('./bin/akamai-test-srv', '--listen', 'localhost:6789', '--secret', 'its-a-secret'),
        None),
    Service('akamai-purger',
        9666, None, None,
        ('./bin/boulder', 'akamai-purger', '--addr', ':9399', '--config', os.path.join(config_dir, 'akamai-purger.json'), '--debug-addr', ':9666'),
        ('akamai-test-srv',)),
    Service('s3-test-srv',
        4501, None, None,
        ('./bin/s3-test-srv', '--listen', ':4501'),
        None),
    Service('crl-storer',
        9667, None, None,
        ('./bin/boulder', 'crl-storer', '--config', os.path.join(config_dir, 'crl-storer.json'), '--addr', ':9309', '--debug-addr', ':9667'),
        ('s3-test-srv',)),
    Service('boulder-ra-1',
        8002, 9394, 'ra.boulder',
        ('./bin/boulder', 'boulder-ra', '--config', os.path.join(config_dir, 'ra.json'), '--addr', ':9394', '--debug-addr', ':8002'),
        ('boulder-sa-1', 'boulder-sa-2', 'boulder-ca-1', 'boulder-ca-2', 'boulder-va-1', 'boulder-va-2', 'akamai-purger', 'boulder-publisher-1', 'boulder-publisher-2')),
    Service('boulder-ra-2',
        8102, 9494, 'ra.boulder',
        ('./bin/boulder', 'boulder-ra', '--config', os.path.join(config_dir, 'ra.json'), '--addr', ':9494', '--debug-addr', ':8102'),
        ('boulder-sa-1', 'boulder-sa-2', 'boulder-ca-1', 'boulder-ca-2', 'boulder-va-1', 'boulder-va-2', 'akamai-purger', 'boulder-publisher-1', 'boulder-publisher-2')),
    # We run a separate instance of the RA for use as the SCTProvider service called by the CA.
    # This solves a small problem of startup order: if a client (the CA in this case) starts
    # up before its backends, gRPC will try to connect immediately (due to health checks),
    # get a connection refused, and enter a backoff state. That backoff state can cause
    # subsequent requests to fail. This issue only exists for the CA-RA pair because they
    # have a circular relationship - the RA calls CA.IssueCertificate, and the CA calls
    # SCTProvider.GetSCTs (offered by the RA).
    Service('boulder-ra-sct-provider-1',
        8118, 9594, 'ra.boulder',
        ('./bin/boulder', 'boulder-ra', '--config', os.path.join(config_dir, 'ra.json'), '--addr', ':9594', '--debug-addr', ':8118'),
        ('boulder-publisher-1', 'boulder-publisher-2')),
    Service('boulder-ra-sct-provider-2',
        8119, 9694, 'ra.boulder',
        ('./bin/boulder', 'boulder-ra', '--config', os.path.join(config_dir, 'ra.json'), '--addr', ':9694', '--debug-addr', ':8119'),
        ('boulder-publisher-1', 'boulder-publisher-2')),
    Service('bad-key-revoker',
        8020, None, None,
        ('./bin/boulder', 'bad-key-revoker', '--config', os.path.join(config_dir, 'bad-key-revoker.json'), '--debug-addr', ':8020'),
        ('boulder-ra-1', 'boulder-ra-2', 'mail-test-srv')),
    # Note: the nonce-service instances bind to specific ports, not "all interfaces",
    # because they use their explicitly bound port in calculating the nonce
    # prefix, which is used by WFEs when deciding where to redeem nonces.
    # The `taro` and `zinc` instances simulate nonce services in two different
    # datacenters. The WFE is configured to get nonces from one of these
    # services, and potentially redeeem from either service (though in practice
    # it will only redeem from the one that is configured for getting nonces).
    Service('nonce-service-taro-1',
        8111, None, None,
        ('./bin/boulder', 'nonce-service', '--config', os.path.join(config_dir, 'nonce-a.json'), '--addr', '10.77.77.77:9301', '--debug-addr', ':8111',),
        None),
    Service('nonce-service-taro-2',
        8113, None, None,
        ('./bin/boulder', 'nonce-service', '--config', os.path.join(config_dir, 'nonce-a.json'), '--addr', '10.77.77.77:9501', '--debug-addr', ':8113',),
        None),
    Service('nonce-service-zinc-1',
        8112, None, None,
        ('./bin/boulder', 'nonce-service', '--config', os.path.join(config_dir, 'nonce-b.json'), '--addr', '10.77.77.77:9401', '--debug-addr', ':8112',),
        None),
    Service('pardot-test-srv',
        # Uses port 9601 to mock Salesforce OAuth2 token API and 9602 to mock
        # the Pardot API. 
        9601, None, None,
        ('./bin/pardot-test-srv', '--config', os.path.join(config_dir, 'pardot-test-srv.json'),),
        None),
    Service('email-exporter',
        8114, None, None,
        ('./bin/boulder', 'email-exporter', '--config', os.path.join(config_dir, 'email-exporter.json'), '--addr', ':9603', '--debug-addr', ':8114'),
        ('pardot-test-srv',)),
    Service('boulder-wfe2',
        4001, None, None,
        ('./bin/boulder', 'boulder-wfe2', '--config', os.path.join(config_dir, 'wfe2.json'), '--addr', ':4001', '--tls-addr', ':4431', '--debug-addr', ':8013'),
        ('boulder-ra-1', 'boulder-ra-2', 'boulder-sa-1', 'boulder-sa-2', 'nonce-service-taro-1', 'nonce-service-taro-2', 'nonce-service-zinc-1', 'email-exporter')),
    Service('sfe',
        4003, None, None,
        ('./bin/boulder', 'sfe', '--config', os.path.join(config_dir, 'sfe.json'), '--addr', ':4003', '--debug-addr', ':8015'),
        ('boulder-ra-1', 'boulder-ra-2', 'boulder-sa-1', 'boulder-sa-2',)),
    Service('log-validator',
        8016, None, None,
        ('./bin/boulder', 'log-validator', '--config', os.path.join(config_dir, 'log-validator.json'), '--debug-addr', ':8016'),
        None),
)

def _service_toposort(services):
    """Yields Service objects in topologically sorted order.

    No service will be yielded until every service listed in its deps value
    has been yielded.
    """
    ready = set([s for s in services if not s.deps])
    blocked = set(services) - ready
    done = set()
    while ready:
        service = ready.pop()
        yield service
        done.add(service.name)
        new = set([s for s in blocked if all([d in done for d in s.deps])])
        ready |= new
        blocked -= new
    if blocked:
        print("WARNING: services with unsatisfied dependencies:")
        for s in blocked:
            print(s.name, ":", s.deps)
        raise(Exception("Unable to satisfy service dependencies"))

processes = []

# NOTE(@cpu): We manage the challSrvProcess separately from the other global
# processes because we want integration tests to be able to stop/start it (e.g.
# to run the load-generator).
challSrvProcess = None

def install(race_detection):
    # Pass empty BUILD_TIME and BUILD_ID flags to avoid constantly invalidating the
    # build cache with new BUILD_TIMEs, or invalidating it on merges with a new
    # BUILD_ID.
    go_build_flags='-tags "integration"'
    if race_detection:
        go_build_flags += ' -race'

    return subprocess.call(["/usr/bin/make", "GO_BUILD_FLAGS=%s" % go_build_flags]) == 0

def run(cmd, fakeclock):
    e = os.environ.copy()
    e.setdefault("GORACE", "halt_on_error=1")
    if fakeclock:
        e.setdefault("FAKECLOCK", fakeclock)
    p = subprocess.Popen(cmd, env=e)
    p.cmd = cmd
    return p

def start(fakeclock):
    """Return True if everything builds and starts.

    Give up and return False if anything fails to build, or dies at
    startup. Anything that did start before this point can be cleaned
    up explicitly by calling stop(), or automatically atexit.
    """
    signal.signal(signal.SIGTERM, lambda _, __: stop())
    signal.signal(signal.SIGINT, lambda _, __: stop())

    # Check that we can resolve the service names before we try to start any
    # services. This prevents a confusing error (timed out health check).
    try:
        socket.getaddrinfo('publisher.service.consul', None)
    except Exception as e:
        print("Error querying DNS. Is consul running? `docker compose ps bconsul`. %s" % (e))
        return False

    # Start the chall-test-srv first so it can be used to resolve DNS for
    # gRPC.
    startChallSrv()

    # Processes are in order of dependency: Each process should be started
    # before any services that intend to send it RPCs. On shutdown they will be
    # killed in reverse order.
    for service in _service_toposort(SERVICES):
        print("Starting service", service.name)
        try:
            global processes
            p = run(service.cmd, fakeclock)
            processes.append(p)
            if service.grpc_port is not None:
                waithealth(' '.join(p.args), service.grpc_port, service.host_override)
            else:
                if not waitport(service.debug_port, ' '.join(p.args), perTickCheck=check):
                    return False
        except Exception as e:
            print("Error starting service %s: %s" % (service.name, e))
            return False

    print("All servers running. Hit ^C to kill.")
    return True

def check():
    """Return true if all started processes are still alive.

    Log about anything that died. The chall-test-srv is not considered when
    checking processes.
    """
    global processes
    busted = []
    stillok = []
    for p in processes:
        if p.poll() is None:
            stillok.append(p)
        else:
            busted.append(p)
    if busted:
        print("\n\nThese processes exited early (check above for their output):")
        for p in busted:
            print("\t'%s' with pid %d exited %d" % (p.cmd, p.pid, p.returncode))
    processes = stillok
    return not busted

def startChallSrv():
    """
    Start the chall-test-srv and wait for it to become available. See also
    stopChallSrv.
    """
    global challSrvProcess
    if challSrvProcess is not None:
        raise(Exception("startChallSrv called more than once"))

    # NOTE(@cpu): We specify explicit bind addresses for -https01 and
    # --tlsalpn01 here to allow HTTPS HTTP-01 responses on 443 for on interface
    # and TLS-ALPN-01 responses on 443 for another interface. The choice of
    # which is used is controlled by mock DNS data added by the relevant
    # integration tests.
    challSrvProcess = run([
        './bin/chall-test-srv',
        '--defaultIPv4', os.environ.get("FAKE_DNS"),
        '-defaultIPv6', '',
        '--dns01', ':8053,:8054',
        '--doh', ':8343,:8443',
        '--doh-cert', 'test/certs/ipki/10.77.77.77/cert.pem',
        '--doh-cert-key', 'test/certs/ipki/10.77.77.77/key.pem',
        '--management', ':8055',
        '--http01', '64.112.117.122:80',
        '-https01', '64.112.117.122:443',
        '--tlsalpn01', '64.112.117.134:443'],
        None)
    # Wait for the chall-test-srv management port.
    if not waitport(8055, ' '.join(challSrvProcess.args)):
        return False

def stopChallSrv():
    """
    Stop the running chall-test-srv (if any) and wait for it to terminate.
    See also startChallSrv.
    """
    global challSrvProcess
    if challSrvProcess is None:
        return
    if challSrvProcess.poll() is None:
        challSrvProcess.send_signal(signal.SIGTERM)
        challSrvProcess.wait()
    challSrvProcess = None

@atexit.register
def stop():
    # When we are about to exit, send SIGTERM to each subprocess and wait for
    # them to nicely die. This reflects the restart process in prod and allows
    # us to exercise the graceful shutdown code paths.
    global processes
    for p in reversed(processes):
        if p.poll() is None:
            p.send_signal(signal.SIGTERM)
            p.wait()
    processes = []

    # Also stop the challenge test server
    stopChallSrv()
