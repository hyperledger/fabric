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
	"encoding/json"
	"errors"
	"math/big"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	pb "github.com/hyperledger/fabric/membersrvc/protos"
	"github.com/op/go-logging"
	"golang.org/x/net/context"
)

var ecaaLogger = logging.MustGetLogger("ecaa")

// ECAA serves the administrator GRPC interface of the ECA.
//
type ECAA struct {
	eca *ECA
}

// RegisterUser registers a new user with the ECA.  If the user had been registered before
// an error is returned.
//
func (ecaa *ECAA) RegisterUser(ctx context.Context, in *pb.RegisterUserReq) (*pb.Token, error) {
	ecaaLogger.Debug("gRPC ECAA:RegisterUser")

	// Check the signature
	err := ecaa.checkRegistrarSignature(in)
	if err != nil {
		return nil, err
	}

	// Register the user
	registrarID := in.Registrar.Id.Id
	in.Registrar.Id = nil
	registrar := pb.RegisterUserReq{Registrar: in.Registrar}
	json, err := json.Marshal(registrar)
	if err != nil {
		return nil, err
	}
	jsonStr := string(json)
	ecaaLogger.Debugf("gRPC ECAA:RegisterUser: json=%s", jsonStr)
	tok, err := ecaa.eca.registerUser(in.Id.Id, in.Affiliation, in.Role, in.Attributes, ecaa.eca.aca, registrarID, jsonStr)

	// Return the one-time password
	return &pb.Token{Tok: []byte(tok)}, err

}

func (ecaa *ECAA) checkRegistrarSignature(in *pb.RegisterUserReq) error {
	ecaaLogger.Debug("ECAA.checkRegistrarSignature")

	// If no registrar was specified
	if in.Registrar == nil || in.Registrar.Id == nil || in.Registrar.Id.Id == "" {
		ecaaLogger.Debug("gRPC ECAA:checkRegistrarSignature: no registrar was specified")
		return errors.New("no registrar was specified")
	}

	// Get the raw cert for the registrar
	registrar := in.Registrar.Id.Id
	raw, err := ecaa.eca.readCertificateByKeyUsage(registrar, x509.KeyUsageDigitalSignature)
	if err != nil {
		return err
	}

	// Parse the cert
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		return err
	}

	// Remove the signature
	sig := in.Sig
	in.Sig = nil

	// Marshall the raw bytes
	r, s := big.NewInt(0), big.NewInt(0)
	r.UnmarshalText(sig.R)
	s.UnmarshalText(sig.S)

	hash := primitives.NewHash()
	raw, _ = proto.Marshal(in)
	hash.Write(raw)

	// Check the signature
	if ecdsa.Verify(cert.PublicKey.(*ecdsa.PublicKey), hash.Sum(nil), r, s) == false {
		// Signature verification failure
		ecaaLogger.Debugf("ECAA.checkRegistrarSignature: failure for %s (len=%d): %+v", registrar, len(raw), in)
		return errors.New("Signature verification failed.")
	}

	// Signature verification was successful
	ecaaLogger.Debugf("ECAA.checkRegistrarSignature: success for %s", registrar)
	return nil
}

// ReadUserSet returns a list of users matching the parameters set in the read request.
//
func (ecaa *ECAA) ReadUserSet(ctx context.Context, in *pb.ReadUserSetReq) (*pb.UserSet, error) {
	ecaaLogger.Debug("gRPC ECAA:ReadUserSet")

	req := in.Req.Id
	if ecaa.eca.readRole(req)&int(pb.Role_AUDITOR) == 0 {
		return nil, errors.New("Access denied.")
	}

	raw, err := ecaa.eca.readCertificateByKeyUsage(req, x509.KeyUsageDigitalSignature)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	sig := in.Sig
	in.Sig = nil

	r, s := big.NewInt(0), big.NewInt(0)
	r.UnmarshalText(sig.R)
	s.UnmarshalText(sig.S)

	hash := primitives.NewHash()
	raw, _ = proto.Marshal(in)
	hash.Write(raw)
	if ecdsa.Verify(cert.PublicKey.(*ecdsa.PublicKey), hash.Sum(nil), r, s) == false {
		return nil, errors.New("Signature verification failed.")
	}

	rows, err := ecaa.eca.readUsers(int(in.Role))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*pb.User
	if err == nil {
		for rows.Next() {
			var id string
			var role int

			err = rows.Scan(&id, &role)
			users = append(users, &pb.User{Id: &pb.Identity{Id: id}, Role: pb.Role(role)})
		}
		err = rows.Err()
	}

	return &pb.UserSet{Users: users}, err
}

// RevokeCertificate revokes a certificate from the ECA.  Not yet implemented.
//
func (ecaa *ECAA) RevokeCertificate(context.Context, *pb.ECertRevokeReq) (*pb.CAStatus, error) {
	ecaaLogger.Debug("gRPC ECAA:RevokeCertificate")

	return nil, errors.New("ECAA:RevokeCertificate method not (yet) implemented")
}

// PublishCRL requests the creation of a certificate revocation list from the ECA.  Not yet implemented.
//
func (ecaa *ECAA) PublishCRL(context.Context, *pb.ECertCRLReq) (*pb.CAStatus, error) {
	ecaaLogger.Debug("gRPC ECAA:CreateCRL")

	return nil, errors.New("ECAA:PublishCRL method not (yet) implemented")
}
