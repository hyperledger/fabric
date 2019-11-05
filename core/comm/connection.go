/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const defaultTimeout = time.Second * 3

var commLogger = flogging.MustGetLogger("comm")
var credSupport *CredentialSupport
var once sync.Once

// CertificateBundle bundles certificates
type CertificateBundle [][]byte

// PerOrgCertificateBundle maps organizations to CertificateBundles
type PerOrgCertificateBundle map[string]CertificateBundle

// OrgRootCAs defines root CA certificates of organizations, by their
// corresponding channels.
// channel --> organization --> certificates
type OrgRootCAs map[string]PerOrgCertificateBundle

type OrdererEndpoint struct {
	Address string
	PEMs    []byte
}

// CertificatesByChannelAndOrg returns the certificates of the given organization in the context
// of the given channel.
func (orc OrgRootCAs) CertificatesByChannelAndOrg(channel string, org string) CertificateBundle {
	if _, exists := orc[channel]; !exists {
		orc[channel] = make(PerOrgCertificateBundle)
	}
	return orc[channel][org]
}

// AppendCertificates appends certificates that belong to the given organization in the context of the given channel.
// This operation isn't thread safe.
func (orc OrgRootCAs) AppendCertificates(channel string, org string, rootCAs CertificateBundle) {
	certsByOrg, exists := orc[channel]
	if !exists {
		certsByOrg = make(PerOrgCertificateBundle)
		orc[channel] = certsByOrg
	}
	certificatesOfOrg := certsByOrg[org]
	certificatesOfOrg = append(certificatesOfOrg, rootCAs...)
	certsByOrg[org] = certificatesOfOrg
}

// CredentialSupport type manages credentials used for gRPC client connections
type CredentialSupport struct {
	sync.RWMutex
	AppRootCAsByChain           map[string]CertificateBundle
	OrdererRootCAsByChainAndOrg OrgRootCAs
	ClientRootCAs               CertificateBundle
	ServerRootCAs               CertificateBundle
	clientCert                  tls.Certificate
}

// GetCredentialSupport returns the singleton CredentialSupport instance
func GetCredentialSupport() *CredentialSupport {

	once.Do(func() {
		credSupport = &CredentialSupport{
			AppRootCAsByChain:           make(map[string]CertificateBundle),
			OrdererRootCAsByChainAndOrg: make(OrgRootCAs),
		}
	})
	return credSupport
}

// SetClientCertificate sets the tls.Certificate to use for gRPC client
// connections
func (cs *CredentialSupport) SetClientCertificate(cert tls.Certificate) {
	cs.clientCert = cert
}

// GetClientCertificate returns the client certificate of the CredentialSupport
func (cs *CredentialSupport) GetClientCertificate() tls.Certificate {
	return cs.clientCert
}

// GetDeliverServiceCredentials returns gRPC transport credentials for given channel
// to be used by gRPC clients which communicate with ordering service endpoints.
// If appendStaticRoots is set to true, ServerRootCAs are also included in the
// credentials.  If the channel isn't found, an error is returned.
func (cs *CredentialSupport) GetDeliverServiceCredentials(
	channelID string,
	appendStaticRoots bool,
	orgs []string,
	endpointOverrides map[string]*OrdererEndpoint,
) (credentials.TransportCredentials, error) {
	cs.RLock()
	defer cs.RUnlock()

	rootCACertsByOrg, exists := cs.OrdererRootCAsByChainAndOrg[channelID]
	if !exists {
		commLogger.Errorf("Attempted to obtain root CA certs of a non existent channel: %s", channelID)
		return nil, fmt.Errorf("didn't find any root CA certs for channel %s", channelID)
	}

	var rootCACerts CertificateBundle
	// Collect all TLS root CA certs for the organizations requested.
	for _, org := range orgs {
		rootCACerts = append(rootCACerts, rootCACertsByOrg[org]...)
	}

	// In case the peer is configured to use additional static TLS root CAs,
	// add them to the list as well.
	if appendStaticRoots {
		for _, cert := range cs.ServerRootCAs {
			rootCACerts = append(rootCACerts, cert)
		}
	}

	// Parse all PEM bundles and add them into the CA cert pool.
	certPool := x509.NewCertPool()

	for _, cert := range rootCACerts {
		block, _ := pem.Decode(cert)
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				certPool.AddCert(cert)
			} else {
				commLogger.Warningf("Failed to add root cert to credentials: %s", err)
			}
		} else {
			commLogger.Warning("Failed to add root cert to credentials")
		}
	}

	for _, override := range endpointOverrides {
		certPool.AppendCertsFromPEM(override.PEMs)
	}

	// Finally, create a TLS client config with the computed TLS root CAs.
	var creds credentials.TransportCredentials
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cs.clientCert},
		RootCAs:      certPool,
	}
	creds = credentials.NewTLS(tlsConfig)
	return creds, nil
}

// GetPeerCredentials returns gRPC transport credentials for use by gRPC
// clients which communicate with remote peer endpoints.
func (cs *CredentialSupport) GetPeerCredentials() credentials.TransportCredentials {
	cs.RLock()
	defer cs.RUnlock()

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cs.clientCert},
	}
	certPool := x509.NewCertPool()
	appRootCAs := [][]byte{}
	for _, appRootCA := range cs.AppRootCAsByChain {
		appRootCAs = append(appRootCAs, appRootCA...)
	}
	// also need to append statically configured root certs
	appRootCAs = append(appRootCAs, cs.ServerRootCAs...)
	// loop through the app root CAs
	for _, appRootCA := range appRootCAs {
		err := AddPemToCertPool(appRootCA, certPool)
		if err != nil {
			commLogger.Warningf("Failed adding certificates to peer's client TLS trust pool: %s", err)
		}
	}

	tlsConfig.RootCAs = certPool
	return credentials.NewTLS(tlsConfig)
}

func getEnv(key, def string) string {
	val := os.Getenv(key)
	if len(val) > 0 {
		return val
	} else {
		return def
	}
}

// NewClientConnectionWithAddress Returns a new grpc.ClientConn to the given address
func NewClientConnectionWithAddress(peerAddress string, block bool, tslEnabled bool,
	creds credentials.TransportCredentials, ka *KeepaliveOptions) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	if ka != nil {
		opts = ClientKeepaliveOptions(ka)
	} else {
		// set to the default options
		opts = ClientKeepaliveOptions(DefaultKeepaliveOptions)
	}

	if tslEnabled {
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}
	if block {
		opts = append(opts, grpc.WithBlock())
	}
	opts = append(opts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(MaxRecvMsgSize),
		grpc.MaxCallSendMsgSize(MaxSendMsgSize),
	))
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, peerAddress, opts...)
	if err != nil {
		return nil, err
	}
	return conn, err
}

func InitTLSForShim(key, certStr string) credentials.TransportCredentials {
	var sn string
	priv, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		commLogger.Panicf("failed decoding private key from base64, string: %s, error: %v", key, err)
	}
	pub, err := base64.StdEncoding.DecodeString(certStr)
	if err != nil {
		commLogger.Panicf("failed decoding public key from base64, string: %s, error: %v", certStr, err)
	}
	cert, err := tls.X509KeyPair(pub, priv)
	if err != nil {
		commLogger.Panicf("failed loading certificate: %v", err)
	}
	b, err := ioutil.ReadFile(config.GetPath("peer.tls.rootcert.file"))
	if err != nil {
		commLogger.Panicf("failed loading root ca cert: %v", err)
	}
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(b) {
		commLogger.Panicf("failed to append certificates")
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      cp,
		ServerName:   sn,
	})
}
