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

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	pb "github.com/hyperledger/fabric/protos/peer"
	logging "github.com/op/go-logging"
	"github.com/spf13/viper"
)

// UndefinedParamValue defines what undefined parameters in the command line will initialise to
const UndefinedParamValue = ""

//InitConfig initializes viper config
func InitConfig(cmdRoot string) error {
	config.InitViper(nil, cmdRoot)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		return fmt.Errorf("Fatal error when reading %s config file: %s\n", cmdRoot, err)
	}

	return nil
}

//InitCrypto initializes crypto for this peer
func InitCrypto(mspMgrConfigDir string, localMSPID string) error {
	// Init the BCCSP
	var bccspConfig *factory.FactoryOpts
	err := viperutil.EnhancedExactUnmarshalKey("peer.BCCSP", &bccspConfig)
	if err != nil {
		return fmt.Errorf("Could not parse YAML config [%s]", err)
	}

	err = mspmgmt.LoadLocalMsp(mspMgrConfigDir, bccspConfig, localMSPID)
	if err != nil {
		return fmt.Errorf("Fatal error when setting up MSP from directory %s: err %s\n", mspMgrConfigDir, err)
	}

	return nil
}

// GetEndorserClient returns a new endorser client connection for this peer
func GetEndorserClient() (pb.EndorserClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = errors.ErrorWithCallstack("PER", "404", "Error trying to connect to local peer").WrapError(err)
		return nil, err
	}
	endorserClient := pb.NewEndorserClient(clientConn)
	return endorserClient, nil
}

// GetAdminClient returns a new admin client connection for this peer
func GetAdminClient() (pb.AdminClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = errors.ErrorWithCallstack("PER", "404", "Error trying to connect to local peer").WrapError(err)
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
		err = CheckLogLevel(logLevelFromViper)
		if err != nil {
			if module == "error" {
				// if 'logging.error' not found in core.yaml or an invalid level has
				// been entered, set default to debug to ensure the callstack is
				// appended to all CallStackErrors
				logLevelFromViper = "debug"
			} else {
				return err
			}
		}
		_, err = flogging.SetModuleLevel(module, logLevelFromViper)
	}
	return err
}

// CheckLogLevel checks that a given log level string is valid
func CheckLogLevel(level string) error {
	_, err := logging.LogLevel(level)
	if err != nil {
		err = errors.ErrorWithCallstack("LOG", "400", "Invalid log level provided - %s", level)
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
