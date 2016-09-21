/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package connection

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/transport"
)

type PeerInfo struct {
	addr string
	cert *x509.Certificate
	cp   *x509.CertPool
}

type Manager struct {
	Server    *grpc.Server
	Listener  net.Listener
	Self      PeerInfo
	tlsConfig *tls.Config
	Cert      *tls.Certificate
}

func New(addr string, certFile string, keyFile string) (_ *Manager, err error) {
	c := &Manager{}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}

	c.Cert = &cert
	c.Self, err = NewPeerInfo("", cert.Certificate[0])

	c.tlsConfig = &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true,
	}

	c.Listener, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	serverTls := c.tlsConfig
	serverTls.ServerName = addr
	c.Server = grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTls)))
	go c.Server.Serve(c.Listener)
	return c, nil
}

func (c *Manager) DialPeer(peer PeerInfo, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return dialPeer(&c.tlsConfig.Certificates[0], peer, opts...)
}

// to check client: credentials.FromContext() -> AuthInfo

type patchedAuthenticator struct {
	credentials.TransportCredentials
	pinnedCert    *x509.Certificate
	tunneledError error
}

func dialPeer(cert *tls.Certificate, peer PeerInfo, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	clientTLS := &tls.Config{InsecureSkipVerify: true}
	if cert != nil {
		clientTLS.Certificates = []tls.Certificate{*cert}
	}

	creds := credentials.NewTLS(clientTLS)
	patchedCreds := &patchedAuthenticator{
		TransportCredentials: creds,
		pinnedCert:           peer.cert,
	}
	opts = append(opts, grpc.WithTransportCredentials(patchedCreds))
	conn, err := grpc.Dial(peer.addr, opts...)
	if err != nil {
		if patchedCreds.tunneledError != nil {
			err = patchedCreds.tunneledError
		}
		return nil, err
	}

	return conn, nil
}

func DialPeer(peer PeerInfo, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return dialPeer(nil, peer, opts...)
}

func GetPeerInfo(s grpc.Stream) PeerInfo {
	var pi PeerInfo

	ctx := s.Context()
	trs, ok := transport.StreamFromContext(ctx)
	if ok {
		pi.addr = trs.ServerTransport().RemoteAddr().String()
	}

	p, _ := peer.FromContext(ctx)
	switch creds := p.AuthInfo.(type) {
	case credentials.TLSInfo:
		state := creds.State
		if len(state.PeerCertificates) > 0 {
			pi.cert = state.PeerCertificates[0]
		}
	}

	return pi
}

func NewPeerInfo(addr string, cert []byte) (_ PeerInfo, err error) {
	var p PeerInfo

	p.addr = addr
	p.cert, err = x509.ParseCertificate(cert)
	if err != nil {
		return
	}
	p.cp = x509.NewCertPool()
	p.cp.AddCert(p.cert)
	return p, nil
}

func (pi *PeerInfo) Fingerprint() string {
	return fmt.Sprintf("%x", sha256.Sum256(pi.cert.Raw))
}

func (pi *PeerInfo) Cert() *x509.Certificate {
	cert := *pi.cert
	return &cert
}

func (pi PeerInfo) String() string {
	return fmt.Sprintf("%.6s [%s]", pi.Fingerprint(), pi.addr)
}
