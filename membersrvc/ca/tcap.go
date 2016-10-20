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
	"crypto/hmac"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/crypto/attributes"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/membersrvc/protos"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"

	"github.com/golang/protobuf/ptypes/timestamp"
)

var tcapLogger = logging.MustGetLogger("tcap")

// TCAP serves the public GRPC interface of the TCA.
type TCAP struct {
	tca *TCA
}

// ReadCACertificate reads the certificate of the TCA.
func (tcap *TCAP) ReadCACertificate(ctx context.Context, in *pb.Empty) (*pb.Cert, error) {
	tcapLogger.Debugf("grpc TCAP:ReadCACertificate")

	return &pb.Cert{Cert: tcap.tca.raw}, nil
}

func (tcap *TCAP) selectValidAttributes(certRaw []byte) ([]*pb.ACAAttribute, error) {
	cert, err := x509.ParseCertificate(certRaw)
	if err != nil {
		return nil, err
	}

	var ans []*pb.ACAAttribute

	if cert.Extensions == nil {
		return ans, nil
	}
	currentTime := time.Now()
	for _, extension := range cert.Extensions {
		acaAtt := &pb.ACAAttribute{AttributeName: "", AttributeValue: nil, ValidFrom: &timestamp.Timestamp{Seconds: 0, Nanos: 0}, ValidTo: &timestamp.Timestamp{Seconds: 0, Nanos: 0}}

		if IsAttributeOID(extension.Id) {
			if err := proto.Unmarshal(extension.Value, acaAtt); err != nil {
				continue
			}

			if acaAtt.AttributeName == "" {
				continue
			}
			var from, to time.Time
			if acaAtt.ValidFrom != nil {
				from = time.Unix(acaAtt.ValidFrom.Seconds, int64(acaAtt.ValidFrom.Nanos))
			}
			if acaAtt.ValidTo != nil {
				to = time.Unix(acaAtt.ValidTo.Seconds, int64(acaAtt.ValidTo.Nanos))
			}

			//Check if the attribute still being valid.
			if (from.Before(currentTime) || from.Equal(currentTime)) && (to.IsZero() || to.After(currentTime)) {
				ans = append(ans, acaAtt)
			}
		}
	}
	return ans, nil
}

func (tcap *TCAP) requestAttributes(id string, ecert []byte, attrs []*pb.TCertAttribute) ([]*pb.ACAAttribute, error) {
	//TODO we are creation a new client connection per each ecer request. We should be implement a connections pool.
	sock, acaP, err := GetACAClient()
	if err != nil {
		return nil, err
	}
	defer sock.Close()
	var attrNames []*pb.TCertAttribute

	for _, att := range attrs {
		attrName := pb.TCertAttribute{AttributeName: att.AttributeName}
		attrNames = append(attrNames, &attrName)
	}

	req := &pb.ACAAttrReq{
		Ts:         &timestamp.Timestamp{Seconds: time.Now().Unix(), Nanos: 0},
		Id:         &pb.Identity{Id: id},
		ECert:      &pb.Cert{Cert: ecert},
		Attributes: attrNames,
		Signature:  nil}

	var rawReq []byte
	rawReq, err = proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	var r, s *big.Int

	r, s, err = primitives.ECDSASignDirect(tcap.tca.priv, rawReq)

	if err != nil {
		return nil, err
	}

	R, _ := r.MarshalText()
	S, _ := s.MarshalText()

	req.Signature = &pb.Signature{Type: pb.CryptoType_ECDSA, R: R, S: S}

	resp, err := acaP.RequestAttributes(context.Background(), req)
	if err != nil {
		return nil, err
	}

	if resp.Status >= pb.ACAAttrResp_FAILURE_MINVAL && resp.Status <= pb.ACAAttrResp_FAILURE_MAXVAL {
		return nil, fmt.Errorf("Error fetching attributes = %s", resp.Status)
	}

	return tcap.selectValidAttributes(resp.Cert.Cert)
}

