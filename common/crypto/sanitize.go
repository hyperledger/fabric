/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"crypto/ecdsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/pkg/errors"
)

// SanitizeIdentity sanitizes the signature scheme of the identity
func SanitizeIdentity(identity []byte) ([]byte, error) {
	sID := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(identity, sID); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling identity %s", string(identity))
	}

	finalPEM, err := SanitizeX509Cert(sID.IdBytes)
	if err != nil {
		return nil, err
	}

	sID.IdBytes = finalPEM

	return proto.Marshal(sID)
}

func SanitizeX509Cert(initialPEM []byte) ([]byte, error) {
	der, _ := pem.Decode(initialPEM)
	if der == nil {
		return nil, errors.Errorf("failed to PEM decode identity bytes: %s", string(initialPEM))
	}
	cert, err := x509.ParseCertificate(der.Bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing certificate %s", string(initialPEM))
	}

	r, s, err := utils.UnmarshalECDSASignature(cert.Signature)
	if err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling ECDSA signature on identity: %s", string(initialPEM))
	}

	// We assume that the consenter and the CA use the same signature scheme.
	curveOrderUsedByCryptoGen := cert.PublicKey.(*ecdsa.PublicKey).Curve.Params().N
	halfOrder := new(big.Int).Rsh(curveOrderUsedByCryptoGen, 1)
	// Low S, nothing to do here!
	if s.Cmp(halfOrder) != 1 {
		return initialPEM, nil
	}
	// Else it's high-S, so shift it below half the order.
	s.Sub(curveOrderUsedByCryptoGen, s)

	var newCert certificate
	_, err = asn1.Unmarshal(cert.Raw, &newCert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling certificate")
	}

	newSig, err := utils.MarshalECDSASignature(r, s)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling ECDSA signature")
	}
	newCert.SignatureValue = asn1.BitString{Bytes: newSig, BitLength: len(newSig) * 8}

	newCert.Raw = nil
	newRaw, err := asn1.Marshal(newCert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling new certificate")
	}

	finalPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: newRaw})
	return finalPEM, nil
}

type certificate struct {
	Raw                asn1.RawContent
	TBSCertificate     tbsCertificate
	SignatureAlgorithm pkix.AlgorithmIdentifier
	SignatureValue     asn1.BitString
}

type tbsCertificate struct {
	Raw                asn1.RawContent
	Version            int `asn1:"optional,explicit,default:0,tag:0"`
	SerialNumber       *big.Int
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Issuer             asn1.RawValue
	Validity           validity
	Subject            asn1.RawValue
	PublicKey          publicKeyInfo
	UniqueId           asn1.BitString   `asn1:"optional,tag:1"`
	SubjectUniqueId    asn1.BitString   `asn1:"optional,tag:2"`
	Extensions         []pkix.Extension `asn1:"optional,explicit,tag:3"`
}

type validity struct {
	NotBefore, NotAfter time.Time
}

type publicKeyInfo struct {
	Raw       asn1.RawContent
	Algorithm pkix.AlgorithmIdentifier
	PublicKey asn1.BitString
}
