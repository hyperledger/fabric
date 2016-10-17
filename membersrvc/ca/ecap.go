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
	"bytes"
	"crypto/ecdsa"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io/ioutil"
	"math/big"
	"strconv"
	"time"

	"github.com/hyperledger/fabric/core/crypto/primitives/ecies"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	pb "github.com/hyperledger/fabric/membersrvc/protos"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

var ecapLogger = logging.MustGetLogger("ecap")

// ECAP serves the public GRPC interface of the ECA.
//
type ECAP struct {
	eca *ECA
}

// ReadCACertificate reads the certificate of the ECA.
//
func (ecap *ECAP) ReadCACertificate(ctx context.Context, in *pb.Empty) (*pb.Cert, error) {
	ecapLogger.Debug("gRPC ECAP:ReadCACertificate")

	return &pb.Cert{Cert: ecap.eca.raw}, nil
}

func (ecap *ECAP) fetchAttributes(cert *pb.Cert) error {
	//TODO we are creating a new client connection per each ecert request. We should implement a connections pool.
	sock, acaP, err := GetACAClient()
	if err != nil {
		return err
	}
	defer sock.Close()

	req := &pb.ACAFetchAttrReq{
		Ts:        &timestamp.Timestamp{Seconds: time.Now().Unix(), Nanos: 0},
		ECert:     cert,
		Signature: nil}

	var rawReq []byte
	rawReq, err = proto.Marshal(req)
	if err != nil {
		return err
	}

	var r, s *big.Int

	r, s, err = primitives.ECDSASignDirect(ecap.eca.priv, rawReq)

	if err != nil {
		return err
	}

	R, _ := r.MarshalText()
	S, _ := s.MarshalText()

	req.Signature = &pb.Signature{Type: pb.CryptoType_ECDSA, R: R, S: S}

	resp, err := acaP.FetchAttributes(context.Background(), req)
	if err != nil {
		return err
	}

	if resp.Status != pb.ACAFetchAttrResp_FAILURE {
		return nil
	}
	return errors.New("Error fetching attributes.")
}

