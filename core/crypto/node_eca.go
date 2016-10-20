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
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"time"

	membersrvc "github.com/hyperledger/fabric/membersrvc/protos"

	"encoding/asn1"
	"errors"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/primitives/ecies"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	// ECertSubjectRole is the ASN1 object identifier of the subject's role.
	ECertSubjectRole = asn1.ObjectIdentifier{2, 1, 3, 4, 5, 6, 7}
)

func (node *nodeImpl) retrieveECACertsChain(userID string) error {
	if !node.ks.certMissing(node.conf.getECACertsChainFilename()) {
		return nil
	}

	// Retrieve ECA certificate and verify it
	ecaCertRaw, err := node.getECACertificate()
	if err != nil {
		node.Errorf("Failed getting ECA certificate [%s].", err.Error())

		return err
	}
	node.Debugf("ECA certificate [% x].", ecaCertRaw)

	// TODO: Test ECA cert againt root CA
	// TODO: check response.Cert against rootCA
	x509ECACert, err := primitives.DERToX509Certificate(ecaCertRaw)
	if err != nil {
		node.Errorf("Failed parsing ECA certificate [%s].", err.Error())

		return err
	}

	// Prepare ecaCertPool
	node.ecaCertPool = x509.NewCertPool()
	node.ecaCertPool.AddCert(x509ECACert)

	// Store ECA cert
	node.Debugf("Storing ECA certificate for [%s]...", userID)

	if err := node.ks.storeCert(node.conf.getECACertsChainFilename(), ecaCertRaw); err != nil {
		node.Errorf("Failed storing eca certificate [%s].", err.Error())
		return err
	}

	return nil
}

func (node *nodeImpl) retrieveEnrollmentData(enrollID, enrollPWD string) error {
	if !node.ks.certMissing(node.conf.getEnrollmentCertFilename()) {
		return nil
	}

	key, enrollCertRaw, enrollChainKey, err := node.getEnrollmentCertificateFromECA(enrollID, enrollPWD)
	if err != nil {
		node.Errorf("Failed getting enrollment certificate [id=%s]: [%s]", enrollID, err)

		return err
	}
	node.Debugf("Enrollment certificate [% x].", enrollCertRaw)

	node.Debugf("Storing enrollment data for user [%s]...", enrollID)

	// Store enrollment id
	err = ioutil.WriteFile(node.conf.getEnrollmentIDPath(), []byte(enrollID), 0700)
	if err != nil {
		node.Errorf("Failed storing enrollment certificate [id=%s]: [%s]", enrollID, err)
		return err
	}

	// Store enrollment key
	if err := node.ks.storePrivateKey(node.conf.getEnrollmentKeyFilename(), key); err != nil {
		node.Errorf("Failed storing enrollment key [id=%s]: [%s]", enrollID, err)
		return err
	}

	// Store enrollment cert
	if err := node.ks.storeCert(node.conf.getEnrollmentCertFilename(), enrollCertRaw); err != nil {
		node.Errorf("Failed storing enrollment certificate [id=%s]: [%s]", enrollID, err)
		return err
	}

	// Code for confidentiality 1.2
	// Store enrollment chain key
	if node.eType == NodeValidator {
		node.Debugf("Enrollment chain key for validator [%s]...", enrollID)
		// enrollChainKey is a secret key

		node.Debugf("key [%s]...", string(enrollChainKey))

		key, err := primitives.PEMtoPrivateKey(enrollChainKey, nil)
		if err != nil {
			node.Errorf("Failed unmarshalling enrollment chain key [id=%s]: [%s]", enrollID, err)
			return err
		}

		if err := node.ks.storePrivateKey(node.conf.getEnrollmentChainKeyFilename(), key); err != nil {
			node.Errorf("Failed storing enrollment chain key [id=%s]: [%s]", enrollID, err)
			return err
		}
	} else {
		node.Debugf("Enrollment chain key for non-validator [%s]...", enrollID)
		// enrollChainKey is a public key

		key, err := primitives.PEMtoPublicKey(enrollChainKey, nil)
		if err != nil {
			node.Errorf("Failed unmarshalling enrollment chain key [id=%s]: [%s]", enrollID, err)
			return err
		}
		node.Debugf("Key decoded from PEM [%s]...", enrollID)

		if err := node.ks.storePublicKey(node.conf.getEnrollmentChainKeyFilename(), key); err != nil {
			node.Errorf("Failed storing enrollment chain key [id=%s]: [%s]", enrollID, err)
			return err
		}
	}

	return nil
}

