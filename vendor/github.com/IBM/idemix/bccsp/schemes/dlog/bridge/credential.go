/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"bytes"

	bccsp "github.com/IBM/idemix/bccsp/schemes"
	idemix "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	"github.com/IBM/idemix/bccsp/schemes/dlog/handlers"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Credential encapsulates the idemix algorithms to produce (sign) a credential
// and verify it. Recall that a credential is produced by the Issuer upon a credential request,
// and it is verified by the requester.
type Credential struct {
	Translator idemix.Translator
	Idemix     *idemix.Idemix
}

// Sign produces an idemix credential. It takes in input the issuer secret key,
// a serialised  credential request, and a list of attribute values.
// Notice that attributes should not contain attributes whose type is IdemixHiddenAttribute
// cause the credential needs to carry all the attribute values.
func (c *Credential) Sign(key handlers.IssuerSecretKey, credentialRequest []byte, attributes []bccsp.IdemixAttribute) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iisk, ok := key.(*IssuerSecretKey)
	if !ok {
		return nil, errors.Errorf("invalid issuer secret key, expected *Big, got [%T]", key)
	}

	cr := &idemix.CredRequest{}
	err = proto.Unmarshal(credentialRequest, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling credential request")
	}

	attrValues := make([]*math.Zr, len(attributes))
	for i := 0; i < len(attributes); i++ {
		switch attributes[i].Type {
		case bccsp.IdemixBytesAttribute:
			attrValues[i] = c.Idemix.Curve.HashToZr(attributes[i].Value.([]byte))
		case bccsp.IdemixIntAttribute:
			var value int64
			if v, ok := attributes[i].Value.(int); ok {
				value = int64(v)
			} else if v, ok := attributes[i].Value.(int64); ok {
				value = v
			} else {
				return nil, errors.Errorf("invalid int type for IdemixIntAttribute attribute")
			}
			attrValues[i] = c.Idemix.Curve.NewZrFromInt(value)
		default:
			return nil, errors.Errorf("attribute type not allowed or supported [%v] at position [%d]", attributes[i].Type, i)
		}
	}

	cred, err := c.Idemix.NewCredential(iisk.SK, cr, attrValues, newRandOrPanic(c.Idemix.Curve), c.Translator)
	if err != nil {
		return nil, errors.WithMessage(err, "failed creating new credential")
	}

	return proto.Marshal(cred)
}

// Verify checks that an idemix credential is cryptographically correct. It takes
// in input the user secret key (sk), the issuer public key (ipk), the serialised credential (credential),
// and a list of attributes. The list of attributes is optional, in case it is specified, Verify
// checks that the credential carries the specified attributes.
func (c *Credential) Verify(sk *math.Zr, ipk handlers.IssuerPublicKey, credential []byte, attributes []bccsp.IdemixAttribute) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", sk)
	}

	cred := &idemix.Credential{}
	err = proto.Unmarshal(credential, cred)
	if err != nil {
		return err
	}

	for i := 0; i < len(attributes); i++ {
		switch attributes[i].Type {
		case bccsp.IdemixBytesAttribute:
			if !bytes.Equal(
				c.Idemix.Curve.HashToZr(attributes[i].Value.([]byte)).Bytes(),
				cred.Attrs[i]) {
				return errors.Errorf("credential does not contain the correct attribute value at position [%d]", i)
			}
		case bccsp.IdemixIntAttribute:
			var value int64
			if v, ok := attributes[i].Value.(int); ok {
				value = int64(v)
			} else if v, ok := attributes[i].Value.(int64); ok {
				value = v
			} else {
				return errors.Errorf("invalid int type for IdemixIntAttribute attribute")
			}

			if !bytes.Equal(
				c.Idemix.Curve.NewZrFromInt(value).Bytes(),
				cred.Attrs[i]) {
				return errors.Errorf("credential does not contain the correct attribute value at position [%d]", i)
			}
		case bccsp.IdemixHiddenAttribute:
			continue
		default:
			return errors.Errorf("attribute type not allowed or supported [%v] at position [%d]", attributes[i].Type, i)
		}
	}

	return cred.Ver(sk, iipk.PK, c.Idemix.Curve, c.Translator)
}
