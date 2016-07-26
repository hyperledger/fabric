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
	"crypto/x509"

	ecies "github.com/hyperledger/fabric/core/crypto/primitives/ecies"
)

func (node *nodeImpl) registerCryptoEngine(enrollID, enrollPWD string) error {
	node.Debug("Registering node crypto engine...")

	// Init CLI
	node.eciesSPI = ecies.NewSPI()

	if err := node.initTLS(); err != nil {
		node.Errorf("Failed initliazing TLS [%s].", err.Error())

		return err
	}

	if err := node.retrieveECACertsChain(enrollID); err != nil {
		node.Errorf("Failed retrieving ECA certs chain [%s].", err.Error())

		return err
	}

	if err := node.retrieveTCACertsChain(enrollID); err != nil {
		node.Errorf("Failed retrieving ECA certs chain [%s].", err.Error())

		return err
	}

	if err := node.retrieveEnrollmentData(enrollID, enrollPWD); err != nil {
		node.Errorf("Failed retrieving enrollment data [%s].", err.Error())

		return err
	}

	if err := node.retrieveTLSCertificate(enrollID, enrollPWD); err != nil {
		node.Errorf("Failed retrieving enrollment data: %s", err)

		return err
	}

	node.Debug("Registering node crypto engine...done!")

	return nil
}

func (node *nodeImpl) initCryptoEngine() error {
	node.Debug("Initializing node crypto engine...")

	// Init CLI
	node.eciesSPI = ecies.NewSPI()

	// Init certPools
	node.rootsCertPool = x509.NewCertPool()
	node.tlsCertPool = x509.NewCertPool()
	node.ecaCertPool = x509.NewCertPool()
	node.tcaCertPool = x509.NewCertPool()

	// Load ECA certs chain
	if err := node.loadECACertsChain(); err != nil {
		return err
	}

	// Load TCA certs chain
	if err := node.loadTCACertsChain(); err != nil {
		return err
	}

	// Load enrollment secret key
	if err := node.loadEnrollmentKey(); err != nil {
		return err
	}

	// Load enrollment certificate and set validator ID
	if err := node.loadEnrollmentCertificate(); err != nil {
		return err
	}

	// Load enrollment id
	if err := node.loadEnrollmentID(); err != nil {
		return err
	}

	// Load enrollment chain key
	if err := node.loadEnrollmentChainKey(); err != nil {
		return err
	}

	// Load TLS certs chain certificate
	if err := node.loadTLSCACertsChain(); err != nil {
		return err
	}

	// Load tls certificate
	if err := node.loadTLSCertificate(); err != nil {
		return err
	}

	node.Debug("Initializing node crypto engine...done!")

	return nil
}
