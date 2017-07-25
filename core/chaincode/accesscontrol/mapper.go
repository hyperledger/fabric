/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"encoding/base64"
	"encoding/pem"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/util"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

var ttl = time.Minute * 10

type certHash string

type certMapper struct {
	keyGen KeyGenFunc
	sync.RWMutex
	m   map[certHash]string
	tls bool
}

func newCertMapper(keyGen KeyGenFunc) *certMapper {
	return &certMapper{
		keyGen: keyGen,
		tls:    viper.GetBool("peer.tls.enabled"),
		m:      make(map[certHash]string),
	}
}

func (r *certMapper) lookup(h certHash) string {
	r.RLock()
	defer r.RUnlock()
	return r.m[h]
}

func (r *certMapper) register(hash certHash, name string) {
	r.Lock()
	defer r.Unlock()
	r.m[hash] = name
	time.AfterFunc(ttl, func() {
		r.purge(hash)
	})
}

func (r *certMapper) purge(hash certHash) {
	r.Lock()
	defer r.Unlock()
	delete(r.m, hash)
}

func certKeyPairFromString(privKey string, pubKey string) (*certKeyPair, error) {
	priv, err := base64.StdEncoding.DecodeString(privKey)
	if err != nil {
		return nil, err
	}
	pub, err := base64.StdEncoding.DecodeString(pubKey)
	if err != nil {
		return nil, err
	}
	return &certKeyPair{
		certBytes: pub,
		keyBytes:  priv,
	}, nil
}

func (r *certMapper) genCert(name string) (*certKeyPair, error) {
	keyPair, err := r.keyGen()
	if err != nil {
		return nil, err
	}
	hash := util.ComputeSHA256(keyPair.cert.Raw)
	r.register(certHash(hash), name)
	return keyPair, nil
}

func encodePEM(keyType string, data []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: keyType, Bytes: data})
}

// ExtractCertificateHash extracts the hash of the certificate from the stream
func extractCertificateHashFromContext(ctx context.Context) []byte {
	pr, extracted := peer.FromContext(ctx)
	if !extracted {
		return nil
	}

	authInfo := pr.AuthInfo
	if authInfo == nil {
		return nil
	}

	tlsInfo, isTLSConn := authInfo.(credentials.TLSInfo)
	if !isTLSConn {
		return nil
	}
	certs := tlsInfo.State.PeerCertificates
	if len(certs) == 0 {
		return nil
	}
	raw := certs[0].Raw
	if len(raw) == 0 {
		return nil
	}
	return util.ComputeSHA256(raw)
}
