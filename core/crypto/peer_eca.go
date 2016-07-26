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

package crypto

import (
	"crypto/x509"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
	membersrvc "github.com/hyperledger/fabric/membersrvc/protos"
	"golang.org/x/net/context"
)

func (peer *peerImpl) getEnrollmentCert(id []byte) (*x509.Certificate, error) {
	if len(id) == 0 {
		return nil, fmt.Errorf("Invalid peer id. It is empty.")
	}

	sid := utils.EncodeBase64(id)

	peer.Debugf("Getting enrollment certificate for [%s]", sid)

	if cert := peer.getNodeEnrollmentCertificate(sid); cert != nil {
		peer.Debugf("Enrollment certificate for [%s] already in memory.", sid)
		return cert, nil
	}

	// Retrieve from the DB or from the ECA in case
	peer.Debugf("Retrieve Enrollment certificate for [%s]...", sid)
	rawCert, err := peer.ks.GetSignEnrollmentCert(id, peer.getEnrollmentCertByHashFromECA)
	if err != nil {
		peer.Errorf("Failed getting enrollment certificate for [%s]: [%s]", sid, err)

		return nil, err
	}

	cert, err := primitives.DERToX509Certificate(rawCert)
	if err != nil {
		peer.Errorf("Failed parsing enrollment certificate for [%s]: [% x],[% x]", sid, rawCert, err)

		return nil, err
	}

	peer.putNodeEnrollmentCertificate(sid, cert)

	return cert, nil
}

func (peer *peerImpl) getEnrollmentCertByHashFromECA(id []byte) ([]byte, []byte, error) {
	// Prepare the request
	peer.Debugf("Reading certificate for hash [% x]", id)

	req := &membersrvc.Hash{Hash: id}
	response, err := peer.callECAReadCertificateByHash(context.Background(), req)
	if err != nil {
		peer.Errorf("Failed requesting enrollment certificate [%s].", err.Error())

		return nil, nil, err
	}

	peer.Debugf("Certificate for hash [% x] = [% x][% x]", id, response.Sign, response.Enc)

	// Verify response.Sign
	x509Cert, err := primitives.DERToX509Certificate(response.Sign)
	if err != nil {
		peer.Errorf("Failed parsing signing enrollment certificate for encrypting: [%s]", err)

		return nil, nil, err
	}

	// Check role
	roleRaw, err := primitives.GetCriticalExtension(x509Cert, ECertSubjectRole)
	if err != nil {
		peer.Errorf("Failed parsing ECertSubjectRole in enrollment certificate for signing: [%s]", err)

		return nil, nil, err
	}

	role, err := strconv.ParseInt(string(roleRaw), 10, len(roleRaw)*8)
	if err != nil {
		peer.Errorf("Failed parsing ECertSubjectRole in enrollment certificate for signing: [%s]", err)

		return nil, nil, err
	}

	if membersrvc.Role(role) != membersrvc.Role_VALIDATOR && membersrvc.Role(role) != membersrvc.Role_PEER {
		peer.Errorf("Invalid ECertSubjectRole in enrollment certificate for signing. Not a validator or peer: [%s]", err)

		return nil, nil, err
	}

	return response.Sign, response.Enc, nil
}

func (peer *peerImpl) getNodeEnrollmentCertificate(sid string) *x509.Certificate {
	peer.nodeEnrollmentCertificatesMutex.RLock()
	defer peer.nodeEnrollmentCertificatesMutex.RUnlock()
	return peer.nodeEnrollmentCertificates[sid]
}

func (peer *peerImpl) putNodeEnrollmentCertificate(sid string, cert *x509.Certificate) {
	peer.nodeEnrollmentCertificatesMutex.Lock()
	defer peer.nodeEnrollmentCertificatesMutex.Unlock()
	peer.nodeEnrollmentCertificates[sid] = cert
}
