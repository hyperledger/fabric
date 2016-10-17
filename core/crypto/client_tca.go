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
	membersrvc "github.com/hyperledger/fabric/membersrvc/protos"

	"bytes"
	"crypto/ecdsa"
	"crypto/hmac"

	"errors"
	"fmt"

	"math/big"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"golang.org/x/net/context"
)

func (client *clientImpl) initTCertEngine() (err error) {
	// load TCertOwnerKDFKey
	if err = client.loadTCertOwnerKDFKey(); err != nil {
		return
	}

	// init TCerPool
	client.Debugf("Using multithreading [%t]", client.conf.IsMultithreadingEnabled())
	client.Debugf("TCert batch size [%d]", client.conf.getTCertBatchSize())

	if client.conf.IsMultithreadingEnabled() {
		client.tCertPool = new(tCertPoolMultithreadingImpl)
	} else {
		client.tCertPool = new(tCertPoolSingleThreadImpl)
	}

	if err = client.tCertPool.init(client); err != nil {
		client.Errorf("Failied inizializing TCertPool: [%s]", err)

		return
	}
	if err = client.tCertPool.Start(); err != nil {
		client.Errorf("Failied starting TCertPool: [%s]", err)

		return
	}
	return
}

func (client *clientImpl) storeTCertOwnerKDFKey() error {
	if err := client.ks.storeKey(client.conf.getTCertOwnerKDFKeyFilename(), client.tCertOwnerKDFKey); err != nil {
		client.Errorf("Failed storing TCertOwnerKDFKey [%s].", err.Error())

		return err
	}
	return nil
}

func (client *clientImpl) loadTCertOwnerKDFKey() error {
	// Load TCertOwnerKDFKey
	client.Debug("Loading TCertOwnerKDFKey...")

	if !client.ks.isAliasSet(client.conf.getTCertOwnerKDFKeyFilename()) {
		client.Debug("TCertOwnerKDFKey is missing, maybe the client has not requested any TCerts from TCA yet")

		return nil
	}

	tCertOwnerKDFKey, err := client.ks.loadKey(client.conf.getTCertOwnerKDFKeyFilename())
	if err != nil {
		client.Errorf("Failed parsing TCertOwnerKDFKey [%s].", err.Error())

		return err
	}
	client.tCertOwnerKDFKey = tCertOwnerKDFKey

	client.Debug("Loading TCertOwnerKDFKey...done!")

	return nil
}

