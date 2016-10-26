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

package chaincode

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

var sysccLogger = logging.MustGetLogger("sysccapi")

// SystemChaincode defines the metadata needed to initialize system chaincode
// when the fabric comes up. SystemChaincodes are installed by adding an
// entry in importsysccs.go
type SystemChaincode struct {
	// Enabled a convenient switch to enable/disable system chaincode without
	// having to remove entry from importsysccs.go
	Enabled bool

	//Unique name of the system chaincode
	Name string

	//Path to the system chaincode; currently not used
	Path string

	//InitArgs initialization arguments to startup the system chaincode
	InitArgs [][]byte

	// Chaincode is the actual chaincode object
	Chaincode shim.Chaincode
}

// RegisterSysCC registers the given system chaincode with the peer
func RegisterSysCC(syscc *SystemChaincode) error {
	if !syscc.Enabled || !isWhitelisted(syscc) {
		sysccLogger.Info(fmt.Sprintf("system chaincode (%s,%s) disabled", syscc.Name, syscc.Path))
		return nil
	}

	err := inproccontroller.Register(syscc.Path, syscc.Chaincode)
	if err != nil {
		//if the type is registered, the instance may not be... keep going
		if _, ok := err.(inproccontroller.SysCCRegisteredErr); !ok {
			errStr := fmt.Sprintf("could not register (%s,%v): %s", syscc.Path, syscc, err)
			sysccLogger.Error(errStr)
			return fmt.Errorf(errStr)
		}
	}

	chaincodeID := &protos.ChaincodeID{Path: syscc.Path, Name: syscc.Name}
	spec := protos.ChaincodeSpec{Type: protos.ChaincodeSpec_Type(protos.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeID: chaincodeID, CtorMsg: &protos.ChaincodeInput{Args: syscc.InitArgs}}

	//PDMP - use default chain to get the simulator
	//Note that we are just colleting simulation though
	//we will not submit transactions. We *COULD* commit
	//transactions ourselves
	chainName := string(DefaultChain)

	lgr := kvledger.GetLedger(chainName)
	var txsim ledger.TxSimulator
	if txsim, err = lgr.NewTxSimulator(); err != nil {
		return err
	}

	defer txsim.Done()

	ctxt := context.WithValue(context.Background(), TXSimulatorKey, txsim)
	if deployErr := DeploySysCC(ctxt, &spec); deployErr != nil {
		errStr := fmt.Sprintf("deploy chaincode failed: %s", deployErr)
		sysccLogger.Error(errStr)
		return fmt.Errorf(errStr)
	}

	sysccLogger.Info("system chaincode %s(%s) registered", syscc.Name, syscc.Path)
	return err
}

// deregisterSysCC stops the system chaincode and deregisters it from inproccontroller
func deregisterSysCC(syscc *SystemChaincode) error {
	chaincodeID := &protos.ChaincodeID{Path: syscc.Path, Name: syscc.Name}
	spec := &protos.ChaincodeSpec{Type: protos.ChaincodeSpec_Type(protos.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeID: chaincodeID, CtorMsg: &protos.ChaincodeInput{Args: syscc.InitArgs}}

	ctx := context.Background()
	// First build and get the deployment spec
	chaincodeDeploymentSpec, err := buildSysCC(ctx, spec)

	if err != nil {
		sysccLogger.Error(fmt.Sprintf("Error deploying chaincode spec: %v\n\n error: %s", spec, err))
		return err
	}

	chaincodeSupport := GetChain(DefaultChain)
	if chaincodeSupport != nil {
		err = chaincodeSupport.Stop(ctx, chaincodeDeploymentSpec)
	}

	return err
}

// buildLocal builds a given chaincode code
func buildSysCC(context context.Context, spec *protos.ChaincodeSpec) (*protos.ChaincodeDeploymentSpec, error) {
	var codePackageBytes []byte
	chaincodeDeploymentSpec := &protos.ChaincodeDeploymentSpec{ExecEnv: protos.ChaincodeDeploymentSpec_SYSTEM, ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

// DeploySysCC deploys the supplied system chaincode to the local peer
func DeploySysCC(ctx context.Context, spec *protos.ChaincodeSpec) error {
	// First build and get the deployment spec
	chaincodeDeploymentSpec, err := buildSysCC(ctx, spec)

	if err != nil {
		sysccLogger.Error(fmt.Sprintf("Error deploying chaincode spec: %v\n\n error: %s", spec, err))
		return err
	}

	transaction, err := protos.NewChaincodeDeployTransaction(chaincodeDeploymentSpec, chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	_, _, err = Execute(ctx, GetChain(DefaultChain), transaction)

	return err
}

func isWhitelisted(syscc *SystemChaincode) bool {
	chaincodes := viper.GetStringMapString("chaincode.system")
	val, ok := chaincodes[syscc.Name]
	enabled := val == "enable" || val == "true" || val == "yes"
	return ok && enabled
}
