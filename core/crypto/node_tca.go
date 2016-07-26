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
	membersrvc "github.com/hyperledger/fabric/membersrvc/protos"

	"errors"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func (node *nodeImpl) retrieveTCACertsChain(userID string) error {
	if !node.ks.certMissing(node.conf.getTCACertsChainFilename()) {
		return nil
	}

	// Retrieve TCA certificate and verify it
	tcaCertRaw, err := node.getTCACertificate()
	if err != nil {
		node.Errorf("Failed getting TCA certificate [%s].", err.Error())

		return err
	}
	node.Debugf("TCA certificate [% x]", tcaCertRaw)

	// TODO: Test TCA cert againt root CA
	_, err = primitives.DERToX509Certificate(tcaCertRaw)
	if err != nil {
		node.Errorf("Failed parsing TCA certificate [%s].", err.Error())

		return err
	}

	// Store TCA cert
	node.Debugf("Storing TCA certificate for [%s]...", userID)

	if err := node.ks.storeCert(node.conf.getTCACertsChainFilename(), tcaCertRaw); err != nil {
		node.Errorf("Failed storing tca certificate [%s].", err.Error())
		return err
	}

	return nil
}

func (node *nodeImpl) loadTCACertsChain() error {
	// Load TCA certs chain
	node.Debug("Loading TCA certificates chain...")

	cert, err := node.ks.loadCert(node.conf.getTCACertsChainFilename())
	if err != nil {
		node.Errorf("Failed loading TCA certificates chain [%s].", err.Error())

		return err
	}

	// Prepare ecaCertPool
	ok := node.tcaCertPool.AppendCertsFromPEM(cert)
	if !ok {
		node.Error("Failed appending TCA certificates chain.")

		return errors.New("Failed appending TCA certificates chain.")
	}

	return nil
}

func (node *nodeImpl) getTCAClient() (*grpc.ClientConn, membersrvc.TCAPClient, error) {
	node.Debug("Getting TCA client...")

	conn, err := node.getClientConn(node.conf.getTCAPAddr(), node.conf.getTCAServerName())
	if err != nil {
		node.Errorf("Failed getting client connection: [%s]", err)
	}

	client := membersrvc.NewTCAPClient(conn)

	node.Debug("Getting TCA client...done")

	return conn, client, nil
}

func (node *nodeImpl) callTCAReadCACertificate(ctx context.Context, opts ...grpc.CallOption) (*membersrvc.Cert, error) {
	// Get a TCA Client
	sock, tcaP, err := node.getTCAClient()
	defer sock.Close()

	// Issue the request
	cert, err := tcaP.ReadCACertificate(ctx, &membersrvc.Empty{}, opts...)
	if err != nil {
		node.Errorf("Failed requesting tca read certificate [%s].", err.Error())

		return nil, err
	}

	return cert, nil
}

func (node *nodeImpl) getTCACertificate() ([]byte, error) {
	response, err := node.callTCAReadCACertificate(context.Background())
	if err != nil {
		node.Errorf("Failed requesting TCA certificate [%s].", err.Error())

		return nil, err
	}

	// TODO: check response.Cert against rootCA

	return response.Cert, nil
}
