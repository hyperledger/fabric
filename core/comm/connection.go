/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/msp"
	"google.golang.org/grpc/credentials"
)

var commLogger = flogging.MustGetLogger("comm")
var credSupport *CredentialSupport
var once sync.Once

// CredentialSupport type manages credentials used for gRPC client connections
type CredentialSupport struct {
	sync.RWMutex
	appRootCAsByChain     map[string][][]byte
	OrdererRootCAsByChain map[string][][]byte
	ServerRootCAs         [][]byte
	clientCert            tls.Certificate
}

// GetCredentialSupport returns the singleton CredentialSupport instance
func GetCredentialSupport() *CredentialSupport {
	once.Do(func() {
		credSupport = &CredentialSupport{
			appRootCAsByChain:     make(map[string][][]byte),
			OrdererRootCAsByChain: make(map[string][][]byte),
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

// GetDeliverServiceCredentials returns gRPC transport credentials for given
// channel to be used by gRPC clients which communicate with ordering service endpoints.
// If the channel isn't found, an error is returned.
func (cs *CredentialSupport) GetDeliverServiceCredentials(channelID string) (credentials.TransportCredentials, error) {
	cs.RLock()
	defer cs.RUnlock()

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cs.clientCert},
	}
	certPool := x509.NewCertPool()

	rootCACerts, exists := cs.OrdererRootCAsByChain[channelID]
	if !exists {
		commLogger.Errorf("Attempted to obtain root CA certs of a non existent channel: %s", channelID)
		return nil, fmt.Errorf("didn't find any root CA certs for channel %s", channelID)
	}

	for _, cert := range rootCACerts {
		block, _ := pem.Decode(cert)
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				certPool.AddCert(cert)
			} else {
				commLogger.Warningf("Failed to add root cert to credentials (%s)", err)
			}
		} else {
			commLogger.Warning("Failed to add root cert to credentials")
		}
	}
	tlsConfig.RootCAs = certPool
	return credentials.NewTLS(tlsConfig), nil
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
	for _, appRootCA := range cs.appRootCAsByChain {
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

func (cs *CredentialSupport) AppRootCAsByChain() map[string][][]byte {
	cs.RLock()
	defer cs.RUnlock()
	return cs.appRootCAsByChain
}

// BuildTrustedRootsForChain populates the appRootCAs and orderRootCAs maps by
// getting the root and intermediate certs for all msps associated with the
// MSPManager.
func (cs *CredentialSupport) BuildTrustedRootsForChain(cm channelconfig.Resources) {
	cs.Lock()
	defer cs.Unlock()

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

	cid := cm.ConfigtxValidator().ChainID()
	commLogger.Debugf("updating root CAs for channel [%s]", cid)
	msps, err := cm.MSPManager().GetMSPs()
	if err != nil {
		commLogger.Errorf("Error getting root CAs for channel %s (%s)", cid, err)
		return
	}

	var appRootCAs [][]byte
	var ordererRootCAs [][]byte
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
			// check to see of this is an orderer org MSP
			if _, ok := ordOrgMSPs[k]; ok {
				commLogger.Debugf("adding orderer root CAs for MSP [%s]", k)
				ordererRootCAs = append(ordererRootCAs, root)
			}
		}
		for _, intermediate := range v.GetTLSIntermediateCerts() {
			// check to see of this is an app org MSP
			if _, ok := appOrgMSPs[k]; ok {
				commLogger.Debugf("adding app root CAs for MSP [%s]", k)
				appRootCAs = append(appRootCAs, intermediate)
			}
			// check to see of this is an orderer org MSP
			if _, ok := ordOrgMSPs[k]; ok {
				commLogger.Debugf("adding orderer root CAs for MSP [%s]", k)
				ordererRootCAs = append(ordererRootCAs, intermediate)
			}
		}
	}
	cs.appRootCAsByChain[cid] = appRootCAs
	cs.OrdererRootCAsByChain[cid] = ordererRootCAs
}