func (node *nodeImpl) loadEnrollmentKey() error {
	node.Debug("Loading enrollment key...")

	enrollPrivKey, err := node.ks.loadPrivateKey(node.conf.getEnrollmentKeyFilename())
	if err != nil {
		node.Errorf("Failed loading enrollment private key [%s].", err.Error())

		return err
	}

	node.enrollPrivKey = enrollPrivKey.(*ecdsa.PrivateKey)

	return nil
}

func (node *nodeImpl) loadEnrollmentCertificate() error {
	node.Debug("Loading enrollment certificate...")

	cert, der, err := node.ks.loadCertX509AndDer(node.conf.getEnrollmentCertFilename())
	if err != nil {
		node.Errorf("Failed parsing enrollment certificate [%s].", err.Error())

		return err
	}
	node.enrollCert = cert

	// TODO: move this to retrieve
	pk := node.enrollCert.PublicKey.(*ecdsa.PublicKey)
	err = primitives.VerifySignCapability(node.enrollPrivKey, pk)
	if err != nil {
		node.Errorf("Failed checking enrollment certificate against enrollment key [%s].", err.Error())

		return err
	}

	// Set node ID
	node.id = primitives.Hash(der)
	node.Debugf("Setting id to [% x].", node.id)

	// Set eCertHash
	node.enrollCertHash = primitives.Hash(der)
	node.Debugf("Setting enrollCertHash to [% x].", node.enrollCertHash)

	return nil
}

func (node *nodeImpl) loadEnrollmentID() error {
	node.Debugf("Loading enrollment id at [%s]...", node.conf.getEnrollmentIDPath())

	enrollID, err := ioutil.ReadFile(node.conf.getEnrollmentIDPath())
	if err != nil {
		node.Errorf("Failed loading enrollment id [%s].", err.Error())

		return err
	}

	// Set enrollment ID
	node.enrollID = string(enrollID)
	node.Debugf("Setting enrollment id to [%s].", node.enrollID)

	return nil
}

func (node *nodeImpl) loadEnrollmentChainKey() error {
	node.Debug("Loading enrollment chain key...")

	// Code for confidentiality 1.2
	if node.eType == NodeValidator {
		// enrollChainKey is a secret key
		enrollChainKey, err := node.ks.loadPrivateKey(node.conf.getEnrollmentChainKeyFilename())
		if err != nil {
			node.Errorf("Failed loading enrollment chain key: [%s]", err)
			return err
		}
		node.enrollChainKey = enrollChainKey
	} else {
		// enrollChainKey is a public key
		enrollChainKey, err := node.ks.loadPublicKey(node.conf.getEnrollmentChainKeyFilename())
		if err != nil {
			node.Errorf("Failed load enrollment chain key: [%s]", err)
			return err
		}
		node.enrollChainKey = enrollChainKey
	}

	return nil
}

func (node *nodeImpl) loadECACertsChain() error {
	node.Debug("Loading ECA certificates chain...")

	pem, err := node.ks.loadCert(node.conf.getECACertsChainFilename())
	if err != nil {
		node.Errorf("Failed loading ECA certificates chain [%s].", err.Error())

		return err
	}

	ok := node.ecaCertPool.AppendCertsFromPEM(pem)
	if !ok {
		node.Error("Failed appending ECA certificates chain.")

		return errors.New("Failed appending ECA certificates chain.")
	}

	return nil
}

