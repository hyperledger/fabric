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

package lccc

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	ledger "github.com/hyperledger/fabric/core/ledgernext"
	"github.com/hyperledger/fabric/core/ledgernext/kvledger"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"golang.org/x/net/context"
)

//The life cycle system chaincode manages chaincodes deployed
//on this peer. It manages chaincodes via Invoke proposals.
//     "Args":["deploy",<ChaincodeDeploymentSpec>]
//     "Args":["upgrade",<ChaincodeDeploymentSpec>]
//     "Args":["stop",<ChaincodeInvocationSpec>]
//     "Args":["start",<ChaincodeInvocationSpec>]

var logger = logging.MustGetLogger("lccc")

const (
	//CHAINCODETABLE prefix for chaincode tables
	CHAINCODETABLE = "chaincodes"

	//chaincode lifecyle commands

	//DEPLOY deploy command
	DEPLOY = "deploy"

	//chaincode query commands

	//GETCCINFO get chaincode
	GETCCINFO = "getid"

	//GETDEPSPEC get ChaincodeDeploymentSpec
	GETDEPSPEC = "getdepspec"
)

//---------- the LCCC -----------------

// LifeCycleSysCC implements chaincode lifecycle and policies aroud it
type LifeCycleSysCC struct {
}

//----------------errors---------------

//AlreadyRegisteredErr Already registered error
type AlreadyRegisteredErr string

func (f AlreadyRegisteredErr) Error() string {
	return fmt.Sprintf("%s already registered", string(f))
}

//InvalidFunctionErr invalid function error
type InvalidFunctionErr string

func (f InvalidFunctionErr) Error() string {
	return fmt.Sprintf("invalid function to lccc %s", string(f))
}

//InvalidArgsLenErr invalid arguments length error
type InvalidArgsLenErr int

func (i InvalidArgsLenErr) Error() string {
	return fmt.Sprintf("invalid number of argument to lccc %d", int(i))
}

//InvalidArgsErr invalid arguments error
type InvalidArgsErr int

func (i InvalidArgsErr) Error() string {
	return fmt.Sprintf("invalid argument (%d) to lccc", int(i))
}

//TXExistsErr transaction exists error
type TXExistsErr string

func (t TXExistsErr) Error() string {
	return fmt.Sprintf("transaction exists %s", string(t))
}

//TXNotFoundErr transaction not found error
type TXNotFoundErr string

func (t TXNotFoundErr) Error() string {
	return fmt.Sprintf("transaction not found %s", string(t))
}

//InvalidDeploymentSpecErr invalide chaincode deployment spec error
type InvalidDeploymentSpecErr string

func (f InvalidDeploymentSpecErr) Error() string {
	return fmt.Sprintf("Invalid deployment spec : %s", string(f))
}

//ChaincodeExistsErr chaincode exists error
type ChaincodeExistsErr string

func (t ChaincodeExistsErr) Error() string {
	return fmt.Sprintf("Chaincode exists %s", string(t))
}

//InvalidChainNameErr invalid chain name error
type InvalidChainNameErr string

func (f InvalidChainNameErr) Error() string {
	return fmt.Sprintf("invalid chain name %s", string(f))
}

//InvalidChaincodeNameErr invalid chaincode name error
type InvalidChaincodeNameErr string

func (f InvalidChaincodeNameErr) Error() string {
	return fmt.Sprintf("invalid chain code name %s", string(f))
}

//-------------- helper functions ------------------
//create the table to maintain list of chaincodes maintained in this
//blockchain.
func (lccc *LifeCycleSysCC) createChaincodeTable(stub shim.ChaincodeStubInterface, cctable string) error {
	// Create table one
	var colDefs []*shim.ColumnDefinition
	nameColDef := shim.ColumnDefinition{Name: "name",
		Type: shim.ColumnDefinition_STRING, Key: true}
	versColDef := shim.ColumnDefinition{Name: "version",
		Type: shim.ColumnDefinition_INT32, Key: false}

	//QUESTION - Should code be separately maintained ?
	codeDef := shim.ColumnDefinition{Name: "code",
		Type: shim.ColumnDefinition_BYTES, Key: false}
	colDefs = append(colDefs, &nameColDef)
	colDefs = append(colDefs, &versColDef)
	colDefs = append(colDefs, &codeDef)
	return stub.CreateTable(cctable, colDefs)
}

