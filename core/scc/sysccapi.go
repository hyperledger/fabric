/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"errors"
	"fmt"

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/peer"

	"github.com/spf13/viper"

	pb "github.com/hyperledger/fabric/protos/peer"
)

var sysccLogger = flogging.MustGetLogger("sccapi")

// Registrar provides a way for system chaincodes to be registered
type Registrar interface {
	// Register registers a system chaincode
	Register(ccid *ccintf.CCID, cc shim.Chaincode) error
}

// SystemChaincode defines the metadata needed to initialize system chaincode
// when the fabric comes up. SystemChaincodes are installed by adding an
// entry in importsysccs.go
type SystemChaincode struct {
	//Unique name of the system chaincode
	Name string

	//Path to the system chaincode; currently not used
	Path string

	//InitArgs initialization arguments to startup the system chaincode
	InitArgs [][]byte

	// Chaincode can be invoked to create the actual chaincode instance
	Chaincode shim.Chaincode

	// InvokableExternal keeps track of whether
	// this system chaincode can be invoked
	// through a proposal sent to this peer
	InvokableExternal bool

	// InvokableCC2CC keeps track of whether
	// this system chaincode can be invoked
	// by way of a chaincode-to-chaincode
	// invocation
	InvokableCC2CC bool

	// Enabled a convenient switch to enable/disable system chaincode without
	// having to remove entry from importsysccs.go
	Enabled bool
}

// registerSysCC registers the given system chaincode with the peer
func (p *Provider) registerSysCC(syscc *SystemChaincode) (bool, error) {
	if !syscc.Enabled || !syscc.isWhitelisted() {
		sysccLogger.Info(fmt.Sprintf("system chaincode (%s,%s,%t) disabled", syscc.Name, syscc.Path, syscc.Enabled))
		return false, nil
	}

	// XXX This is an ugly hack, version should be tied to the chaincode instance, not he peer binary
	version := util.GetSysCCVersion()

	ccid := &ccintf.CCID{
		Name:    syscc.Name,
		Version: version,
	}
	err := p.Registrar.Register(ccid, syscc.Chaincode)
	if err != nil {
		//if the type is registered, the instance may not be... keep going
		if _, ok := err.(inproccontroller.SysCCRegisteredErr); !ok {
			errStr := fmt.Sprintf("could not register (%s,%v): %s", syscc.Path, syscc, err)
			sysccLogger.Error(errStr)
			return false, fmt.Errorf(errStr)
		}
	}

	sysccLogger.Infof("system chaincode %s(%s) registered", syscc.Name, syscc.Path)
	return true, err
}

// deploySysCC deploys the given system chaincode on a chain
func (syscc *SystemChaincode) deploySysCC(chainID string, ccprov ccprovider.ChaincodeProvider) error {
	if !syscc.Enabled || !syscc.isWhitelisted() {
		sysccLogger.Info(fmt.Sprintf("system chaincode (%s,%s) disabled", syscc.Name, syscc.Path))
		return nil
	}

	txid := util.GenerateUUID()

	ctxt := context.Background()
	if chainID != "" {
		lgr := peer.GetLedger(chainID)
		if lgr == nil {
			panic(fmt.Sprintf("syschain %s start up failure - unexpected nil ledger for channel %s", syscc.Name, chainID))
		}

		//init can do GetState (and other Get's) even if Puts cannot be
		//be handled. Need ledger for this
		ctxt2, txsim, err := ccprov.GetContext(lgr, txid)
		if err != nil {
			return err
		}

		ctxt = ctxt2

		defer txsim.Done()
	}

	chaincodeID := &pb.ChaincodeID{Path: syscc.Path, Name: syscc.Name}
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: syscc.InitArgs}}

	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ExecEnv: pb.ChaincodeDeploymentSpec_SYSTEM, ChaincodeSpec: spec}

	// XXX This is an ugly hack, version should be tied to the chaincode instance, not he peer binary
	version := util.GetSysCCVersion()

	cccid := ccprovider.NewCCContext(chainID, chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeId.Name, version, txid, true, nil, nil)

	resp, _, err := ccprov.Execute(ctxt, cccid, chaincodeDeploymentSpec)
	if err == nil && resp.Status != shim.OK {
		err = errors.New(resp.Message)
	}

	sysccLogger.Infof("system chaincode %s/%s(%s) deployed", syscc.Name, chainID, syscc.Path)

	return err
}

// deDeploySysCC stops the system chaincode and deregisters it from inproccontroller
func (syscc *SystemChaincode) deDeploySysCC(chainID string, ccprov ccprovider.ChaincodeProvider) error {
	chaincodeID := &pb.ChaincodeID{Path: syscc.Path, Name: syscc.Name}
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: syscc.InitArgs}}

	ctx := context.Background()
	// First build and get the deployment spec
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ExecEnv: pb.ChaincodeDeploymentSpec_SYSTEM, ChaincodeSpec: spec}

	// XXX This is an ugly hack, version should be tied to the chaincode instance, not he peer binary
	version := util.GetSysCCVersion()

	cccid := ccprovider.NewCCContext(chainID, syscc.Name, version, "", true, nil, nil)

	err := ccprov.Stop(ctx, cccid, chaincodeDeploymentSpec)

	return err
}

func (syscc *SystemChaincode) isWhitelisted() bool {
	chaincodes := viper.GetStringMapString("chaincode.system")
	val, ok := chaincodes[syscc.Name]
	enabled := val == "enable" || val == "true" || val == "yes"
	return ok && enabled
}