func (client *clientImpl) getTCertFromExternalDER(der []byte) (tCert, error) {
	// DER to x509
	x509Cert, err := primitives.DERToX509Certificate(der)
	if err != nil {
		client.Errorf("Failed parsing certificate [% x]: [%s].", der, err)

		return nil, err
	}

	// Handle Critical Extension TCertEncTCertIndex
	tCertIndexCT, err := primitives.GetCriticalExtension(x509Cert, primitives.TCertEncTCertIndex)
	if err != nil {
		client.Errorf("Failed getting extension TCERT_ENC_TCERTINDEX [% x]: [%s].", der, err)

		return nil, err
	}

	// Handle Critical Extension TCertEncEnrollmentID TODO validate encEnrollmentID
	_, err = primitives.GetCriticalExtension(x509Cert, primitives.TCertEncEnrollmentID)
	if err != nil {
		client.Errorf("Failed getting extension TCERT_ENC_ENROLLMENT_ID [%s].", err.Error())

		return nil, err
	}

	// Handle Critical Extension TCertAttributes
	//	for i := 0; i < len(x509Cert.Extensions) - 2; i++ {
	//		attributeExtensionIdentifier := append(utils.TCertEncAttributesBase, i + 9)
	//		_ , err = utils.GetCriticalExtension(x509Cert, attributeExtensionIdentifier)
	//		if err != nil {
	//			client.Errorf("Failed getting extension TCERT_ATTRIBUTE_%s [%s].", i, err.Error())
	//
	//			return nil, err
	//		}
	//	}

	// Verify certificate against root
	if _, err := primitives.CheckCertAgainRoot(x509Cert, client.tcaCertPool); err != nil {
		client.Warningf("Warning verifing certificate [% x]: [%s].", der, err)

		return nil, err
	}

	// Try to extract the signing key from the TCert by decrypting the TCertIndex

	// 384-bit ExpansionValue = HMAC(Expansion_Key, TCertIndex)
	// Let TCertIndex = Timestamp, RandValue, 1,2,…
	// Timestamp assigned, RandValue assigned and counter reinitialized to 1 per batch
	// Decrypt ct to TCertIndex (TODO: || EnrollPub_Key || EnrollID ?)
	TCertOwnerEncryptKey := primitives.HMACAESTruncated(client.tCertOwnerKDFKey, []byte{1})
	ExpansionKey := primitives.HMAC(client.tCertOwnerKDFKey, []byte{2})
	pt, err := primitives.CBCPKCS7Decrypt(TCertOwnerEncryptKey, tCertIndexCT)

	if err == nil {
		// Compute ExpansionValue based on TCertIndex
		TCertIndex := pt
		//		TCertIndex := []byte(strconv.Itoa(i))

		// TODO: verify that TCertIndex has right format.

		client.Debugf("TCertIndex: [% x].", TCertIndex)
		mac := hmac.New(primitives.NewHash, ExpansionKey)
		mac.Write(TCertIndex)
		ExpansionValue := mac.Sum(nil)

		// Derive tpk and tsk accordingly to ExpansionValue from enrollment pk,sk
		// Computable by TCA / Auditor: TCertPub_Key = EnrollPub_Key + ExpansionValue G
		// using elliptic curve point addition per NIST FIPS PUB 186-4- specified P-384

		// Compute temporary secret key
		tempSK := &ecdsa.PrivateKey{
			PublicKey: ecdsa.PublicKey{
				Curve: client.enrollPrivKey.Curve,
				X:     new(big.Int),
				Y:     new(big.Int),
			},
			D: new(big.Int),
		}

		var k = new(big.Int).SetBytes(ExpansionValue)
		var one = new(big.Int).SetInt64(1)
		n := new(big.Int).Sub(client.enrollPrivKey.Params().N, one)
		k.Mod(k, n)
		k.Add(k, one)

		tempSK.D.Add(client.enrollPrivKey.D, k)
		tempSK.D.Mod(tempSK.D, client.enrollPrivKey.PublicKey.Params().N)

		// Compute temporary public key
		tempX, tempY := client.enrollPrivKey.PublicKey.ScalarBaseMult(k.Bytes())
		tempSK.PublicKey.X, tempSK.PublicKey.Y =
			tempSK.PublicKey.Add(
				client.enrollPrivKey.PublicKey.X, client.enrollPrivKey.PublicKey.Y,
				tempX, tempY,
			)

		// Verify temporary public key is a valid point on the reference curve
		isOn := tempSK.Curve.IsOnCurve(tempSK.PublicKey.X, tempSK.PublicKey.Y)
		if !isOn {
			client.Warning("Failed temporary public key IsOnCurve check. This is an foreign certificate.")

			return &tCertImpl{client, x509Cert, nil, []byte{}}, nil
		}

		// Check that the derived public key is the same as the one in the certificate
		certPK := x509Cert.PublicKey.(*ecdsa.PublicKey)

		if certPK.X.Cmp(tempSK.PublicKey.X) != 0 {
			client.Warning("Derived public key is different on X. This is an foreign certificate.")

			return &tCertImpl{client, x509Cert, nil, []byte{}}, nil
		}

		if certPK.Y.Cmp(tempSK.PublicKey.Y) != 0 {
			client.Warning("Derived public key is different on Y. This is an foreign certificate.")

			return &tCertImpl{client, x509Cert, nil, []byte{}}, nil
		}

		// Verify the signing capability of tempSK
		err = primitives.VerifySignCapability(tempSK, x509Cert.PublicKey)
		if err != nil {
			client.Warning("Failed verifing signing capability [%s]. This is an foreign certificate.", err.Error())

			return &tCertImpl{client, x509Cert, nil, []byte{}}, nil
		}

		// Marshall certificate and secret key to be stored in the database
		if err != nil {
			client.Warningf("Failed marshalling private key [%s]. This is an foreign certificate.", err.Error())

			return &tCertImpl{client, x509Cert, nil, []byte{}}, nil
		}

		if err = primitives.CheckCertPKAgainstSK(x509Cert, interface{}(tempSK)); err != nil {
			client.Warningf("Failed checking TCA cert PK against private key [%s]. This is an foreign certificate.", err.Error())

			return &tCertImpl{client, x509Cert, nil, []byte{}}, nil
		}

		return &tCertImpl{client, x509Cert, tempSK, []byte{}}, nil
	}
	client.Warningf("Failed decrypting extension TCERT_ENC_TCERTINDEX [%s]. This is an foreign certificate.", err.Error())
	return &tCertImpl{client, x509Cert, nil, []byte{}}, nil
}

