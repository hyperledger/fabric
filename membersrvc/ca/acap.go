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
	"encoding/asn1"
	"errors"
	"math/big"

	"crypto/ecdsa"
	"crypto/x509"
	"crypto/x509/pkix"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	pb "github.com/hyperledger/fabric/membersrvc/protos"
)

var acapLogger = logging.MustGetLogger("acap")

// ACAP serves the public GRPC interface of the ACA.
//
type ACAP struct {
	aca *ACA
}

// FetchAttributes fetchs the attributes from the outside world and populate them into the database.
func (acap *ACAP) FetchAttributes(ctx context.Context, in *pb.ACAFetchAttrReq) (*pb.ACAFetchAttrResp, error) {
	acapLogger.Debug("grpc ACAP:FetchAttributes")

	if in.Ts == nil || in.ECert == nil || in.Signature == nil {
		return &pb.ACAFetchAttrResp{Status: pb.ACAFetchAttrResp_FAILURE, Msg: "Bad request"}, nil
	}

	cert, err := acap.aca.getECACertificate()
	if err != nil {
		return &pb.ACAFetchAttrResp{Status: pb.ACAFetchAttrResp_FAILURE}, errors.New("Error getting ECA certificate.")
	}

	ecaPub := cert.PublicKey.(*ecdsa.PublicKey)
	r, s := big.NewInt(0), big.NewInt(0)
	r.UnmarshalText(in.Signature.R)
	s.UnmarshalText(in.Signature.S)

	in.Signature = nil

	hash := primitives.NewHash()
	raw, _ := proto.Marshal(in)
	hash.Write(raw)

	if ecdsa.Verify(ecaPub, hash.Sum(nil), r, s) == false {
		return &pb.ACAFetchAttrResp{Status: pb.ACAFetchAttrResp_FAILURE, Msg: "Signature does not verify"}, nil
	}

	cert, err = x509.ParseCertificate(in.ECert.Cert)
	if err != nil {
		return &pb.ACAFetchAttrResp{Status: pb.ACAFetchAttrResp_FAILURE}, err
	}

	var id, affiliation string
	id, affiliation, err = acap.aca.parseEnrollID(cert.Subject.CommonName)
	if err != nil {
		return &pb.ACAFetchAttrResp{Status: pb.ACAFetchAttrResp_FAILURE}, err
	}

	err = acap.aca.fetchAndPopulateAttributes(id, affiliation)
	if err != nil {
		return &pb.ACAFetchAttrResp{Status: pb.ACAFetchAttrResp_FAILURE}, err
	}

	return &pb.ACAFetchAttrResp{Status: pb.ACAFetchAttrResp_SUCCESS}, nil
}

func (acap *ACAP) createRequestAttributeResponse(status pb.ACAAttrResp_StatusCode, cert *pb.Cert) *pb.ACAAttrResp {
	resp := &pb.ACAAttrResp{Status: status, Cert: cert, Signature: nil}
	rawReq, err := proto.Marshal(resp)
	if err != nil {
		return &pb.ACAAttrResp{Status: pb.ACAAttrResp_FAILURE, Cert: nil, Signature: nil}
	}

	r, s, err := primitives.ECDSASignDirect(acap.aca.priv, rawReq)
	if err != nil {
		return &pb.ACAAttrResp{Status: pb.ACAAttrResp_FAILURE, Cert: nil, Signature: nil}
	}

	R, _ := r.MarshalText()
	S, _ := s.MarshalText()

	resp.Signature = &pb.Signature{Type: pb.CryptoType_ECDSA, R: R, S: S}

	return resp
}

