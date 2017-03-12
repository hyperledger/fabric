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
package signer

import (
	"crypto/rand"
	"os"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
)

var (
	swBCCSPInstance bccsp.BCCSP
)

func getBCCSP(t *testing.T) bccsp.BCCSP {
	if swBCCSPInstance == nil {
		var err error
		swBCCSPInstance, err = sw.NewDefaultSecurityLevel(os.TempDir())
		if err != nil {
			t.Fatalf("Failed initializing key store [%s]", err)
		}
	}

	return swBCCSPInstance
}

func TestInit(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	signer := &CryptoSigner{}
	err = signer.Init(csp, k)
	if err != nil {
		t.Fatalf("Failed initializing CryptoSigner [%s]", err)
	}
}

func TestPublic(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	signer := &CryptoSigner{}
	err = signer.Init(csp, k)
	if err != nil {
		t.Fatalf("Failed initializing CryptoSigner [%s]", err)
	}

	pk := signer.Public()
	if pk == nil {
		t.Fatal("Failed getting PublicKey. Nil.")
	}
}

func TestSign(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	signer := &CryptoSigner{}
	err = signer.Init(csp, k)
	if err != nil {
		t.Fatalf("Failed initializing CryptoSigner [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, nil)
	if err != nil {
		t.Fatalf("Failed generating digest [%s]", err)
	}

	signature, err := signer.Sign(rand.Reader, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := csp.Verify(k, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}
