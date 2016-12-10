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

package common

import (
	"fmt"

	"github.com/hyperledger/fabric/core/errors"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/peer/msp"
	"github.com/hyperledger/fabric/flogging"
	"github.com/hyperledger/fabric/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

// UndefinedParamValue defines what undefined parameters in the command line will initialise to
const UndefinedParamValue = ""

// GetEndorserClient returns a new endorser client connection for this peer
func GetEndorserClient() (pb.EndorserClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = errors.ErrorWithCallstack(errors.Peer, errors.PeerConnectionError, err.Error())
		return nil, err
	}
	endorserClient := pb.NewEndorserClient(clientConn)
	return endorserClient, nil
}

// GetAdminClient returns a new admin client connection for this peer
func GetAdminClient() (pb.AdminClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = errors.ErrorWithCallstack(errors.Peer, errors.PeerConnectionError, err.Error())
		return nil, err
	}
	adminClient := pb.NewAdminClient(clientConn)
	return adminClient, nil
}

// SetErrorLoggingLevel sets the 'error' module's logger to the value in
// core.yaml
func SetErrorLoggingLevel() error {
	viperErrorLoggingLevel := viper.GetString("logging.error")
	_, err := flogging.SetModuleLogLevel("error", viperErrorLoggingLevel)

	return err
}

// GetDefaultSigner return a default Signer(Default/PERR) for cli
func GetDefaultSigner() (msp.SigningIdentity, error) {
	signer, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		return nil, fmt.Errorf("Error obtaining the default signing identity, err %s", err)
	}

	return signer, err
}