// RequestAttributes lookups the atributes in the database and return a certificate with attributes included in the request and found in the database.
func (acap *ACAP) RequestAttributes(ctx context.Context, in *pb.ACAAttrReq) (*pb.ACAAttrResp, error) {
	acapLogger.Debug("grpc ACAP:RequestAttributes")

	fail := pb.ACAAttrResp_FULL_SUCCESSFUL // else explicit which-param-failed error
	if nil == in.Ts {
		fail = pb.ACAAttrResp_FAIL_NIL_TS
	} else if nil == in.Id {
		fail = pb.ACAAttrResp_FAIL_NIL_ID
	} else if nil == in.ECert {
		fail = pb.ACAAttrResp_FAIL_NIL_ECERT
	} else if nil == in.Signature {
		fail = pb.ACAAttrResp_FAIL_NIL_SIGNATURE
	}

	if pb.ACAAttrResp_FULL_SUCCESSFUL != fail {
		return acap.createRequestAttributeResponse(fail, nil), nil
	}

	if in.Attributes == nil {
		in.Attributes = []*pb.TCertAttribute{}
	}

	attrs := make(map[string]bool)
	for _, attrPair := range in.Attributes {
		if attrs[attrPair.AttributeName] {
			return acap.createRequestAttributeResponse(pb.ACAAttrResp_BAD_REQUEST, nil), nil
		}
		attrs[attrPair.AttributeName] = true
	}

	cert, err := acap.aca.getTCACertificate()
	if err != nil {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_FAILURE, nil), errors.New("Error getting TCA certificate.")
	}

	tcaPub := cert.PublicKey.(*ecdsa.PublicKey)
	r, s := big.NewInt(0), big.NewInt(0)
	r.UnmarshalText(in.Signature.R)
	s.UnmarshalText(in.Signature.S)

	in.Signature = nil

	hash := primitives.NewHash()
	raw, _ := proto.Marshal(in)
	hash.Write(raw)
	if ecdsa.Verify(tcaPub, hash.Sum(nil), r, s) == false {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_FAILURE, nil), errors.New("Signature does not verify")
	}

	cert, err = x509.ParseCertificate(in.ECert.Cert)

	if err != nil {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_FAILURE, nil), err
	}
	var id, affiliation string
	id, affiliation, err = acap.aca.parseEnrollID(cert.Subject.CommonName)
	if err != nil {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_FAILURE, nil), err
	}
	//Before continue with the request we perform a refresh of the attributes.
	err = acap.aca.fetchAndPopulateAttributes(id, affiliation)
	if err != nil {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_FAILURE, nil), err
	}

	var verifyCounter int
	var attributes = make([]AttributePair, 0)
	owner := &AttributeOwner{id, affiliation}
	for _, attrPair := range in.Attributes {
		verifiedPair, _ := acap.aca.findAttribute(owner, attrPair.AttributeName)
		if verifiedPair != nil {
			verifyCounter++
			attributes = append(attributes, *verifiedPair)
		}
	}

	var extensions = make([]pkix.Extension, 0)
	extensions, err = acap.addAttributesToExtensions(&attributes, extensions)
	if err != nil {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_FAILURE, nil), err
	}

	spec := NewDefaultCertificateSpec(id, cert.PublicKey, cert.KeyUsage, extensions...)
	raw, err = acap.aca.newCertificateFromSpec(spec)
	if err != nil {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_FAILURE, nil), err
	}

	if verifyCounter == 0 {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_NO_ATTRIBUTES_FOUND, &pb.Cert{Cert: raw}), nil
	}

	count := len(in.Attributes)

	if count == verifyCounter {
		return acap.createRequestAttributeResponse(pb.ACAAttrResp_FULL_SUCCESSFUL, &pb.Cert{Cert: raw}), nil
	}
	return acap.createRequestAttributeResponse(pb.ACAAttrResp_PARTIAL_SUCCESSFUL, &pb.Cert{Cert: raw}), nil
}

func (acap *ACAP) addAttributesToExtensions(attributes *[]AttributePair, extensions []pkix.Extension) ([]pkix.Extension, error) {
	count := 11
	exts := extensions
	for _, a := range *attributes {
		//Save the position of the attribute extension on the header.
		att := a.ToACAAttribute()
		raw, err := proto.Marshal(att)
		if err != nil {
			continue
		}
		exts = append(exts, pkix.Extension{Id: asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, count}, Critical: false, Value: raw})
		count++
	}
	return exts, nil
}

// ReadCACertificate reads the certificate of the ACA.
//
func (acap *ACAP) ReadCACertificate(ctx context.Context, in *pb.Empty) (*pb.Cert, error) {
	acapLogger.Debug("grpc ACAP:ReadCACertificate")

	return &pb.Cert{Cert: acap.aca.raw}, nil
}
