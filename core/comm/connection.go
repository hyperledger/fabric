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

package comm

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

const defaultTimeout = time.Second * 3

var commLogger = logging.MustGetLogger("comm")
var caSupport *CASupport
var once sync.Once

// CASupport type manages certificate authorities scoped by channel
type CASupport struct {
	sync.RWMutex
	AppRootCAsByChain     map[string][][]byte
	OrdererRootCAsByChain map[string][][]byte
	ClientRootCAs         [][]byte
	ServerRootCAs         [][]byte
}

// GetCASupport returns the signleton CASupport instance
func GetCASupport() *CASupport {

	once.Do(func() {
		caSupport = &CASupport{
			AppRootCAsByChain:     make(map[string][][]byte),
			OrdererRootCAsByChain: make(map[string][][]byte),
		}
	})
	return caSupport
}

// GetServerRootCAs returns the PEM-encoded root certificates for all of the
// application and orderer organizations defined for all chains.  The root
// certificates returned should be used to set the trusted server roots for
// TLS clients.
func (cas *CASupport) GetServerRootCAs() (appRootCAs, ordererRootCAs [][]byte) {
	cas.RLock()
	defer cas.RUnlock()

	appRootCAs = [][]byte{}
	ordererRootCAs = [][]byte{}

	for _, appRootCA := range cas.AppRootCAsByChain {
		appRootCAs = append(appRootCAs, appRootCA...)
	}

	for _, ordererRootCA := range cas.AppRootCAsByChain {
		ordererRootCAs = append(ordererRootCAs, ordererRootCA...)
	}

	// also need to append statically configured root certs
	appRootCAs = append(appRootCAs, cas.ServerRootCAs...)
	ordererRootCAs = append(ordererRootCAs, cas.ServerRootCAs...)
	return appRootCAs, ordererRootCAs
}

// GetDeliverServiceCredentials returns GRPC transport credentials for use by GRPC
// clients which communicate with ordering service endpoints.
func (cas *CASupport) GetDeliverServiceCredentials() credentials.TransportCredentials {
	var creds credentials.TransportCredentials
	var tlsConfig = &tls.Config{}
	var certPool = x509.NewCertPool()
	// loop through the orderer CAs
	_, roots := cas.GetServerRootCAs()
	for _, root := range roots {
		block, _ := pem.Decode(root)
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
	creds = credentials.NewTLS(tlsConfig)
	return creds
}

// GetClientRootCAs returns the PEM-encoded root certificates for all of the
// application and orderer organizations defined for all chains.  The root
// certificates returned should be used to set the trusted client roots for
// TLS servers.
func (cas *CASupport) GetClientRootCAs() (appRootCAs, ordererRootCAs [][]byte) {
	cas.RLock()
	defer cas.RUnlock()

	appRootCAs = [][]byte{}
	ordererRootCAs = [][]byte{}

	for _, appRootCA := range cas.AppRootCAsByChain {
		appRootCAs = append(appRootCAs, appRootCA...)
	}

	for _, ordererRootCA := range cas.AppRootCAsByChain {
		ordererRootCAs = append(ordererRootCAs, ordererRootCA...)
	}

	// also need to append statically configured root certs
	appRootCAs = append(appRootCAs, cas.ClientRootCAs...)
	ordererRootCAs = append(ordererRootCAs, cas.ClientRootCAs...)
	return appRootCAs, ordererRootCAs
}

func getEnv(key, def string) string {
	val := os.Getenv(key)
	if len(val) > 0 {
		return val
	} else {
		return def
	}
}

func GetPeerTestingAddress(port string) string {
	return getEnv("UNIT_TEST_PEER_IP", "localhost") + ":" + port
}

// NewClientConnectionWithAddress Returns a new grpc.ClientConn to the given address.
func NewClientConnectionWithAddress(peerAddress string, block bool, tslEnabled bool, creds credentials.TransportCredentials) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if tslEnabled {
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}
	opts = append(opts, grpc.WithTimeout(defaultTimeout))
	if block {
		opts = append(opts, grpc.WithBlock())
	}
	conn, err := grpc.Dial(peerAddress, opts...)
	if err != nil {
		return nil, err
	}
	return conn, err
}

// InitTLSForPeer returns TLS credentials for peer
func InitTLSForPeer() credentials.TransportCredentials {
	var sn string
	if viper.GetString("peer.tls.serverhostoverride") != "" {
		sn = viper.GetString("peer.tls.serverhostoverride")
	}
	var creds credentials.TransportCredentials
	if viper.GetString("peer.tls.rootcert.file") != "" {
		var err error
		creds, err = credentials.NewClientTLSFromFile(viper.GetString("peer.tls.rootcert.file"), sn)
		if err != nil {
			grpclog.Fatalf("Failed to create TLS credentials %v", err)
		}
	} else {
		creds = credentials.NewClientTLSFromCert(nil, sn)
	}
	return creds
}
