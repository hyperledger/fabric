/*
Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package backend

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/rsa"
	"testing"
)

func TestSignAndVerifyRsa(t *testing.T) {
	data := []byte{1, 1, 1, 1, 1}
	privateKey, err := rsa.GenerateKey(crand.Reader, 1024)
	if err != nil {
		panic("RSA failed to generate private key in test.")
	}
	s := Sign(privateKey, data)
	if s == nil {
		t.Error("Nil signature was generated.")
	}

	publicKey := privateKey.Public()
	err = CheckSig(publicKey, data, s)
	if err != nil {
		t.Errorf("Signature check failed: %s", err)
	}
}

func TestSignAndVerifyEcdsa(t *testing.T) {
	data := []byte{1, 1, 1, 1, 1}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		panic("ECDSA failed to generate private key in test.")
	}
	s := Sign(privateKey, data)
	if s == nil {
		t.Error("Nil signature was generated.")
	}

	publicKey := privateKey.Public()
	err = CheckSig(publicKey, data, s)
	if err != nil {
		t.Errorf("Signature check failed: %s", err)
	}
}