// CreateCertificateSet requests the creation of a new transaction certificate set by the TCA.
func (tcap *TCAP) CreateCertificateSet(ctx context.Context, in *pb.TCertCreateSetReq) (*pb.TCertCreateSetResp, error) {
	tcapLogger.Debugf("grpc TCAP:CreateCertificateSet")

	id := in.Id.Id
	raw, err := tcap.tca.eca.readCertificateByKeyUsage(id, x509.KeyUsageDigitalSignature)
	if err != nil {
		return nil, err
	}

	return tcap.createCertificateSet(ctx, raw, in)
}

func (tcap *TCAP) createCertificateSet(ctx context.Context, raw []byte, in *pb.TCertCreateSetReq) (*pb.TCertCreateSetResp, error) {
	var attrs = []*pb.ACAAttribute{}
	var err error
	var id = in.Id.Id
	var timestamp = in.Ts.Seconds
	const tcertSubjectCommonNameValue string = "Transaction Certificate"

	if in.Attributes != nil && viper.GetBool("aca.enabled") {
		attrs, err = tcap.requestAttributes(id, raw, in.Attributes)
		if err != nil {
			return nil, err
		}
	}

	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	pub := cert.PublicKey.(*ecdsa.PublicKey)

	r, s := big.NewInt(0), big.NewInt(0)
	r.UnmarshalText(in.Sig.R)
	s.UnmarshalText(in.Sig.S)

	//sig := in.Sig
	in.Sig = nil

	hash := primitives.NewHash()
	raw, _ = proto.Marshal(in)
	hash.Write(raw)
	if ecdsa.Verify(pub, hash.Sum(nil), r, s) == false {
		return nil, errors.New("signature does not verify")
	}

	// Generate nonce for TCertIndex
	nonce := make([]byte, 16) // 8 bytes rand, 8 bytes timestamp
	rand.Reader.Read(nonce[:8])
	binary.LittleEndian.PutUint64(nonce[8:], uint64(in.Ts.Seconds))

	mac := hmac.New(primitives.GetDefaultHash(), tcap.tca.hmacKey)
	raw, _ = x509.MarshalPKIXPublicKey(pub)
	mac.Write(raw)
	kdfKey := mac.Sum(nil)

	num := int(in.Num)
	if num == 0 {
		num = 1
	}

	// the batch of TCerts
	var set []*pb.TCert

	for i := 0; i < num; i++ {
		tcertid := util.GenerateIntUUID()

		// Compute TCertIndex
		tidx := []byte(strconv.Itoa(2*i + 1))
		tidx = append(tidx[:], nonce[:]...)
		tidx = append(tidx[:], Padding...)

		mac := hmac.New(primitives.GetDefaultHash(), kdfKey)
		mac.Write([]byte{1})
		extKey := mac.Sum(nil)[:32]

		mac = hmac.New(primitives.GetDefaultHash(), kdfKey)
		mac.Write([]byte{2})
		mac = hmac.New(primitives.GetDefaultHash(), mac.Sum(nil))
		mac.Write(tidx)

		one := new(big.Int).SetInt64(1)
		k := new(big.Int).SetBytes(mac.Sum(nil))
		k.Mod(k, new(big.Int).Sub(pub.Curve.Params().N, one))
		k.Add(k, one)

		tmpX, tmpY := pub.ScalarBaseMult(k.Bytes())
		txX, txY := pub.Curve.Add(pub.X, pub.Y, tmpX, tmpY)
		txPub := ecdsa.PublicKey{Curve: pub.Curve, X: txX, Y: txY}

		// Compute encrypted TCertIndex
		encryptedTidx, err := primitives.CBCPKCS7Encrypt(extKey, tidx)
		if err != nil {
			return nil, err
		}

		extensions, preK0, err := tcap.generateExtensions(tcertid, encryptedTidx, cert, attrs)

		if err != nil {
			return nil, err
		}

		spec := NewDefaultPeriodCertificateSpecWithCommonName(id, tcertSubjectCommonNameValue, tcertid, &txPub, x509.KeyUsageDigitalSignature, extensions...)
		if raw, err = tcap.tca.createCertificateFromSpec(spec, timestamp, kdfKey, false); err != nil {
			tcapLogger.Error(err)
			return nil, err
		}

		set = append(set, &pb.TCert{Cert: raw, Prek0: preK0})
	}

	tcap.tca.persistCertificateSet(id, timestamp, nonce, kdfKey)

	return &pb.TCertCreateSetResp{Certs: &pb.CertSet{Ts: in.Ts, Id: in.Id, Key: kdfKey, Certs: set}}, nil
}