//register create the chaincode table. name can be used to different
//tables of chaincodes. This would provide the way to associate chaincodes
//with chains(and ledgers)
func (lccc *LifeCycleSysCC) register(stub shim.ChaincodeStubInterface, name string) error {
	ccname := CHAINCODETABLE + "-" + name

	row, err := stub.GetTable(ccname)
	if err == nil && row != nil { //table exists, do nothing
		return AlreadyRegisteredErr(name)
	}

	//there may be other err's but assume "not exists". Anything
	//more serious than that bound to show up
	err = lccc.createChaincodeTable(stub, ccname)

	return err
}

//create the chaincode on the given chain
func (lccc *LifeCycleSysCC) createChaincode(stub shim.ChaincodeStubInterface, chainname string, ccname string, cccode []byte) (*shim.Row, error) {
	var columns []*shim.Column

	nameCol := shim.Column{Value: &shim.Column_String_{String_: ccname}}
	versCol := shim.Column{Value: &shim.Column_Int32{Int32: 0}}
	codeCol := shim.Column{Value: &shim.Column_Bytes{Bytes: cccode}}

	columns = append(columns, &nameCol)
	columns = append(columns, &versCol)
	columns = append(columns, &codeCol)

	row := &shim.Row{Columns: columns}
	_, err := stub.InsertRow(CHAINCODETABLE+"-"+chainname, *row)
	if err != nil {
		return nil, fmt.Errorf("insertion of chaincode failed. %s", err)
	}
	return row, nil
}

//checks for existence of chaincode on the given chain
func (lccc *LifeCycleSysCC) getChaincode(stub shim.ChaincodeStubInterface, chainname string, ccname string) (shim.Row, bool, error) {
	var columns []shim.Column
	nameCol := shim.Column{Value: &shim.Column_String_{String_: ccname}}
	columns = append(columns, nameCol)

	row, err := stub.GetRow(CHAINCODETABLE+"-"+chainname, columns)
	if err != nil {
		return shim.Row{}, false, err
	}

	if len(row.Columns) > 0 {
		return row, true, nil
	}
	return row, false, nil
}

//getChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec given args
func (lccc *LifeCycleSysCC) getChaincodeDeploymentSpec(code []byte) (*pb.ChaincodeDeploymentSpec, error) {
	cds := &pb.ChaincodeDeploymentSpec{}

	err := proto.Unmarshal(code, cds)
	if err != nil {
		return nil, InvalidDeploymentSpecErr(err.Error())
	}

	return cds, nil
}

//do access control
func (lccc *LifeCycleSysCC) acl(stub shim.ChaincodeStubInterface, chainname chaincode.ChainName, cds *pb.ChaincodeDeploymentSpec) error {
	return nil
}

//check validity of chain name
func (lccc *LifeCycleSysCC) isValidChainName(chainname string) bool {
	//TODO we probably need more checks and have
	if chainname == "" {
		return false
	}
	return true
}

//check validity of chaincode name
func (lccc *LifeCycleSysCC) isValidChaincodeName(chaincodename string) bool {
	//TODO we probably need more checks and have
	if chaincodename == "" {
		return false
	}
	return true
}

//deploy the chaincode on to the chain
func (lccc *LifeCycleSysCC) deploy(stub shim.ChaincodeStubInterface, chainname string, cds *pb.ChaincodeDeploymentSpec) error {
	//TODO : this needs to be converted to another data structure to be handled
	//       by the chaincode framework (which currently handles "Transaction")
	t, err := lccc.toTransaction(cds)
	if err != nil {
		return fmt.Errorf("could not convert proposal to transaction %s", err)
	}

	//if unit testing, just return..we cannot do the actual deploy
	if _, ismock := stub.(*shim.MockStub); ismock {
		//we got this far just stop short of actual deploy for test purposes
		return nil
	}

	ctxt := context.Background()

	//TODO - we are in the LCCC chaincode simulating an "invoke" to deploy another
	//chaincode. Deploying the chaincode and calling its "init" involves ledger access
	//for the called chaincode - ie, it needs to undergo state simulatio as well.
	//How do we handle the simulation for the called called chaincode ?
	//    1) don't allow state initialization on deploy
	//    2) combine both LCCC and the called chaincodes into one RW set
	//    3) just drop the second
	lgr := kvledger.GetLedger(chainname)

	var dummytxsim ledger.TxSimulator

	if dummytxsim, err = lgr.NewTxSimulator(); err != nil {
		return fmt.Errorf("Could not get simulator for %s", chainname)
	}

	ctxt = context.WithValue(ctxt, chaincode.TXSimulatorKey, dummytxsim)

	//TODO - create chaincode support for chainname, for now use DefaultChain
	chaincodeSupport := chaincode.GetChain(chaincode.ChainName(chainname))

	_, err = chaincodeSupport.Deploy(ctxt, t)
	if err != nil {
		return fmt.Errorf("Failed to deploy chaincode spec(%s)", err)
	}

	//launch and wait for ready
	_, _, err = chaincodeSupport.Launch(ctxt, t)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	//stop now that we are done
	chaincodeSupport.Stop(ctxt, cds)

	return nil
}

