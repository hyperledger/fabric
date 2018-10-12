/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
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

	// Chaincode holds the actual chaincode instance
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

type SysCCWrapper struct {
	SCC *SystemChaincode
}

func (sccw *SysCCWrapper) Name() string              { return sccw.SCC.Name }
func (sccw *SysCCWrapper) Path() string              { return sccw.SCC.Path }
func (sccw *SysCCWrapper) InitArgs() [][]byte        { return sccw.SCC.InitArgs }
func (sccw *SysCCWrapper) Chaincode() shim.Chaincode { return sccw.SCC.Chaincode }
func (sccw *SysCCWrapper) InvokableExternal() bool   { return sccw.SCC.InvokableExternal }
func (sccw *SysCCWrapper) InvokableCC2CC() bool      { return sccw.SCC.InvokableCC2CC }
func (sccw *SysCCWrapper) Enabled() bool             { return sccw.SCC.Enabled }

type SelfDescribingSysCC interface {
	//Unique name of the system chaincode
	Name() string

	//Path to the system chaincode; currently not used
	Path() string

	//InitArgs initialization arguments to startup the system chaincode
	InitArgs() [][]byte

	// Chaincode returns the underlying chaincode
	Chaincode() shim.Chaincode

	// InvokableExternal keeps track of whether
	// this system chaincode can be invoked
	// through a proposal sent to this peer
	InvokableExternal() bool

	// InvokableCC2CC keeps track of whether
	// this system chaincode can be invoked
	// by way of a chaincode-to-chaincode
	// invocation
	InvokableCC2CC() bool

	// Enabled a convenient switch to enable/disable system chaincode without
	// having to remove entry from importsysccs.go
	Enabled() bool
}

// registerSysCC registers the given system chaincode with the peer
func (p *Provider) registerSysCC(syscc SelfDescribingSysCC) (bool, error) {
	if !syscc.Enabled() || !isWhitelisted(syscc) {
		sysccLogger.Info(fmt.Sprintf("system chaincode (%s,%s,%t) disabled", syscc.Name(), syscc.Path(), syscc.Enabled()))
		return false, nil
	}

	// XXX This is an ugly hack, version should be tied to the chaincode instance, not he peer binary
	version := util.GetSysCCVersion()

	ccid := &ccintf.CCID{
		Name:    syscc.Name(),
		Version: version,
	}
	err := p.Registrar.Register(ccid, syscc.Chaincode())
	if err != nil {
		//if the type is registered, the instance may not be... keep going
		if _, ok := err.(inproccontroller.SysCCRegisteredErr); !ok {
			errStr := fmt.Sprintf("could not register (%s,%v): %s", syscc.Path(), syscc, err)
			sysccLogger.Error(errStr)
			return false, fmt.Errorf(errStr)
		}
	}

	sysccLogger.Infof("system chaincode %s(%s) registered", syscc.Name(), syscc.Path())
	return true, err
}

// deploySysCC deploys the given system chaincode on a chain
func deploySysCC(chainID string, ccprov ccprovider.ChaincodeProvider, syscc SelfDescribingSysCC) error {
	if !syscc.Enabled() || !isWhitelisted(syscc) {
		sysccLogger.Info(fmt.Sprintf("system chaincode (%s,%s) disabled", syscc.Name(), syscc.Path()))
		return nil
	}

	txid := util.GenerateUUID()

	// Note, this structure is barely initialized,
	// we omit the history query executor, the proposal
	// and the signed proposal
	txParams := &ccprovider.TransactionParams{
		TxID:      txid,
		ChannelID: chainID,
	}

	if chainID != "" {
		lgr := peer.GetLedger(chainID)
		if lgr == nil {
			panic(fmt.Sprintf("syschain %s start up failure - unexpected nil ledger for channel %s", syscc.Name(), chainID))
		}

		txsim, err := lgr.NewTxSimulator(txid)
		if err != nil {
			return err
		}

		txParams.TXSimulator = txsim
		defer txsim.Done()
	}

	chaincodeID := &pb.ChaincodeID{Path: syscc.Path(), Name: syscc.Name()}
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: syscc.InitArgs()}}

	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ExecEnv: pb.ChaincodeDeploymentSpec_SYSTEM, ChaincodeSpec: spec}

	// XXX This is an ugly hack, version should be tied to the chaincode instance, not he peer binary
	version := util.GetSysCCVersion()

	cccid := &ccprovider.CCContext{
		Name:    chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeId.Name,
		Version: version,
	}

	resp, _, err := ccprov.ExecuteLegacyInit(txParams, cccid, chaincodeDeploymentSpec)
	if err == nil && resp.Status != shim.OK {
		err = errors.New(resp.Message)
	}

	sysccLogger.Infof("system chaincode %s/%s(%s) deployed", syscc.Name(), chainID, syscc.Path())

	return err
}

// deDeploySysCC stops the system chaincode and deregisters it from inproccontroller
func deDeploySysCC(chainID string, ccprov ccprovider.ChaincodeProvider, syscc SelfDescribingSysCC) error {
	// XXX This is an ugly hack, version should be tied to the chaincode instance, not he peer binary
	version := util.GetSysCCVersion()

	ccci := &ccprovider.ChaincodeContainerInfo{
		Type:          "GOLANG",
		Name:          syscc.Name(),
		Path:          syscc.Path(),
		Version:       version,
		ContainerType: inproccontroller.ContainerType,
	}

	err := ccprov.Stop(ccci)

	return err
}

func isWhitelisted(syscc SelfDescribingSysCC) bool {
	chaincodes := viper.GetStringMapString("chaincode.system")
	val, ok := chaincodes[syscc.Name()]
	enabled := val == "enable" || val == "true" || val == "yes"
	return ok && enabled
}
