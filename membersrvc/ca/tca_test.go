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

package ca

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/membersrvc/protos"
)

func TestNewTCA(t *testing.T) {
	tca, err := initTCA()
	if err != nil {
		t.Fatal(err)
	}

	if tca.hmacKey == nil || len(tca.hmacKey) == 0 {
		t.Fatal("Could not read hmacKey from TCA")
	}

	if tca.rootPreKey == nil || len(tca.rootPreKey) == 0 {
		t.Fatal("Could not read rootPreKey from TCA")
	}

	if tca.preKeys == nil || len(tca.preKeys) == 0 {
		t.Fatal("Could not read preKeys from TCA")
	}
}

func TestCreateCertificateSet(t *testing.T) {
	tca, err := initTCA()
	if err != nil {
		t.Fatal(err)
	}

	enrollmentID := "test_user0"
	enrollmentPassword := "MS9qrN8hFjlE"

	ecertRaw, priv, err := loadECertAndEnrollmentPrivateKey(enrollmentID, enrollmentPassword)
	if err != nil {
		t.Fatal(err)
	}

	const expectedTcertSubjectCommonNameValue string = "Transaction Certificate"
	ncerts := 1
	for nattributes := -1; nattributes < 1; nattributes++ {
		certificateSetRequest, err := buildCertificateSetRequest(enrollmentID, priv, ncerts, nattributes)
		if err != nil {
			t.Fatal(err)
		}

		var certSets []*TCertSet
		certSets, err = tca.getCertificateSets(enrollmentID)
		if err != nil {
			t.Fatal(err)
		}

		certSetsCountBefore := len(certSets)

		tcap := &TCAP{tca}
		response, err := tcap.createCertificateSet(context.Background(), ecertRaw, certificateSetRequest)
		if err != nil {
			t.Fatal(err)
		}

		certSets, err = tca.getCertificateSets(enrollmentID)
		if err != nil {
			t.Fatal(err)
		}
		certSetsCountAfter := len(certSets)

		if certSetsCountBefore != certSetsCountAfter-1 {
			t.Fatal("TCertSets count should be increased by 1 after requesting a new set of TCerts")
		}

		tcerts := response.GetCerts()
		if len(tcerts.Certs) != ncerts {
			t.Fatal(fmt.Errorf("Invalid tcert size. Expected: %v, Actual: %v", ncerts, len(tcerts.Certs)))
		}

		for pos, eachTCert := range tcerts.Certs {
			tcert, err := x509.ParseCertificate(eachTCert.Cert)
			if err != nil {
				t.Fatalf("Error: %v\nCould not x509.ParseCertificate %v", err, eachTCert.Cert)
			}

			t.Logf("Examining TCert[%d]'s Subject: %v", pos, tcert.Subject)
			if tcert.Subject.CommonName != expectedTcertSubjectCommonNameValue {
				t.Fatalf("The TCert's Subject.CommonName is '%s' which is different than '%s'", tcert.Subject.CommonName, expectedTcertSubjectCommonNameValue)
			}
			t.Logf("Successfully verified that TCert[%d].Subject.CommonName == '%s'", pos, tcert.Subject.CommonName)
		}
	}
}

func loadECertAndEnrollmentPrivateKey(enrollmentID string, password string) ([]byte, *ecdsa.PrivateKey, error) {
	cooked, err := ioutil.ReadFile("./test_resources/key_" + enrollmentID + ".dump")
	if err != nil {
		return nil, nil, err
	}

	block, _ := pem.Decode(cooked)
	decryptedBlock, err := x509.DecryptPEMBlock(block, []byte(password))
	if err != nil {
		return nil, nil, err
	}

	enrollmentPrivateKey, err := x509.ParseECPrivateKey(decryptedBlock)
	if err != nil {
		return nil, nil, err
	}

	if err != nil {
		return nil, nil, err
	}

	ecertRaw, err := ioutil.ReadFile("./test_resources/ecert_" + enrollmentID + ".dump")
	if err != nil {
		return nil, nil, err
	}

	return ecertRaw, enrollmentPrivateKey, nil
}

func initTCA() (*TCA, error) {
	//init the crypto layer
	if err := crypto.Init(); err != nil {
		return nil, fmt.Errorf("Failed initializing the crypto layer [%v]", err)
	}

	CacheConfiguration() // Cache configuration

	aca := NewACA()
	if aca == nil {
		return nil, fmt.Errorf("Could not create a new ACA")
	}

	eca := NewECA(aca)
	if eca == nil {
		return nil, fmt.Errorf("Could not create a new ECA")
	}

	tca := NewTCA(eca)
	if tca == nil {
		return nil, fmt.Errorf("Could not create a new TCA")
	}

	return tca, nil
}

func buildCertificateSetRequest(enrollID string, enrollmentPrivKey *ecdsa.PrivateKey, num, numattrs int) (*protos.TCertCreateSetReq, error) {
	now := time.Now()
	timestamp := timestamp.Timestamp{Seconds: int64(now.Second()), Nanos: int32(now.Nanosecond())}

	var attributes []*protos.TCertAttribute
	if numattrs >= 0 { // else negative means use nil from above
		attributes = make([]*protos.TCertAttribute, numattrs)
	}

	req := &protos.TCertCreateSetReq{
		Ts:         &timestamp,
		Id:         &protos.Identity{Id: enrollID},
		Num:        uint32(num),
		Attributes: attributes,
		Sig:        nil,
	}

	rawReq, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("Failed marshaling request [%v].", err)
	}

	r, s, err := primitives.ECDSASignDirect(enrollmentPrivKey, rawReq)
	if err != nil {
		return nil, fmt.Errorf("Failed creating signature for [%v]: [%v].", rawReq, err)
	}

	R, _ := r.MarshalText()
	S, _ := s.MarshalText()

	req.Sig = &protos.Signature{Type: protos.CryptoType_ECDSA, R: R, S: S}
	return req, nil
}
