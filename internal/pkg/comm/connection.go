/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/tls"
	"crypto/x509"
	"sync"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/msp"
	"google.golang.org/grpc/credentials"
)

var commLogger = flogging.MustGetLogger("comm")

// CredentialSupport type manages credentials used for gRPC client connections
type CredentialSupport struct {
	mutex             sync.RWMutex
	appRootCAsByChain map[string][][]byte
	serverRootCAs     [][]byte
	clientCert        tls.Certificate
}

// NewCredentialSupport creates a CredentialSupport instance.
func NewCredentialSupport(rootCAs ...[]byte) *CredentialSupport {
	return &CredentialSupport{
		appRootCAsByChain: make(map[string][][]byte),
		serverRootCAs:     rootCAs,
	}
}

// SetClientCertificate sets the tls.Certificate to use for gRPC client
// connections
func (cs *CredentialSupport) SetClientCertificate(cert tls.Certificate) {
	cs.mutex.Lock()
	cs.clientCert = cert
	cs.mutex.Unlock()
}

// GetClientCertificate returns the client certificate of the CredentialSupport
func (cs *CredentialSupport) GetClientCertificate() tls.Certificate {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	return cs.clientCert
}

// GetPeerCredentials returns gRPC transport credentials for use by gRPC
// clients which communicate with remote peer endpoints.
func (cs *CredentialSupport) GetPeerCredentials() credentials.TransportCredentials {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()

	var appRootCAs [][]byte
	appRootCAs = append(appRootCAs, cs.serverRootCAs...)
	for _, appRootCA := range cs.appRootCAsByChain {
		appRootCAs = append(appRootCAs, appRootCA...)
	}

	certPool := x509.NewCertPool()
	for _, appRootCA := range appRootCAs {
		if !certPool.AppendCertsFromPEM(appRootCA) {
			commLogger.Warningf("Failed adding certificates to peer's client TLS trust pool")
		}
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cs.clientCert},
		RootCAs:      certPool,
	})
}

func (cs *CredentialSupport) AppRootCAsByChain() map[string][][]byte {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	return cs.appRootCAsByChain
}

// BuildTrustedRootsForChain populates the appRootCAs and orderRootCAs maps by
// getting the root and intermediate certs for all msps associated with the
// MSPManager.
func (cs *CredentialSupport) BuildTrustedRootsForChain(cm channelconfig.Resources) {
	appOrgMSPs := make(map[string]struct{})
	if ac, ok := cm.ApplicationConfig(); ok {
		for _, appOrg := range ac.Organizations() {
			appOrgMSPs[appOrg.MSPID()] = struct{}{}
		}
	}

	ordOrgMSPs := make(map[string]struct{})
	if ac, ok := cm.OrdererConfig(); ok {
		for _, ordOrg := range ac.Organizations() {
			ordOrgMSPs[ordOrg.MSPID()] = struct{}{}
		}
	}

	cid := cm.ConfigtxValidator().ChannelID()
	msps, err := cm.MSPManager().GetMSPs()
	if err != nil {
		commLogger.Errorf("Error getting root CAs for channel %s (%s)", cid, err)
		return
	}

	var appRootCAs [][]byte
	for k, v := range msps {
		// we only support the fabric MSP
		if v.GetType() != msp.FABRIC {
			continue
		}

		for _, root := range v.GetTLSRootCerts() {
			// check to see of this is an app org MSP
			if _, ok := appOrgMSPs[k]; ok {
				commLogger.Debugf("adding app root CAs for MSP [%s]", k)
				appRootCAs = append(appRootCAs, root)
			}
		}
		for _, intermediate := range v.GetTLSIntermediateCerts() {
			// check to see of this is an app org MSP
			if _, ok := appOrgMSPs[k]; ok {
				commLogger.Debugf("adding app root CAs for MSP [%s]", k)
				appRootCAs = append(appRootCAs, intermediate)
			}
		}
	}

	cs.mutex.Lock()
	cs.appRootCAsByChain[cid] = appRootCAs
	cs.mutex.Unlock()
}
