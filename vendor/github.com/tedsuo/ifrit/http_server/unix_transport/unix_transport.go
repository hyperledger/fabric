package unix_transport

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func NewWithTLS(socketPath string, tlsConfig *tls.Config) *http.Transport {
	unixTransport := &http.Transport{TLSClientConfig: tlsConfig}

	unixTransport.RegisterProtocol("unix", NewUnixRoundTripperTls(socketPath, tlsConfig))
	return unixTransport
}

func New(socketPath string) *http.Transport {
	unixTransport := &http.Transport{}
	unixTransport.RegisterProtocol("unix", NewUnixRoundTripper(socketPath))
	return unixTransport
}

type UnixRoundTripper struct {
	path      string
	conn      httputil.ClientConn
	useTls    bool
	tlsConfig *tls.Config
}

func NewUnixRoundTripper(path string) *UnixRoundTripper {
	return &UnixRoundTripper{path: path}
}

func NewUnixRoundTripperTls(path string, tlsConfig *tls.Config) *UnixRoundTripper {
	return &UnixRoundTripper{
		path:      path,
		useTls:    true,
		tlsConfig: tlsConfig,
	}
}

// The RoundTripper (http://golang.org/pkg/net/http/#RoundTripper) for the socket transport dials the socket
// each time a request is made.
func (roundTripper UnixRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var conn net.Conn
	var err error
	if roundTripper.useTls {

		conn, err = tls.Dial("unix", roundTripper.path, roundTripper.tlsConfig)
		if err != nil {
			return nil, err
		}
		if conn == nil {
			return nil, errors.New("net/http: Transport.DialTLS returned (nil, nil)")
		}
		if tc, ok := conn.(*tls.Conn); ok {
			// Handshake here, in case DialTLS didn't. TLSNextProto below
			// depends on it for knowing the connection state.
			if err := tc.Handshake(); err != nil {
				go conn.Close()
				return nil, err
			}
		}
	} else {
		conn, err = net.Dial("unix", roundTripper.path)
		if err != nil {
			return nil, err
		}
	}

	socketClientConn := httputil.NewClientConn(conn, nil)
	defer socketClientConn.Close()

	newReq, err := roundTripper.rewriteRequest(req)
	if err != nil {
		return nil, err
	}

	return socketClientConn.Do(newReq)
}

func (roundTripper *UnixRoundTripper) rewriteRequest(req *http.Request) (*http.Request, error) {
	requestPath := req.URL.Path
	if !strings.HasPrefix(requestPath, roundTripper.path) {
		return nil, fmt.Errorf("Wrong unix socket [unix://%s]. Expected unix socket is [%s]", requestPath, roundTripper.path)
	}

	reqPath := strings.TrimPrefix(requestPath, roundTripper.path)
	newReqUrl := fmt.Sprintf("unix://%s", reqPath)

	var err error
	newURL, err := url.Parse(newReqUrl)
	if err != nil {
		return nil, err
	}

	req.URL.Path = newURL.Path
	req.URL.Host = roundTripper.path
	return req, nil

}