// CreateCertificatePair requests the creation of a new enrollment certificate pair by the ECA.
//
func (ecap *ECAP) CreateCertificatePair(ctx context.Context, in *pb.ECertCreateReq) (*pb.ECertCreateResp, error) {
	ecapLogger.Debug("gRPC ECAP:CreateCertificate")

	// validate token
	var tok, prev []byte
	var role, state int
	var enrollID string

	id := in.Id.Id
	err := ecap.eca.readUser(id).Scan(&role, &tok, &state, &prev, &enrollID)

	if err != nil {
		errMsg := "Identity lookup error: " + err.Error()
		ecapLogger.Debug(errMsg)
		return nil, errors.New(errMsg)
	}
	if !bytes.Equal(tok, in.Tok.Tok) {
		ecapLogger.Debugf("id or token mismatch: id=%s", id)
		return nil, errors.New("Identity or token does not match.")
	}

	ekey, err := x509.ParsePKIXPublicKey(in.Enc.Key)
	if err != nil {
		return nil, err
	}

	fetchResult := pb.FetchAttrsResult{Status: pb.FetchAttrsResult_SUCCESS, Msg: ""}
	switch {
	case state == 0:
		// initial request, create encryption challenge
		tok = []byte(randomString(12))

		mutex.Lock()
		_, err = ecap.eca.db.Exec("UPDATE Users SET token=?, state=?, key=? WHERE id=?", tok, 1, in.Enc.Key, id)
		mutex.Unlock()

		if err != nil {
			ecapLogger.Error(err)
			return nil, err
		}

		spi := ecies.NewSPI()
		eciesKey, err := spi.NewPublicKey(nil, ekey.(*ecdsa.PublicKey))
		if err != nil {
			return nil, err
		}

		ecies, err := spi.NewAsymmetricCipherFromPublicKey(eciesKey)
		if err != nil {
			return nil, err
		}

		out, err := ecies.Process(tok)

		return &pb.ECertCreateResp{Certs: nil, Chain: nil, Pkchain: nil, Tok: &pb.Token{Tok: out}}, err

	case state == 1:
		// ensure that the same encryption key is signed that has been used for the challenge
		if subtle.ConstantTimeCompare(in.Enc.Key, prev) != 1 {
			return nil, errors.New("Encryption keys do not match.")
		}

		// validate request signature
		sig := in.Sig
		in.Sig = nil

		r, s := big.NewInt(0), big.NewInt(0)
		r.UnmarshalText(sig.R)
		s.UnmarshalText(sig.S)

		if in.Sign.Type != pb.CryptoType_ECDSA {
			return nil, errors.New("Unsupported (signing) key type.")
		}
		skey, err := x509.ParsePKIXPublicKey(in.Sign.Key)
		if err != nil {
			return nil, err
		}

		hash := primitives.NewHash()
		raw, _ := proto.Marshal(in)
		hash.Write(raw)
		if ecdsa.Verify(skey.(*ecdsa.PublicKey), hash.Sum(nil), r, s) == false {
			return nil, errors.New("Signature verification failed.")
		}

		// create new certificate pair
		ts := time.Now().Add(-1 * time.Minute).UnixNano()

		spec := NewDefaultCertificateSpecWithCommonName(id, enrollID, skey.(*ecdsa.PublicKey), x509.KeyUsageDigitalSignature, pkix.Extension{Id: ECertSubjectRole, Critical: true, Value: []byte(strconv.Itoa(ecap.eca.readRole(id)))})
		sraw, err := ecap.eca.createCertificateFromSpec(spec, ts, nil, true)
		if err != nil {
			ecapLogger.Error(err)
			return nil, err
		}

		_ = ioutil.WriteFile("/tmp/ecert_"+id, sraw, 0644)

		spec = NewDefaultCertificateSpecWithCommonName(id, enrollID, ekey.(*ecdsa.PublicKey), x509.KeyUsageDataEncipherment, pkix.Extension{Id: ECertSubjectRole, Critical: true, Value: []byte(strconv.Itoa(ecap.eca.readRole(id)))})
		eraw, err := ecap.eca.createCertificateFromSpec(spec, ts, nil, true)
		if err != nil {
			mutex.Lock()
			ecap.eca.db.Exec("DELETE FROM Certificates Where id=?", id)
			mutex.Unlock()
			ecapLogger.Error(err)
			return nil, err
		}

		mutex.Lock()
		_, err = ecap.eca.db.Exec("UPDATE Users SET state=? WHERE id=?", 2, id)
		mutex.Unlock()
		if err != nil {
			mutex.Lock()
			ecap.eca.db.Exec("DELETE FROM Certificates Where id=?", id)
			mutex.Unlock()
			ecapLogger.Error(err)
			return nil, err
		}

		var obcECKey []byte
		if role == int(pb.Role_VALIDATOR) {
			obcECKey = ecap.eca.obcPriv
		} else {
			obcECKey = ecap.eca.obcPub
		}
		if role == int(pb.Role_CLIENT) {
			//Only client have to fetch attributes.
			if viper.GetBool("aca.enabled") {
				err = ecap.fetchAttributes(&pb.Cert{Cert: sraw})
				if err != nil {
					fetchResult = pb.FetchAttrsResult{Status: pb.FetchAttrsResult_FAILURE, Msg: err.Error()}

				}
			}
		}

		return &pb.ECertCreateResp{Certs: &pb.CertPair{Sign: sraw, Enc: eraw}, Chain: &pb.Token{Tok: ecap.eca.obcKey}, Pkchain: obcECKey, Tok: nil, FetchResult: &fetchResult}, nil
	}

	return nil, errors.New("Invalid (=expired) certificate creation token provided.")
}

// ReadCertificatePair reads an enrollment certificate pair from the ECA.
//
func (ecap *ECAP) ReadCertificatePair(ctx context.Context, in *pb.ECertReadReq) (*pb.CertPair, error) {
	ecapLogger.Debug("gRPC ECAP:ReadCertificate")

	rows, err := ecap.eca.readCertificates(in.Id.Id)
	defer rows.Close()

	hasResults := false
	var certs [][]byte
	if err == nil {
		for rows.Next() {
			hasResults = true
			var raw []byte
			err = rows.Scan(&raw)
			certs = append(certs, raw)
		}
		err = rows.Err()
	}

	if !hasResults {
		return nil, errors.New("No certificates for the given identity were found.")
	}
	return &pb.CertPair{Sign: certs[0], Enc: certs[1]}, err
}

// ReadCertificateByHash reads a single enrollment certificate by hash from the ECA.
//
func (ecap *ECAP) ReadCertificateByHash(ctx context.Context, hash *pb.Hash) (*pb.Cert, error) {
	ecapLogger.Debug("gRPC ECAP:ReadCertificateByHash")

	raw, err := ecap.eca.readCertificateByHash(hash.Hash)
	return &pb.Cert{Cert: raw}, err
}

// RevokeCertificatePair revokes a certificate pair from the ECA.  Not yet implemented.
//
func (ecap *ECAP) RevokeCertificatePair(context.Context, *pb.ECertRevokeReq) (*pb.CAStatus, error) {
	ecapLogger.Debug("gRPC ECAP:RevokeCertificate")

	return nil, errors.New("ECAP:RevokeCertificate method not (yet) implemented")
}
