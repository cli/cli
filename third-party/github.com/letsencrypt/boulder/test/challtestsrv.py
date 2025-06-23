import json
import requests

class ChallTestServer:
    """
    ChallTestServer is a wrapper around pebble-challtestsrv's HTTP management
    API. If the pebble-challtestsrv process you want to interact with is using
    a -management argument other than the default ('http://10.77.77.77:8055') you
    can instantiate the ChallTestServer using the -management address in use. If
    no custom address is provided the default is assumed.
    """
    _baseURL = "http://10.77.77.77:8055"

    _paths = {
            "set-ipv4": "/set-default-ipv4",
            "set-ipv6": "/set-default-ipv6",
            "del-history": "/clear-request-history",
            "get-http-history": "/http-request-history",
            "get-dns-history": "/dns-request-history",
            "get-alpn-history": "/tlsalpn01-request-history",
            "add-a": "/add-a",
            "del-a": "/clear-a",
            "add-aaaa": "/add-aaaa",
            "del-aaaa": "/clear-aaaa",
            "add-caa": "/add-caa",
            "del-caa": "/clear-caa",
            "add-redirect": "/add-redirect",
            "del-redirect": "/del-redirect",
            "add-http": "/add-http01",
            "del-http": "/del-http01",
            "add-txt": "/set-txt",
            "del-txt": "/clear-txt",
            "add-alpn": "/add-tlsalpn01",
            "del-alpn": "/del-tlsalpn01",
            "add-servfail": "/set-servfail",
            "del-servfail": "/clear-servfail",
            }

    def __init__(self, url=None):
        if url is not None:
            self._baseURL = url

    def _postURL(self, url, body):
        response = requests.post(
                url,
                data=json.dumps(body))
        return response.text

    def _URL(self, path):
        urlPath = self._paths.get(path, None)
        if urlPath is None:
            raise Exception("No challenge test server URL path known for {0}".format(path))
        return self._baseURL + urlPath

    def _clear_request_history(self, host, typ):
        return self._postURL(
                self._URL("del-history"),
                { "host": host, "type": typ })

    def set_default_ipv4(self, address):
        """
        set_default_ipv4 sets the challenge server's default IPv4 address used
        to respond to A queries when there are no specific mock A addresses for
        the hostname being queried. Provide an empty string as the default
        address to disable answering A queries except for hosts that have mock
        A addresses added.
        """
        return self._postURL(
                self._URL("set-ipv4"),
                { "ip": address })

    def set_default_ipv6(self, address):
        """
        set_default_ipv6 sets the challenge server's default IPv6 address used
        to respond to AAAA queries when there are no specific mock AAAA
        addresses for the hostname being queried. Provide an empty string as the
        default address to disable answering AAAA queries except for hosts that
        have mock AAAA addresses added.
        """
        return self._postURL(
                self._URL("set-ipv6"),
                { "ip": address })

    def add_a_record(self, host, addresses):
        """
        add_a_record adds a mock A response to the challenge server's DNS
        interface for the given host and IPv4 addresses.
        """
        return self._postURL(
                self._URL("add-a"),
                { "host": host, "addresses": addresses })

    def remove_a_record(self, host):
        """
        remove_a_record removes a mock A response from the challenge server's DNS
        interface for the given host.
        """
        return self._postURL(
                self._URL("del-a"),
                { "host": host })

    def add_aaaa_record(self, host, addresses):
        """
        add_aaaa_record adds a mock AAAA response to the challenge server's DNS
        interface for the given host and IPv6 addresses.
        """
        return self._postURL(
                self._URL("add-aaaa"),
                { "host": host, "addresses": addresses })

    def remove_aaaa_record(self, host):
        """
        remove_aaaa_record removes mock AAAA response from the challenge server's DNS
        interface for the given host.
        """
        return self._postURL(
                self._URL("del-aaaa"),
                { "host": host })

    def add_caa_issue(self, host, value):
        """
        add_caa_issue adds a mock CAA response to the challenge server's DNS
        interface. The mock CAA response will contain one policy with an "issue"
        tag specifying the provided value.
        """
        return self._postURL(
                self._URL("add-caa"),
                {
                    "host": host,
                    "policies": [{ "tag": "issue", "value": value}],
                })

    def remove_caa_issue(self, host):
        """
        remove_caa_issue removes a mock CAA response from the challenge server's
        DNS interface for the given host.
        """
        return self._postURL(
                self._URL("del-caa"),
                { "host": host })

    def http_request_history(self, host):
        """
        http_request_history fetches the challenge server's HTTP request history for the given host.
        """
        return json.loads(self._postURL(
                self._URL("get-http-history"),
                { "host": host }))

    def clear_http_request_history(self, host):
        """
        clear_http_request_history clears the challenge server's HTTP request history for the given host.
        """
        return self._clear_request_history(host, "http")

    def add_http_redirect(self, path, targetURL):
        """
        add_http_redirect adds a redirect to the challenge server's HTTP
        interfaces for HTTP requests to the given path directing the client to
        the targetURL. Redirects are not served for HTTPS requests.
        """
        return self._postURL(
                self._URL("add-redirect"),
                { "path": path, "targetURL": targetURL })

    def remove_http_redirect(self, path):
        """
        remove_http_redirect removes a redirect from the challenge server's HTTP
        interfaces for the given path.
        """
        return self._postURL(
                self._URL("del-redirect"),
                { "path": path })

    def add_http01_response(self, token, keyauth):
        """
        add_http01_response adds an ACME HTTP-01 challenge response for the
        provided token under the /.well-known/acme-challenge/ path of the
        challenge test server's HTTP interfaces. The given keyauth will be
        returned as the HTTP response body for requests to the challenge token.
        """
        return self._postURL(
                self._URL("add-http"),
                { "token": token, "content": keyauth })

    def remove_http01_response(self, token):
        """
        remove_http01_response removes an ACME HTTP-01 challenge response for
        the provided token from the challenge test server.
        """
        return self._postURL(
                self._URL("del-http"),
                { "token": token })

    def add_servfail_response(self, host):
        """
        add_servfail_response configures the challenge test server to return
        SERVFAIL for all queries made for the provided host. This will override
        any other mocks for the host until removed with remove_servfail_response.
        """
        return self._postURL(
                self._URL("add-servfail"),
                { "host": host})

    def remove_servfail_response(self, host):
        """
        remove_servfail_response undoes the work of add_servfail_response,
        removing the SERVFAIL configuration for the given host.
        """
        return self._postURL(
                self._URL("del-servfail"),
                { "host": host})

    def add_dns01_response(self, host, value):
        """
        add_dns01_response adds an ACME DNS-01 challenge response for the
        provided host to the challenge test server's DNS interfaces. The
        provided value will be served for TXT queries for
        _acme-challenge.<host>.
        """
        if host.endswith(".") is False:
            host = host + "."
        return self._postURL(
                self._URL("add-txt"),
                { "host": host, "value": value})

    def remove_dns01_response(self, host):
        """
        remove_dns01_response removes an ACME DNS-01 challenge response for the
        provided host from the challenge test server's DNS interfaces.
        """
        return self._postURL(
                self._URL("del-txt"),
                { "host": host })

    def dns_request_history(self, host):
        """
        dns_request_history returns the history of DNS requests made to the
        challenge test server's DNS interfaces for the given host.
        """
        return json.loads(self._postURL(
                self._URL("get-dns-history"),
                { "host": host }))

    def clear_dns_request_history(self, host):
        """
        clear_dns_request_history clears the history of DNS requests made to the
        challenge test server's DNS interfaces for the given host.
        """
        return self._clear_request_history(host, "dns")

    def add_tlsalpn01_response(self, host, value):
        """
        add_tlsalpn01_response adds an ACME TLS-ALPN-01 challenge response
        certificate to the challenge test server's TLS-ALPN-01 interface for the
        given host. The provided key authorization value will be embedded in the
        response certificate served to clients that initiate a TLS-ALPN-01
        challenge validation with the challenge test server for the provided
        host.
        """
        return self._postURL(
                self._URL("add-alpn"),
                { "host": host, "content": value})

    def remove_tlsalpn01_response(self, host):
        """
        remove_tlsalpn01_response removes an ACME TLS-ALPN-01 challenge response
        certificate from the challenge test server's TLS-ALPN-01 interface for
        the given host.
        """
        return self._postURL(
                self._URL("del-alpn"),
                { "host": host })

    def tlsalpn01_request_history(self, host):
        """
        tls_alpn01_request_history returns the history of TLS-ALPN-01 requests
        made to the challenge test server's TLS-ALPN-01 interface for the given
        host.
        """
        return json.loads(self._postURL(
                self._URL("get-alpn-history"),
                { "host": host }))

    def clear_tlsalpn01_request_history(self, host):
        """
        clear_tlsalpn01_request_history clears the history of TLS-ALPN-01
        requests made to the challenge test server's TLS-ALPN-01 interface for
        the given host.
        """
        return self._clear_request_history(host, "tlsalpn")
