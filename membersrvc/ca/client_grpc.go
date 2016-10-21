package ca

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"time"

	pb "github.com/hyperledger/fabric/membersrvc/protos"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

/** Performs Certificate type validation **/
/*
*  Checks for valid Cert format type
*  Cert expiration
*
 */
func isValidCertFormatted(certLocation string) bool {

	var isvalidCert = false
	certificate, err := ioutil.ReadFile(certLocation)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(certificate)
	if block == nil {
		certificates, err := x509.ParseCertificates(certificate)
		if err != nil {
			caLogger.Error("Not a valid Certificate")
		} else {
			validCert := validateCert(certificates[0])
			if !validCert {
				caLogger.Error("Certificate has expired")
			}
			return validCert
		}
	} else {
		certificates, err := x509.ParseCertificates(block.Bytes)
		if err != nil {
			caLogger.Error("Not a valid Certificate")
		} else {
			validCert := validateCert(certificates[0])
			if !validCert {
				caLogger.Error("Certificate has expired")
			}
			return validCert
		}
	}

	return isvalidCert

}

/** Given the cert , it checks for expiry
*  Does not check for revocation
 */
func validateCert(cert *x509.Certificate) bool {

	notBefore := cert.NotBefore
	notAfter := cert.NotAfter

	currentTime := time.Now()
	diffFromExpiry := notAfter.Sub(currentTime)
	diffFromStart := currentTime.Sub(notBefore)

	return ((diffFromExpiry > 0) && (diffFromStart > 0))

}

// NewClientTLSFromFile creates Client TLS connection credentials
// @certFile : TLS Server Certificate in PEM format
// @serverNameOverride : Common Name (CN) of the TLS Server Certificate
// returns Secure Transport Credentials
//
func NewClientTLSFromFile(certFile, serverNameOverride string) (credentials.TransportCredentials, error) {
	caLogger.Debug("upgrading to TLS1.2")
	b, err := ioutil.ReadFile(certFile)

	if err != nil {
		caLogger.Errorf("Certificate could not be found in the [%s] path", certFile)
		return nil, err
	}

	if !isValidCertFormatted(certFile) {
		return nil, nil
	}

	cp := x509.NewCertPool()

	ok := cp.AppendCertsFromPEM(b)
	if !ok {
		caLogger.Error("credentials: failed to append certificates: ")
		return nil, nil
	}
	return credentials.NewTLS(&tls.Config{ServerName: serverNameOverride, RootCAs: cp, MinVersion: 0, MaxVersion: 0}), nil
}

//GetClientConn returns a connection to the server located on *address*.
func GetClientConn(address string, serverName string) (*grpc.ClientConn, error) {

	caLogger.Debug("GetACAClient: using the given gRPC client connection to return a new ACA client")
	var opts []grpc.DialOption

	if viper.GetBool("security.tls_enabled") {
		caLogger.Debug("TLS was enabled [security.tls_enabled == true]")

		creds, err := NewClientTLSFromFile(viper.GetString("security.client.cert.file"), viper.GetString("security.serverhostoverride"))

		if err != nil {
			caLogger.Error("Could not establish TLS client connection in GetClientConn while getting creds:")
			caLogger.Error(err)
			return nil, err
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		caLogger.Debug("TLS was not enabled [security.tls_enabled == false]")
		opts = append(opts, grpc.WithInsecure())
	}
	opts = append(opts, grpc.WithTimeout(time.Second*3))
	return grpc.Dial(address, opts...)
}

//GetACAClient returns a client to Attribute Certificate Authority.
func GetACAClient() (*grpc.ClientConn, pb.ACAPClient, error) {
	caLogger.Debug("GetACAClient: Trying to create a new ACA Client from the connection provided")
	conn, err := GetClientConn(viper.GetString("aca.address"), viper.GetString("aca.server-name"))
	if err != nil {
		return nil, nil, err
	}

	client := pb.NewACAPClient(conn)

	return conn, client, nil
}
