/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"io/ioutil"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/pkg/errors"
)

type genTLSCertFunc func() (*tlsgen.CertKeyPair, error)

// Config defines configuration of a Client
type Config struct {
	CertPath       string
	KeyPath        string
	PeerCACertPath string
	Timeout        time.Duration
}

// ToSecureOptions converts this Config to SecureOptions.
// The given function generates a self signed client TLS certificate if
// the TLS certificate and key aren't present at the config
func (conf Config) ToSecureOptions(newSelfSignedTLSCert genTLSCertFunc) (comm.SecureOptions, error) {
	if conf.PeerCACertPath == "" {
		return comm.SecureOptions{}, nil
	}
	caBytes, err := loadFile(conf.PeerCACertPath)
	if err != nil {
		return comm.SecureOptions{}, errors.WithStack(err)
	}
	var keyBytes, certBytes []byte
	// If TLS key and certificate aren't given, generate a self signed one on the fly
	if conf.KeyPath == "" && conf.CertPath == "" {
		tlsCert, err := newSelfSignedTLSCert()
		if err != nil {
			return comm.SecureOptions{}, err
		}
		keyBytes, certBytes = tlsCert.Key, tlsCert.Cert
	} else {
		keyBytes, err = loadFile(conf.KeyPath)
		if err != nil {
			return comm.SecureOptions{}, errors.WithStack(err)
		}
		certBytes, err = loadFile(conf.CertPath)
		if err != nil {
			return comm.SecureOptions{}, errors.WithStack(err)
		}
	}
	return comm.SecureOptions{
		Key:               keyBytes,
		Certificate:       certBytes,
		UseTLS:            true,
		ServerRootCAs:     [][]byte{caBytes},
		RequireClientCert: true,
	}, nil
}

func loadFile(path string) ([]byte, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Errorf("Failed opening file %s: %v", path, err)
	}
	return b, nil
}
