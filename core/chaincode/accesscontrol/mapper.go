/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/util"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

var ttl = time.Minute * 10

type certHash string

type KeyGenFunc func() (*tlsgen.CertKeyPair, error)

type certMapper struct {
	keyGen KeyGenFunc
	sync.RWMutex
	m map[certHash]string
}

func newCertMapper(keyGen KeyGenFunc) *certMapper {
	return &certMapper{
		keyGen: keyGen,
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

func (r *certMapper) genCert(name string) (*tlsgen.CertKeyPair, error) {
	keyPair, err := r.keyGen()
	if err != nil {
		return nil, err
	}
	hash := util.ComputeSHA256(keyPair.TLSCert.Raw)
	r.register(certHash(hash), name)
	return keyPair, nil
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
