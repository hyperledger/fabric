/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	pb "github.com/hyperledger/fabric/protos/peer"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("accessControl")

// Authenticator wraps a chaincode service and authenticates
// chaincode shims (containers)
type Authenticator interface {
	// DisableAccessCheck disables the access control
	// enforcement of the Authenticator
	DisableAccessCheck()

	// Generate returns a pair of certificate and private key,
	// and associates the hash of the certificate with the given
	// chaincode name
	Generate(ccName string) (*CertAndPrivKeyPair, error)

	// ChaincodeSupportServer - The Authenticator is registered
	// as a chaincode service
	pb.ChaincodeSupportServer
}

// CertAndPrivKeyPair contains a certificate
// and its corresponding private key in base64 format
type CertAndPrivKeyPair struct {
	// Cert - an x509 certificate encoded in base64
	Cert string
	// Key  - a private key of the corresponding certificate
	Key string
}

type authenticator struct {
	bypass bool
	mapper *certMapper
	pb.ChaincodeSupportServer
}

// NewAuthenticator returns a new authenticator that would wrap the given chaincode service
func NewAuthenticator(srv pb.ChaincodeSupportServer, ca CA) Authenticator {
	auth := &authenticator{
		mapper: newCertMapper(ca.newCertKeyPair),
	}
	auth.ChaincodeSupportServer = newInterceptor(srv, auth.authenticate)
	return auth
}

// DisableAccessCheck disables the access control
// enforcement of the Authenticator
func (ac *authenticator) DisableAccessCheck() {
	ac.bypass = true
}

// Generate returns a pair of certificate and private key,
// and associates the hash of the certificate with the given
// chaincode name
func (ac *authenticator) Generate(ccName string) (*CertAndPrivKeyPair, error) {
	cert, err := ac.mapper.genCert(ccName)
	if err != nil {
		return nil, err
	}
	return &CertAndPrivKeyPair{
		Key:  cert.privKeyString(),
		Cert: cert.pubKeyString(),
	}, nil
}

func (ac *authenticator) authenticate(msg *pb.ChaincodeMessage, stream grpc.ServerStream) error {
	if ac.bypass {
		return nil
	}

	if msg.Type != pb.ChaincodeMessage_REGISTER {
		logger.Warning("Got message", msg, "but expected a ChaincodeMessage_REGISTER message")
		return errors.New("First message needs to be a register")
	}

	chaincodeID := &pb.ChaincodeID{}
	err := proto.Unmarshal(msg.Payload, chaincodeID)
	if err != nil {
		logger.Warning("Failed unmarshaling message:", err)
		return err
	}
	ccName := chaincodeID.Name
	// Obtain certificate from stream
	hash := extractCertificateHashFromContext(stream.Context())
	if len(hash) == 0 {
		errMsg := fmt.Sprintf("TLS is active but chaincode %s didn't send certificate", ccName)
		logger.Warning(errMsg)
		return errors.New(errMsg)
	}
	// Look it up in the mapper
	registeredName := ac.mapper.lookup(certHash(hash))
	if registeredName == "" {
		errMsg := fmt.Sprintf("Chaincode %s with given certificate hash %v not found in registry", ccName, hash)
		logger.Warning(errMsg)
		return errors.New(errMsg)
	}
	if registeredName != ccName {
		errMsg := fmt.Sprintf("Chaincode %s with given certificate hash %v belongs to a different chaincode", ccName, hash)
		logger.Warning(errMsg)
		return fmt.Errorf(errMsg)
	}

	logger.Debug("Chaincode", ccName, "'s authentication is authorized")
	return nil
}