func (client *clientImpl) getTCertFromDER(certBlk *TCertDBBlock) (certBlock *TCertBlock, err error) {
	if client.tCertOwnerKDFKey == nil {
		return nil, fmt.Errorf("KDF key not initialized yet")
	}

	TCertOwnerEncryptKey := primitives.HMACAESTruncated(client.tCertOwnerKDFKey, []byte{1})
	ExpansionKey := primitives.HMAC(client.tCertOwnerKDFKey, []byte{2})

	// DER to x509
	x509Cert, err := primitives.DERToX509Certificate(certBlk.tCertDER)
	if err != nil {
		client.Errorf("Failed parsing certificate [% x]: [%s].", certBlk.tCertDER, err)

		return
	}

	// Handle Critical Extenstion TCertEncTCertIndex
	tCertIndexCT, err := primitives.GetCriticalExtension(x509Cert, primitives.TCertEncTCertIndex)
	if err != nil {
		client.Errorf("Failed getting extension TCERT_ENC_TCERTINDEX [%v].", err.Error())

		return
	}

	// Verify certificate against root
	if _, err = primitives.CheckCertAgainRoot(x509Cert, client.tcaCertPool); err != nil {
		client.Warningf("Warning verifing certificate [%s].", err.Error())

		return
	}

	// Verify public key

	// 384-bit ExpansionValue = HMAC(Expansion_Key, TCertIndex)
	// Let TCertIndex = Timestamp, RandValue, 1,2,…
	// Timestamp assigned, RandValue assigned and counter reinitialized to 1 per batch

	// Decrypt ct to TCertIndex (TODO: || EnrollPub_Key || EnrollID ?)
	pt, err := primitives.CBCPKCS7Decrypt(TCertOwnerEncryptKey, tCertIndexCT)
	if err != nil {
		client.Errorf("Failed decrypting extension TCERT_ENC_TCERTINDEX [%s].", err.Error())

		return
	}

	// Compute ExpansionValue based on TCertIndex
	TCertIndex := pt
	//		TCertIndex := []byte(strconv.Itoa(i))

	client.Debugf("TCertIndex: [% x].", TCertIndex)
	mac := hmac.New(primitives.NewHash, ExpansionKey)
	mac.Write(TCertIndex)
	ExpansionValue := mac.Sum(nil)

	// Derive tpk and tsk accordingly to ExpansionValue from enrollment pk,sk
	// Computable by TCA / Auditor: TCertPub_Key = EnrollPub_Key + ExpansionValue G
	// using elliptic curve point addition per NIST FIPS PUB 186-4- specified P-384

	// Compute temporary secret key
	tempSK := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: client.enrollPrivKey.Curve,
			X:     new(big.Int),
			Y:     new(big.Int),
		},
		D: new(big.Int),
	}

	var k = new(big.Int).SetBytes(ExpansionValue)
	var one = new(big.Int).SetInt64(1)
	n := new(big.Int).Sub(client.enrollPrivKey.Params().N, one)
	k.Mod(k, n)
	k.Add(k, one)

	tempSK.D.Add(client.enrollPrivKey.D, k)
	tempSK.D.Mod(tempSK.D, client.enrollPrivKey.PublicKey.Params().N)

	// Compute temporary public key
	tempX, tempY := client.enrollPrivKey.PublicKey.ScalarBaseMult(k.Bytes())
	tempSK.PublicKey.X, tempSK.PublicKey.Y =
		tempSK.PublicKey.Add(
			client.enrollPrivKey.PublicKey.X, client.enrollPrivKey.PublicKey.Y,
			tempX, tempY,
		)

	// Verify temporary public key is a valid point on the reference curve
	isOn := tempSK.Curve.IsOnCurve(tempSK.PublicKey.X, tempSK.PublicKey.Y)
	if !isOn {
		client.Error("Failed temporary public key IsOnCurve check.")

		return nil, fmt.Errorf("Failed temporary public key IsOnCurve check.")
	}

	// Check that the derived public key is the same as the one in the certificate
	certPK := x509Cert.PublicKey.(*ecdsa.PublicKey)

	if certPK.X.Cmp(tempSK.PublicKey.X) != 0 {
		client.Error("Derived public key is different on X")

		return nil, fmt.Errorf("Derived public key is different on X")
	}

	if certPK.Y.Cmp(tempSK.PublicKey.Y) != 0 {
		client.Error("Derived public key is different on Y")

		return nil, fmt.Errorf("Derived public key is different on Y")
	}

	// Verify the signing capability of tempSK
	err = primitives.VerifySignCapability(tempSK, x509Cert.PublicKey)
	if err != nil {
		client.Errorf("Failed verifing signing capability [%s].", err.Error())

		return
	}

	// Marshall certificate and secret key to be stored in the database
	if err != nil {
		client.Errorf("Failed marshalling private key [%s].", err.Error())

		return
	}

	if err = primitives.CheckCertPKAgainstSK(x509Cert, interface{}(tempSK)); err != nil {
		client.Errorf("Failed checking TCA cert PK against private key [%s].", err.Error())

		return
	}

	certBlock = &TCertBlock{&tCertImpl{client, x509Cert, tempSK, certBlk.preK0}, certBlk.attributesHash}

	return
}

