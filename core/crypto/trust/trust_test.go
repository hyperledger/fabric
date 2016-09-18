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

// This file is the unit test for trust.go

package trust

import (
	"testing"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

func TestTrustStore(t *testing.T) {

	// Create a certificate
	primitives.SetSecurityLevel("SHA3", 256)

	// Initialize trust store for peer1 using MS1 (membership services 1)
	InitMgr("peer1", "/tmp/trust")
	mgr1 := GetMgr()
	mgr1.AddToCertPool("tls", newCert(t))
	mgr1.AddToCertPool("transaction", newCert(t))
	mgr1.AddToCertPool("transaction", newCert(t))
	mgr1.SetCertByID("peer1", newCert(t))

	// Initialize trust store for peer2 using MS2 (membership services 2)
	InitMgr("peer2", "/tmp/trust")
	mgr2 := GetMgr()
	mgr2.AddToCertPool("tls", newCert(t))
	mgr2.AddToCertPool("transaction", newCert(t))
	mgr2.AddToCertPool("transaction", newCert(t))
	mgr2.SetCertByID("peer2", newCert(t))

	// Initialize trust store again and make sure it contains appropriate certs
	InitMgr("peer3", "/tmp/trust")
	mgr3 := GetMgr()
	tlsPool := mgr3.GetCertPool("tls")
	if tlsPool == nil {
		t.Fatal("TLS cert pool not found")
	}
	tlsCount := len(tlsPool.Subjects())
	if tlsCount != 2 {
		t.Errorf("expecting 2 TLS certs but found %d", tlsCount)
	}
	transactionPool := mgr3.GetCertPool("transaction")
	if transactionPool == nil {
		t.Fatal("transaction cert pool not found")
	}
	transactionCount := len(transactionPool.Subjects())
	if transactionCount != 4 {
		t.Errorf("expecting 4 transaction certs but found %d", transactionCount)
	}
	peer1ECert := mgr3.GetCertByID("peer1")
	if peer1ECert == nil {
		t.Error("Did not find peer1's ECert")
	}
	peer2ECert := mgr3.GetCertByID("peer2")
	if peer2ECert == nil {
		t.Error("Did not find peer2's ECert")
	}
	peer3ECert := mgr3.GetCertByID("peer3")
	if peer3ECert != nil {
		t.Error("Found peer3's ECert")
	}
}

func newCert(t *testing.T) []byte {
	cert, _, err := primitives.NewSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed creating self signed cert [%s]", err)
	}
	return cert
}
