/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/core/peer"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("chaincode.accesscontrol")

// CertAndPrivKeyPair contains a certificate
// and its corresponding private key in base64 format
type CertAndPrivKeyPair struct {
	// Cert is an x509 certificate
	Cert []byte
	// Key is a private key of the corresponding certificate
	Key []byte
}

type Authenticator struct {
	mapper *certMapper
}

func (auth *Authenticator) Wrap(srv pb.ChaincodeSupportServer) pb.ChaincodeSupportServer {
	return newInterceptor(srv, auth.authenticate)
}

func GetPeerCerts() (*tlsgen.CertKeyPair, error) {
	config, err := peer.GetServerConfig()
	if err != nil {
		logger.Error("Cannot get peer server config inside access")
		return nil, err
	}
	block, _ := pem.Decode(config.SecOpts.Certificate)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		logger.Error("Could not generate necessary X509 certificate inside access")
		return nil, err
	}

	return &tlsgen.CertKeyPair{
		Cert:    config.SecOpts.Certificate,
		Key:     config.SecOpts.Key,
		TLSCert: cert}, nil
}

// NewAuthenticator returns a new authenticator that can wrap a chaincode service
func NewAuthenticator(ca tlsgen.CA) *Authenticator {
	return &Authenticator{
		mapper: newCertMapper(ca.NewClientCertKeyPair),
	}
}

// Generate returns a pair of certificate and private key,
// and associates the hash of the certificate with the given
// chaincode name
func (ac *Authenticator) Generate(ccName string) (*CertAndPrivKeyPair, error) {
	cert, err := ac.mapper.genCert(ccName)
	if err != nil {
		return nil, err
	}
	return &CertAndPrivKeyPair{
		Key:  cert.PrivKeyString(),
		Cert: cert.PubKeyString(),
	}, nil
}

func (ac *Authenticator) authenticate(msg *pb.ChaincodeMessage, stream grpc.ServerStream) error {
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