func (client *clientImpl) getTCertsFromTCA(attrhash string, attributes []string, num int) error {
	client.Debugf("Get [%d] certificates from the TCA...", num)

	// Contact the TCA
	TCertOwnerKDFKey, certDERs, err := client.callTCACreateCertificateSet(num, attributes)
	if err != nil {
		client.Errorf("Failed contacting TCA [%s].", err.Error())

		return err
	}

	//	client.debug("TCertOwnerKDFKey [%s].", utils.EncodeBase64(TCertOwnerKDFKey))

	// Store TCertOwnerKDFKey and checks that every time it is always the same key
	if client.tCertOwnerKDFKey != nil {
		// Check that the keys are the same
		equal := bytes.Equal(client.tCertOwnerKDFKey, TCertOwnerKDFKey)
		if !equal {
			return errors.New("Failed reciving kdf key from TCA. The keys are different.")
		}
	} else {
		client.tCertOwnerKDFKey = TCertOwnerKDFKey

		// TODO: handle this situation more carefully
		if err := client.storeTCertOwnerKDFKey(); err != nil {
			client.Errorf("Failed storing TCertOwnerKDFKey [%s].", err.Error())

			return err
		}
	}

	// Validate the Certificates obtained

	TCertOwnerEncryptKey := primitives.HMACAESTruncated(client.tCertOwnerKDFKey, []byte{1})
	ExpansionKey := primitives.HMAC(client.tCertOwnerKDFKey, []byte{2})

	j := 0
	for i := 0; i < num; i++ {
		// DER to x509
		x509Cert, err := primitives.DERToX509Certificate(certDERs[i].Cert)
		prek0 := certDERs[i].Prek0
		if err != nil {
			client.Errorf("Failed parsing certificate [% x]: [%s].", certDERs[i].Cert, err)

			continue
		}

		// Handle Critical Extenstion TCertEncTCertIndex
		tCertIndexCT, err := primitives.GetCriticalExtension(x509Cert, primitives.TCertEncTCertIndex)
		if err != nil {
			client.Errorf("Failed getting extension TCERT_ENC_TCERTINDEX [% x]: [%s].", primitives.TCertEncTCertIndex, err)

			continue
		}

		// Verify certificate against root
		if _, err := primitives.CheckCertAgainRoot(x509Cert, client.tcaCertPool); err != nil {
			client.Warningf("Warning verifing certificate [%s].", err.Error())

			continue
		}

		// Verify public key

		// 384-bit ExpansionValue = HMAC(Expansion_Key, TCertIndex)
		// Let TCertIndex = Timestamp, RandValue, 1,2,…
		// Timestamp assigned, RandValue assigned and counter reinitialized to 1 per batch

		// Decrypt ct to TCertIndex (TODO: || EnrollPub_Key || EnrollID ?)
		pt, err := primitives.CBCPKCS7Decrypt(TCertOwnerEncryptKey, tCertIndexCT)
		if err != nil {
			client.Errorf("Failed decrypting extension TCERT_ENC_TCERTINDEX [%s].", err.Error())

			continue
		}

		// Compute ExpansionValue based on TCertIndex
		TCertIndex := pt
		//		TCertIndex := []byte(strconv.Itoa(i))

		client.Debugf("TCertIndex: [% x].", TCertIndex)
		mac := hmac.New(primitives.NewHash, ExpansionKey)
		mac.Write(TCertIndex)
		ExpansionValue := mac.Sum(nil)

		// Derive tpk and tsk accordingly to ExpansionValue from enrollment pk,sk
		// Computable by TCA / Auditor: TCertPub_Key = EnrollPub_Key + ExpansionValue G
		// using elliptic curve point addition per NIST FIPS PUB 186-4- specified P-384

		// Compute temporary secret key
		tempSK := &ecdsa.PrivateKey{
			PublicKey: ecdsa.PublicKey{
				Curve: client.enrollPrivKey.Curve,
				X:     new(big.Int),
				Y:     new(big.Int),
			},
			D: new(big.Int),
		}

		var k = new(big.Int).SetBytes(ExpansionValue)
		var one = new(big.Int).SetInt64(1)
		n := new(big.Int).Sub(client.enrollPrivKey.Params().N, one)
		k.Mod(k, n)
		k.Add(k, one)

		tempSK.D.Add(client.enrollPrivKey.D, k)
		tempSK.D.Mod(tempSK.D, client.enrollPrivKey.PublicKey.Params().N)

		// Compute temporary public key
		tempX, tempY := client.enrollPrivKey.PublicKey.ScalarBaseMult(k.Bytes())
		tempSK.PublicKey.X, tempSK.PublicKey.Y =
			tempSK.PublicKey.Add(
				client.enrollPrivKey.PublicKey.X, client.enrollPrivKey.PublicKey.Y,
				tempX, tempY,
			)

		// Verify temporary public key is a valid point on the reference curve
		isOn := tempSK.Curve.IsOnCurve(tempSK.PublicKey.X, tempSK.PublicKey.Y)
		if !isOn {
			client.Error("Failed temporary public key IsOnCurve check.")

			continue
		}

		// Check that the derived public key is the same as the one in the certificate
		certPK := x509Cert.PublicKey.(*ecdsa.PublicKey)

		if certPK.X.Cmp(tempSK.PublicKey.X) != 0 {
			client.Error("Derived public key is different on X")

			continue
		}

		if certPK.Y.Cmp(tempSK.PublicKey.Y) != 0 {
			client.Error("Derived public key is different on Y")

			continue
		}

		// Verify the signing capability of tempSK
		err = primitives.VerifySignCapability(tempSK, x509Cert.PublicKey)
		if err != nil {
			client.Errorf("Failed verifing signing capability [%s].", err.Error())

			continue
		}

		// Marshall certificate and secret key to be stored in the database
		if err != nil {
			client.Errorf("Failed marshalling private key [%s].", err.Error())

			continue
		}

		if err := primitives.CheckCertPKAgainstSK(x509Cert, interface{}(tempSK)); err != nil {
			client.Errorf("Failed checking TCA cert PK against private key [%s].", err.Error())

			continue
		}

		client.Debugf("Sub index [%d]", j)
		j++
		client.Debugf("Certificate [%d] validated.", i)

		prek0Cp := make([]byte, len(prek0))
		copy(prek0Cp, prek0)

		tcertBlk := new(TCertBlock)

		tcertBlk.tCert = &tCertImpl{client, x509Cert, tempSK, prek0Cp}
		tcertBlk.attributesHash = attrhash

		client.tCertPool.AddTCert(tcertBlk)
	}

	if j == 0 {
		client.Error("No valid TCert was sent")

		return errors.New("No valid TCert was sent.")
	}

	return nil
}