//this implements "deploy" Invoke transaction
func (lccc *LifeCycleSysCC) executeDeploy(stub shim.ChaincodeStubInterface, chainname string, code []byte) error {
	//lazy creation of chaincode table for chainname...its possible
	//there are chains without chaincodes
	if err := lccc.register(stub, chainname); err != nil {
		//if its already registered, ok... proceed
		if _, ok := err.(AlreadyRegisteredErr); !ok {
			return err
		}
	}

	cds, err := lccc.getChaincodeDeploymentSpec(code)

	if err != nil {
		return err
	}

	if !lccc.isValidChaincodeName(cds.ChaincodeSpec.ChaincodeID.Name) {
		return InvalidChaincodeNameErr(cds.ChaincodeSpec.ChaincodeID.Name)
	}

	if err = lccc.acl(stub, chaincode.DefaultChain, cds); err != nil {
		return err
	}

	_, exists, err := lccc.getChaincode(stub, chainname, cds.ChaincodeSpec.ChaincodeID.Name)
	if exists {
		return ChaincodeExistsErr(cds.ChaincodeSpec.ChaincodeID.Name)
	}

	/**TODO - this is done in the endorser service for now so we can
		 * collect all state changes under one TXSim. Revisit this ...
	         * maybe this *is* the right solution
		 *if err = lccc.deploy(stub, chainname, cds); err != nil {
		 *	return err
		 *}
		 **/

	_, err = lccc.createChaincode(stub, chainname, cds.ChaincodeSpec.ChaincodeID.Name, code)

	return err
}

//TODO - this is temporary till we use Transaction in chaincode code
func (lccc *LifeCycleSysCC) toTransaction(cds *pb.ChaincodeDeploymentSpec) (*pb.Transaction, error) {
	return pb.NewChaincodeDeployTransaction(cds, cds.ChaincodeSpec.ChaincodeID.Name)
}

//-------------- the chaincode stub interface implementation ----------

//Init does nothing
func (lccc *LifeCycleSysCC) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	return nil, nil
}

// Invoke implements lifecycle functions "deploy", "start", "stop", "upgrade".
// Deploy's arguments -  {[]byte("deploy"), []byte(<chainname>), <unmarshalled pb.ChaincodeDeploymentSpec>}
//
// Invoke also implements some query-like functions
// Get chaincode arguments -  {[]byte("getid"), []byte(<chainname>), []byte(<chaincodename>)}
func (lccc *LifeCycleSysCC) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	args := stub.GetArgs()
	if len(args) < 1 {
		return nil, InvalidArgsLenErr(len(args))
	}

	function := string(args[0])

	switch function {
	case DEPLOY:
		if len(args) != 3 {
			return nil, InvalidArgsLenErr(len(args))
		}

		//chain the chaincode shoud be associated with. It
		//should be created with a register call
		chainname := string(args[1])

		if !lccc.isValidChainName(chainname) {
			return nil, InvalidChainNameErr(chainname)
		}

		//bytes corresponding to deployment spec
		code := args[2]

		err := lccc.executeDeploy(stub, chainname, code)

		return nil, err
	case GETCCINFO, GETDEPSPEC:
		if len(args) != 3 {
			return nil, InvalidArgsLenErr(len(args))
		}

		chain := string(args[1])
		ccname := string(args[2])
		//get chaincode given <chain, name>

		ccrow, exists, _ := lccc.getChaincode(stub, chain, ccname)
		if !exists {
			logger.Debug("ChaincodeID [%s/%s] does not exist", chain, ccname)
			return nil, TXNotFoundErr(chain + "/" + ccname)
		}

		if function == GETCCINFO {
			return []byte(ccrow.Columns[1].GetString_()), nil
		}
		return ccrow.Columns[2].GetBytes(), nil
	}

	return nil, InvalidFunctionErr(function)
}

// Query is no longer implemented. Will be removed
func (lccc *LifeCycleSysCC) Query(stub shim.ChaincodeStubInterface) ([]byte, error) {
	return nil, nil
}
