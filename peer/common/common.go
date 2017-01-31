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
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/errors"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

// UndefinedParamValue defines what undefined parameters in the command line will initialise to
const UndefinedParamValue = ""

//InitConfig initializes viper config
func InitConfig(cmdRoot string) error {
	var alternativeCfgPath = os.Getenv("PEER_CFG_PATH")
	if alternativeCfgPath != "" {
		viper.AddConfigPath(alternativeCfgPath) // Path to look for the config file in
	} else {
		viper.AddConfigPath("./") // Path to look for the config file in
		// Path to look for the config file in based on GOPATH
		gopath := os.Getenv("GOPATH")
		for _, p := range filepath.SplitList(gopath) {
			peerpath := filepath.Join(p, "src/github.com/hyperledger/fabric/peer")
			viper.AddConfigPath(peerpath)
		}
	}

	// Now set the configuration file.
	viper.SetConfigName(cmdRoot) // Name of config file (without extension)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		return fmt.Errorf("Fatal error when reading %s config file: %s\n", cmdRoot, err)
	}

	return nil
}

//InitCrypto initializes crypto for this peer
func InitCrypto(mspMgrConfigDir string) error {
	// FIXME: when this peer joins a chain, it should get the
	// config for that chain with the list of MSPs that the
	// chain uses; however this is not yet implemented.
	// Additionally, we might always want to have an MSP for
	// the local test chain so that we can run tests with the
	// peer CLI. This is why we create this fake setup here for now
	err := mspmgmt.LoadFakeSetupWithLocalMspAndTestChainMsp(mspMgrConfigDir)
	if err != nil {
		return fmt.Errorf("Fatal error when setting up MSP from directory %s: err %s\n", mspMgrConfigDir, err)
	}

	return nil
}

// GetEndorserClient returns a new endorser client connection for this peer
func GetEndorserClient() (pb.EndorserClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = errors.ErrorWithCallstack("Peer", "ConnectionError", "Error trying to connect to local peer: %s", err.Error())
		return nil, err
	}
	endorserClient := pb.NewEndorserClient(clientConn)
	return endorserClient, nil
}

func GetAnchorPeersParser(anchorPeerParam string) *AnchorPeerParser {
	if len(anchorPeerParam) == 0 {
		return GetDefaultAnchorPeerParser()
	}
	return &AnchorPeerParser{anchorPeerParam: anchorPeerParam}
}

// GetAdminClient returns a new admin client connection for this peer
func GetAdminClient() (pb.AdminClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = errors.ErrorWithCallstack("Peer", "ConnectionError", "Error trying to connect to local peer: %s", err.Error())
		return nil, err
	}
	adminClient := pb.NewAdminClient(clientConn)
	return adminClient, nil
}

// SetLogLevelFromViper sets the log level for 'module' logger to the value in
// core.yaml
func SetLogLevelFromViper(module string) error {
	var err error
	if module != "" {
		logLevelFromViper := viper.GetString("logging." + module)
		_, err = flogging.SetModuleLevel(module, logLevelFromViper)
	}
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
