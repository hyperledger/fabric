// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc/keepalive"
)

// Config contains chaincode's configuration
type Config struct {
	ChaincodeName string
	TLS           *tls.Config
	KaOpts        keepalive.ClientParameters
}

// LoadConfig loads the chaincode configuration
func LoadConfig() (Config, error) {
	var err error
	tlsEnabled, err := strconv.ParseBool(os.Getenv("CORE_PEER_TLS_ENABLED"))
	if err != nil {
		return Config{}, errors.New("'CORE_PEER_TLS_ENABLED' must be set to 'true' or 'false'")
	}

	conf := Config{
		ChaincodeName: os.Getenv("CORE_CHAINCODE_ID_NAME"),
		// hardcode to match chaincode server
		KaOpts: keepalive.ClientParameters{
			Time:                1 * time.Minute,
			Timeout:             20 * time.Second,
			PermitWithoutStream: true,
		},
	}

	if !tlsEnabled {
		return conf, nil
	}

	var key []byte
	path, set := os.LookupEnv("CORE_TLS_CLIENT_KEY_FILE")
	if set {
		key, err = ioutil.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("failed to read private key file: %s", err)
		}
	} else {
		data, err := ioutil.ReadFile(os.Getenv("CORE_TLS_CLIENT_KEY_PATH"))
		if err != nil {
			return Config{}, fmt.Errorf("failed to read private key file: %s", err)
		}
		key, err = base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return Config{}, fmt.Errorf("failed to decode private key file: %s", err)
		}
	}

	var cert []byte
	path, set = os.LookupEnv("CORE_TLS_CLIENT_CERT_FILE")
	if set {
		cert, err = ioutil.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("failed to read public key file: %s", err)
		}
	} else {
		data, err := ioutil.ReadFile(os.Getenv("CORE_TLS_CLIENT_CERT_PATH"))
		if err != nil {
			return Config{}, fmt.Errorf("failed to read public key file: %s", err)
		}
		cert, err = base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return Config{}, fmt.Errorf("failed to decode public key file: %s", err)
		}
	}

	root, err := ioutil.ReadFile(os.Getenv("CORE_PEER_TLS_ROOTCERT_FILE"))
	if err != nil {
		return Config{}, fmt.Errorf("failed to read root cert file: %s", err)
	}

	tlscfg, err := LoadTLSConfig(false, key, cert, root)
	if err != nil {
		return Config{}, err
	}

	conf.TLS = tlscfg

	return conf, nil
}

// LoadTLSConfig loads the TLS configuration for the chaincode
func LoadTLSConfig(isserver bool, key, cert, root []byte) (*tls.Config, error) {
	if key == nil {
		return nil, fmt.Errorf("key not provided")
	}

	if cert == nil {
		return nil, fmt.Errorf("cert not provided")
	}

	if !isserver && root == nil {
		return nil, fmt.Errorf("root cert not provided")
	}

	cccert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse client key pair: %s", err)
	}

	var rootCertPool *x509.CertPool
	if root != nil {
		rootCertPool = x509.NewCertPool()
		if ok := rootCertPool.AppendCertsFromPEM(root); !ok {
			return nil, errors.New("failed to load root cert file")
		}
	}

	tlscfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cccert},
	}

	//follow Peer's server default config properties
	if isserver {
		tlscfg.ClientCAs = rootCertPool
		tlscfg.SessionTicketsDisabled = true
		tlscfg.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		}
		if rootCertPool != nil {
			tlscfg.ClientAuth = tls.RequireAndVerifyClientCert
		}
	} else {
		tlscfg.RootCAs = rootCertPool
	}

	return tlscfg, nil
}
