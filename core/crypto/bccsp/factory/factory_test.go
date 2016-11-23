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
package factory

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/crypto/bccsp/sw"
)

func TestGetDefault(t *testing.T) {
	bccsp, err := GetDefault()
	if err != nil {
		t.Fatalf("Failed getting default BCCSP [%s]", err)
	}
	if bccsp == nil {
		t.Fatal("Failed getting default BCCSP. Nil instance.")
	}
}

func TestGetBCCPEphemeral(t *testing.T) {
	ks := &sw.FileBasedKeyStore{}
	if err := ks.Init(nil, os.TempDir(), false); err != nil {
		t.Fatalf("Failed initializing key store [%s]", err)
	}

	bccsp1, err := GetBCCSP(&SwOpts{Ephemeral_: true, SecLevel: 256, HashFamily: "SHA2", KeyStore: ks})
	if err != nil {
		t.Fatalf("Failed getting ephemeral software-based BCCSP [%s]", err)
	}

	bccsp2, err := GetBCCSP(&SwOpts{Ephemeral_: true, SecLevel: 256, HashFamily: "SHA2", KeyStore: ks})
	if err != nil {
		t.Fatalf("Failed getting ephemeral software-based BCCSP [%s]", err)
	}

	if bccsp1 == bccsp2 {
		t.Fatal("Ephemeral BCCSPs should point to different instances")
	}
}

func TestGetBCCP2Ephemeral(t *testing.T) {
	ks := &sw.FileBasedKeyStore{}
	if err := ks.Init(nil, os.TempDir(), false); err != nil {
		t.Fatalf("Failed initializing key store [%s]", err)
	}

	bccsp1, err := GetBCCSP(&SwOpts{Ephemeral_: false, SecLevel: 256, HashFamily: "SHA2", KeyStore: ks})
	if err != nil {
		t.Fatalf("Failed getting non-ephemeral software-based BCCSP [%s]", err)
	}

	bccsp2, err := GetBCCSP(&SwOpts{Ephemeral_: false, SecLevel: 256, HashFamily: "SHA2", KeyStore: ks})
	if err != nil {
		t.Fatalf("Failed getting non-ephemeral software-based BCCSP [%s]", err)
	}

	if bccsp1 != bccsp2 {
		t.Fatal("Non-ephemeral BCCSPs should point to the same instance")
	}
}