func (node *nodeImpl) getECAClient() (*grpc.ClientConn, membersrvc.ECAPClient, error) {
	node.Debug("Getting ECA client...")

	conn, err := node.getClientConn(node.conf.getECAPAddr(), node.conf.getECAServerName())
	if err != nil {
		node.Errorf("Failed getting client connection: [%s]", err)
	}

	client := membersrvc.NewECAPClient(conn)

	node.Debug("Getting ECA client...done")

	return conn, client, nil
}

func (node *nodeImpl) callECAReadCACertificate(ctx context.Context, opts ...grpc.CallOption) (*membersrvc.Cert, error) {
	// Get an ECA Client
	sock, ecaP, err := node.getECAClient()
	defer sock.Close()

	// Issue the request
	cert, err := ecaP.ReadCACertificate(ctx, &membersrvc.Empty{}, opts...)
	if err != nil {
		node.Errorf("Failed requesting read certificate [%s].", err.Error())

		return nil, err
	}

	return cert, nil
}

func (node *nodeImpl) callECAReadCertificate(ctx context.Context, in *membersrvc.ECertReadReq, opts ...grpc.CallOption) (*membersrvc.CertPair, error) {
	// Get an ECA Client
	sock, ecaP, err := node.getECAClient()
	defer sock.Close()

	// Issue the request
	resp, err := ecaP.ReadCertificatePair(ctx, in, opts...)
	if err != nil {
		node.Errorf("Failed requesting read certificate [%s].", err.Error())

		return nil, err
	}

	return resp, nil
}

func (node *nodeImpl) callECAReadCertificateByHash(ctx context.Context, in *membersrvc.Hash, opts ...grpc.CallOption) (*membersrvc.CertPair, error) {
	// Get an ECA Client
	sock, ecaP, err := node.getECAClient()
	defer sock.Close()

	// Issue the request
	resp, err := ecaP.ReadCertificateByHash(ctx, in, opts...)
	if err != nil {
		node.Errorf("Failed requesting read certificate [%s].", err.Error())

		return nil, err
	}

	return &membersrvc.CertPair{Sign: resp.Cert, Enc: nil}, nil
}

