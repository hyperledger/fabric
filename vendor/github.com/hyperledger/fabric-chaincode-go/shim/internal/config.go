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

// Config ...
type Config struct {
	ChaincodeName string
	TLS           *tls.Config
	KaOpts        keepalive.ClientParameters
}

// LoadConfig ...
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

	rootCertPool := x509.NewCertPool()
	if ok := rootCertPool.AppendCertsFromPEM(root); !ok {
		return Config{}, errors.New("failed to load root cert file")
	}
	clientCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return Config{}, errors.New("failed to parse client key pair")
	}

	conf.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      rootCertPool,
	}

	return conf, nil
}