// Generate encrypted extensions to be included into the TCert (TCertIndex, EnrollmentID and attributes).
func (tcap *TCAP) generateExtensions(tcertid *big.Int, tidx []byte, enrollmentCert *x509.Certificate, attrs []*pb.ACAAttribute) ([]pkix.Extension, []byte, error) {
	// For each TCert we need to store and retrieve to the user the list of Ks used to encrypt the EnrollmentID and the attributes.
	extensions := make([]pkix.Extension, len(attrs))

	// Compute preK_1 to encrypt attributes and enrollment ID
	preK1, err := tcap.tca.getPreKFrom(enrollmentCert)
	if err != nil {
		return nil, nil, err
	}

	mac := hmac.New(primitives.GetDefaultHash(), preK1)
	mac.Write(tcertid.Bytes())
	preK0 := mac.Sum(nil)

	// Compute encrypted EnrollmentID
	mac = hmac.New(primitives.GetDefaultHash(), preK0)
	mac.Write([]byte("enrollmentID"))
	enrollmentIDKey := mac.Sum(nil)[:32]

	enrollmentID := []byte(enrollmentCert.Subject.CommonName)
	enrollmentID = append(enrollmentID, Padding...)

	encEnrollmentID, err := primitives.CBCPKCS7Encrypt(enrollmentIDKey, enrollmentID)
	if err != nil {
		return nil, nil, err
	}

	attributeIdentifierIndex := 9
	count := 0
	attrsHeader := make(map[string]int)
	// Encrypt and append attrs to the extensions slice
	for _, a := range attrs {
		count++

		value := []byte(a.AttributeValue)

		//Save the position of the attribute extension on the header.
		attrsHeader[a.AttributeName] = count

		if isEnabledAttributesEncryption() {
			value, err = attributes.EncryptAttributeValuePK0(preK0, a.AttributeName, value)
			if err != nil {
				return nil, nil, err
			}
		}

		// Generate an ObjectIdentifier for the extension holding the attribute
		TCertEncAttributes := asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, attributeIdentifierIndex + count}

		// Add the attribute extension to the extensions array
		extensions[count-1] = pkix.Extension{Id: TCertEncAttributes, Critical: false, Value: value}
	}

	// Append the TCertIndex to the extensions
	extensions = append(extensions, pkix.Extension{Id: TCertEncTCertIndex, Critical: true, Value: tidx})

	// Append the encrypted EnrollmentID to the extensions
	extensions = append(extensions, pkix.Extension{Id: TCertEncEnrollmentID, Critical: false, Value: encEnrollmentID})

	// Append the attributes header if there was attributes to include in the TCert
	if len(attrs) > 0 {
		headerValue, err := attributes.BuildAttributesHeader(attrsHeader)
		if err != nil {
			return nil, nil, err
		}
		if isEnabledAttributesEncryption() {
			headerValue, err = attributes.EncryptAttributeValuePK0(preK0, attributes.HeaderAttributeName, headerValue)
			if err != nil {
				return nil, nil, err
			}
		}
		extensions = append(extensions, pkix.Extension{Id: TCertAttributesHeaders, Critical: false, Value: headerValue})
	}

	return extensions, preK0, nil
}

// RevokeCertificate revokes a certificate from the TCA.  Not yet implemented.
func (tcap *TCAP) RevokeCertificate(context.Context, *pb.TCertRevokeReq) (*pb.CAStatus, error) {
	tcapLogger.Debugf("grpc TCAP:RevokeCertificate")

	return nil, errors.New("not yet implemented")
}

// RevokeCertificateSet revokes a certificate set from the TCA.  Not yet implemented.
func (tcap *TCAP) RevokeCertificateSet(context.Context, *pb.TCertRevokeSetReq) (*pb.CAStatus, error) {
	tcapLogger.Debugf("grpc TCAP:RevokeCertificateSet")

	return nil, errors.New("not yet implemented")
}

func isEnabledAttributesEncryption() bool {
	//TODO this code is commented because attributes encryption is not yet implemented.
	//return viper.GetBool("tca.attribute-encryption.enabled")
	return false
}