func (node *nodeImpl) getEnrollmentCertificateFromECA(id, pw string) (interface{}, []byte, []byte, error) {
	// Get a new ECA Client
	sock, ecaP, err := node.getECAClient()
	defer sock.Close()

	// Run the protocol

	signPriv, err := primitives.NewECDSAKey()
	if err != nil {
		node.Errorf("Failed generating ECDSA key [%s].", err.Error())

		return nil, nil, nil, err
	}
	signPub, err := x509.MarshalPKIXPublicKey(&signPriv.PublicKey)
	if err != nil {
		node.Errorf("Failed mashalling ECDSA key [%s].", err.Error())

		return nil, nil, nil, err
	}

	encPriv, err := primitives.NewECDSAKey()
	if err != nil {
		node.Errorf("Failed generating Encryption key [%s].", err.Error())

		return nil, nil, nil, err
	}
	encPub, err := x509.MarshalPKIXPublicKey(&encPriv.PublicKey)
	if err != nil {
		node.Errorf("Failed marshalling Encryption key [%s].", err.Error())

		return nil, nil, nil, err
	}

	req := &membersrvc.ECertCreateReq{
		Ts:   &timestamp.Timestamp{Seconds: time.Now().Unix(), Nanos: 0},
		Id:   &membersrvc.Identity{Id: id},
		Tok:  &membersrvc.Token{Tok: []byte(pw)},
		Sign: &membersrvc.PublicKey{Type: membersrvc.CryptoType_ECDSA, Key: signPub},
		Enc:  &membersrvc.PublicKey{Type: membersrvc.CryptoType_ECDSA, Key: encPub},
		Sig:  nil}

	resp, err := ecaP.CreateCertificatePair(context.Background(), req)
	if err != nil {
		node.Errorf("Failed invoking CreateCertficatePair [%s].", err.Error())

		return nil, nil, nil, err
	}

	if resp.FetchResult != nil && resp.FetchResult.Status != membersrvc.FetchAttrsResult_SUCCESS {
		node.Warning(resp.FetchResult.Msg)
	}
	//out, err := rsa.DecryptPKCS1v15(rand.Reader, encPriv, resp.Tok.Tok)
	spi := ecies.NewSPI()
	eciesKey, err := spi.NewPrivateKey(nil, encPriv)
	if err != nil {
		node.Errorf("Failed parsing decrypting key [%s].", err.Error())

		return nil, nil, nil, err
	}

	ecies, err := spi.NewAsymmetricCipherFromPublicKey(eciesKey)
	if err != nil {
		node.Errorf("Failed creating asymmetrinc cipher [%s].", err.Error())

		return nil, nil, nil, err
	}

	out, err := ecies.Process(resp.Tok.Tok)
	if err != nil {
		node.Errorf("Failed decrypting toke [%s].", err.Error())

		return nil, nil, nil, err
	}

	req.Tok.Tok = out
	req.Sig = nil

	hash := primitives.NewHash()
	raw, _ := proto.Marshal(req)
	hash.Write(raw)

	r, s, err := ecdsa.Sign(rand.Reader, signPriv, hash.Sum(nil))
	if err != nil {
		node.Errorf("Failed signing [%s].", err.Error())

		return nil, nil, nil, err
	}
	R, _ := r.MarshalText()
	S, _ := s.MarshalText()
	req.Sig = &membersrvc.Signature{Type: membersrvc.CryptoType_ECDSA, R: R, S: S}

	resp, err = ecaP.CreateCertificatePair(context.Background(), req)
	if err != nil {
		node.Errorf("Failed invoking CreateCertificatePair [%s].", err.Error())

		return nil, nil, nil, err
	}

	// Verify response

	// Verify cert for signing
	node.Debugf("Enrollment certificate for signing [% x]", primitives.Hash(resp.Certs.Sign))

	x509SignCert, err := primitives.DERToX509Certificate(resp.Certs.Sign)
	if err != nil {
		node.Errorf("Failed parsing signing enrollment certificate for signing: [%s]", err)

		return nil, nil, nil, err
	}

	_, err = primitives.GetCriticalExtension(x509SignCert, ECertSubjectRole)
	if err != nil {
		node.Errorf("Failed parsing ECertSubjectRole in enrollment certificate for signing: [%s]", err)

		return nil, nil, nil, err
	}

	err = primitives.CheckCertAgainstSKAndRoot(x509SignCert, signPriv, node.ecaCertPool)
	if err != nil {
		node.Errorf("Failed checking signing enrollment certificate for signing: [%s]", err)

		return nil, nil, nil, err
	}

	// Verify cert for encrypting
	node.Debugf("Enrollment certificate for encrypting [% x]", primitives.Hash(resp.Certs.Enc))

	x509EncCert, err := primitives.DERToX509Certificate(resp.Certs.Enc)
	if err != nil {
		node.Errorf("Failed parsing signing enrollment certificate for encrypting: [%s]", err)

		return nil, nil, nil, err
	}

	_, err = primitives.GetCriticalExtension(x509EncCert, ECertSubjectRole)
	if err != nil {
		node.Errorf("Failed parsing ECertSubjectRole in enrollment certificate for encrypting: [%s]", err)

		return nil, nil, nil, err
	}

	err = primitives.CheckCertAgainstSKAndRoot(x509EncCert, encPriv, node.ecaCertPool)
	if err != nil {
		node.Errorf("Failed checking signing enrollment certificate for encrypting: [%s]", err)

		return nil, nil, nil, err
	}

	return signPriv, resp.Certs.Sign, resp.Pkchain, nil
}

func (node *nodeImpl) getECACertificate() ([]byte, error) {
	responce, err := node.callECAReadCACertificate(context.Background())
	if err != nil {
		node.Errorf("Failed requesting ECA certificate [%s].", err.Error())

		return nil, err
	}

	return responce.Cert, nil
}