func (client *clientImpl) callTCACreateCertificateSet(num int, attributes []string) ([]byte, []*membersrvc.TCert, error) {
	// Get a TCA Client
	sock, tcaP, err := client.getTCAClient()
	defer sock.Close()

	var attributesList []*membersrvc.TCertAttribute

	for _, k := range attributes {
		tcertAttr := new(membersrvc.TCertAttribute)
		tcertAttr.AttributeName = k
		attributesList = append(attributesList, tcertAttr)
	}

	// Execute the protocol
	now := time.Now()
	timestamp := timestamp.Timestamp{Seconds: int64(now.Second()), Nanos: int32(now.Nanosecond())}
	req := &membersrvc.TCertCreateSetReq{
		Ts:         &timestamp,
		Id:         &membersrvc.Identity{Id: client.enrollID},
		Num:        uint32(num),
		Attributes: attributesList,
		Sig:        nil,
	}

	rawReq, err := proto.Marshal(req)
	if err != nil {
		client.Errorf("Failed marshaling request [%s].", err.Error())
		return nil, nil, err
	}

	// 2. Sign rawReq
	r, s, err := client.ecdsaSignWithEnrollmentKey(rawReq)
	if err != nil {
		client.Errorf("Failed creating signature for [% x]: [%s].", rawReq, err.Error())
		return nil, nil, err
	}

	R, _ := r.MarshalText()
	S, _ := s.MarshalText()

	// 3. Append the signature
	req.Sig = &membersrvc.Signature{Type: membersrvc.CryptoType_ECDSA, R: R, S: S}

	// 4. Send request
	certSet, err := tcaP.CreateCertificateSet(context.Background(), req)
	if err != nil {
		client.Errorf("Failed requesting tca create certificate set [%s].", err.Error())

		return nil, nil, err
	}

	return certSet.Certs.Key, certSet.Certs.Certs, nil
}
