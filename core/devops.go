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

package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"

	"encoding/asn1"
	"encoding/base64"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/container"
	crypto "github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

var devopsLogger = logging.MustGetLogger("devops")

// NewDevopsServer creates and returns a new Devops server instance.
func NewDevopsServer(coord peer.MessageHandlerCoordinator) *Devops {
	d := new(Devops)
	d.coord = coord
	d.isSecurityEnabled = viper.GetBool("security.enabled")
	d.bindingMap = &bindingMap{m: make(map[string]crypto.TransactionHandler)}
	return d
}

// bindingMap Used to store map of binding to TransactionHandler
type bindingMap struct {
	sync.RWMutex
	m map[string]crypto.TransactionHandler
}

// Devops implementation of Devops services
type Devops struct {
	coord             peer.MessageHandlerCoordinator
	isSecurityEnabled bool
	bindingMap        *bindingMap
}

func (b *bindingMap) getKeyFromBinding(binding []byte) string {
	return base64.StdEncoding.EncodeToString(binding)
}

func (b *bindingMap) addBinding(bindingToAdd []byte, txHandler crypto.TransactionHandler) {
	b.Lock()
	defer b.Unlock()
	key := b.getKeyFromBinding(bindingToAdd)
	b.m[key] = txHandler
}

func (b *bindingMap) getTxHandlerForBinding(binding []byte) (crypto.TransactionHandler, error) {
	b.Lock()
	defer b.Unlock()
	key := b.getKeyFromBinding(binding)
	txHandler, ok := b.m[key]
	if ok != true {
		// TXhandler not found by key, return error
		return nil, fmt.Errorf("Transaction handler not found for binding key = %s", key)
	}
	return txHandler, nil
}

// Login establishes the security context with the Devops service
func (d *Devops) Login(ctx context.Context, secret *pb.Secret) (*pb.Response, error) {
	if err := crypto.RegisterClient(secret.EnrollId, nil, secret.EnrollId, secret.EnrollSecret); nil != err {
		return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
	}
	return &pb.Response{Status: pb.Response_SUCCESS}, nil

	// TODO: Handle timeout and expiration
}

// Build builds the supplied chaincode image
func (*Devops) Build(context context.Context, spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	mode := viper.GetString("chaincode.mode")
	var codePackageBytes []byte
	if mode != chaincode.DevModeUserRunsChaincode {
		devopsLogger.Debugf("Received build request for chaincode spec: %v", spec)
		if err := CheckSpec(spec); err != nil {
			return nil, err
		}

		vm, err := container.NewVM()
		if err != nil {
			return nil, fmt.Errorf("Error getting vm")
		}

		codePackageBytes, err = vm.BuildChaincodeContainer(spec)
		if err != nil {
			devopsLogger.Error(fmt.Sprintf("%s", err))
			return nil, err
		}
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

// get chaincode bytes
func (*Devops) getChaincodeBytes(context context.Context, spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	mode := viper.GetString("chaincode.mode")
	var codePackageBytes []byte
	if mode != chaincode.DevModeUserRunsChaincode {
		devopsLogger.Debugf("Received build request for chaincode spec: %v", spec)
		var err error
		if err = CheckSpec(spec); err != nil {
			return nil, err
		}

		codePackageBytes, err = container.GetChaincodePackageBytes(spec)
		if err != nil {
			err = fmt.Errorf("Error getting chaincode package bytes: %s", err)
			devopsLogger.Error(fmt.Sprintf("%s", err))
			return nil, err
		}
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

// Deploy deploys the supplied chaincode image to the validators through a transaction
func (d *Devops) Deploy(ctx context.Context, spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	// get the deployment spec
	chaincodeDeploymentSpec, err := d.getChaincodeBytes(ctx, spec)

	if err != nil {
		devopsLogger.Error(fmt.Sprintf("Error deploying chaincode spec: %v\n\n error: %s", spec, err))
		return nil, err
	}

	// Now create the Transactions message and send to Peer.

	transID := chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name

	var tx *pb.Transaction
	var sec crypto.Client

	if peer.SecurityEnabled() {
		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debugf("Initializing secure devops using context %s", spec.SecureContext)
		}
		sec, err = crypto.InitClient(spec.SecureContext, nil)
		defer crypto.CloseClient(sec)

		// remove the security context since we are no longer need it down stream
		spec.SecureContext = ""

		if nil != err {
			return nil, err
		}

		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debugf("Creating secure transaction %s", transID)
		}
		tx, err = sec.NewChaincodeDeployTransaction(chaincodeDeploymentSpec, transID, spec.Attributes...)
		if nil != err {
			return nil, err
		}
	} else {
		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debugf("Creating deployment transaction (%s)", transID)
		}
		tx, err = pb.NewChaincodeDeployTransaction(chaincodeDeploymentSpec, transID)
		if err != nil {
			return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
		}
	}

	if devopsLogger.IsEnabledFor(logging.DEBUG) {
		devopsLogger.Debugf("Sending deploy transaction (%s) to validator", tx.Txid)
	}
	resp := d.coord.ExecuteTransaction(tx)
	if resp.Status == pb.Response_FAILURE {
		err = fmt.Errorf(string(resp.Msg))
	}

	return chaincodeDeploymentSpec, err
}

func (d *Devops) invokeOrQuery(ctx context.Context, chaincodeInvocationSpec *pb.ChaincodeInvocationSpec, attributes []string, invoke bool) (*pb.Response, error) {

	if chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name == "" {
		return nil, fmt.Errorf("name not given for invoke/query")
	}

	// Now create the Transactions message and send to Peer.
	var customIDgenAlg = strings.ToLower(chaincodeInvocationSpec.IdGenerationAlg)
	var id string
	var generr error
	if invoke {
		if customIDgenAlg != "" {
			ctorbytes, merr := asn1.Marshal(*chaincodeInvocationSpec.ChaincodeSpec.CtorMsg)
			if merr != nil {
				return nil, fmt.Errorf("Error marshalling constructor: %s", merr)
			}
			id, generr = util.GenerateIDWithAlg(customIDgenAlg, ctorbytes)
			if generr != nil {
				return nil, generr
			}
		} else {
			id = util.GenerateUUID()
		}
	} else {
		id = util.GenerateUUID()
	}
	devopsLogger.Infof("Transaction ID: %v", id)
	var transaction *pb.Transaction
	var err error
	var sec crypto.Client
	if peer.SecurityEnabled() {
		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debugf("Initializing secure devops using context %s", chaincodeInvocationSpec.ChaincodeSpec.SecureContext)
		}
		sec, err = crypto.InitClient(chaincodeInvocationSpec.ChaincodeSpec.SecureContext, nil)
		defer crypto.CloseClient(sec)
		// remove the security context since we are no longer need it down stream
		chaincodeInvocationSpec.ChaincodeSpec.SecureContext = ""
		if nil != err {
			return nil, err
		}
	}

	transaction, err = d.createExecTx(chaincodeInvocationSpec, attributes, id, invoke, sec)
	if err != nil {
		return nil, err
	}
	if devopsLogger.IsEnabledFor(logging.DEBUG) {
		devopsLogger.Debugf("Sending invocation transaction (%s) to validator", transaction.Txid)
	}
	resp := d.coord.ExecuteTransaction(transaction)
	if resp.Status == pb.Response_FAILURE {
		err = fmt.Errorf(string(resp.Msg))
	} else {
		if !invoke && nil != sec && viper.GetBool("security.privacy") {
			if resp.Msg, err = sec.DecryptQueryResult(transaction, resp.Msg); nil != err {
				devopsLogger.Errorf("Failed decrypting query transaction result %s", string(resp.Msg[:]))
				//resp = &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}
			}
		}
	}
	return resp, err
}

func (d *Devops) createExecTx(spec *pb.ChaincodeInvocationSpec, attributes []string, uuid string, invokeTx bool, sec crypto.Client) (*pb.Transaction, error) {
	var tx *pb.Transaction
	var err error

	//TODO What should we do with the attributes
	if nil != sec {
		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debugf("Creating secure invocation transaction %s", uuid)
		}
		if invokeTx {
			tx, err = sec.NewChaincodeExecute(spec, uuid, attributes...)
		} else {
			tx, err = sec.NewChaincodeQuery(spec, uuid, attributes...)
		}
		if nil != err {
			return nil, err
		}
	} else {
		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debugf("Creating invocation transaction (%s)", uuid)
		}
		var t pb.Transaction_Type
		if invokeTx {
			t = pb.Transaction_CHAINCODE_INVOKE
		} else {
			t = pb.Transaction_CHAINCODE_QUERY
		}
		tx, err = pb.NewChaincodeExecute(spec, uuid, t)
		if nil != err {
			return nil, err
		}
	}
	return tx, nil
}

// Invoke performs the supplied invocation on the specified chaincode through a transaction
func (d *Devops) Invoke(ctx context.Context, chaincodeInvocationSpec *pb.ChaincodeInvocationSpec) (*pb.Response, error) {
	return d.invokeOrQuery(ctx, chaincodeInvocationSpec, chaincodeInvocationSpec.ChaincodeSpec.Attributes, true)
}

// Query performs the supplied query on the specified chaincode through a transaction
func (d *Devops) Query(ctx context.Context, chaincodeInvocationSpec *pb.ChaincodeInvocationSpec) (*pb.Response, error) {
	return d.invokeOrQuery(ctx, chaincodeInvocationSpec, chaincodeInvocationSpec.ChaincodeSpec.Attributes, false)
}

// CheckSpec to see if chaincode resides within current package capture for language.
func CheckSpec(spec *pb.ChaincodeSpec) error {
	// Don't allow nil value
	if spec == nil {
		return errors.New("Expected chaincode specification, nil received")
	}

	platform, err := platforms.Find(spec.Type)
	if err != nil {
		return fmt.Errorf("Failed to determine platform type: %s", err)
	}

	return platform.ValidateSpec(spec)
}

// EXP_GetApplicationTCert retrieves an application TCert for the supplied user
func (d *Devops) EXP_GetApplicationTCert(ctx context.Context, secret *pb.Secret) (*pb.Response, error) {
	var sec crypto.Client
	var err error

	if d.isSecurityEnabled {
		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debug("Initializing secure devops using context %s", secret.EnrollId)
		}
		sec, err = crypto.InitClient(secret.EnrollId, nil)
		defer crypto.CloseClient(sec)

		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}

		devopsLogger.Debug("Getting TCert for id: %s", secret.EnrollId)
		tcertHandler, err := sec.GetTCertificateHandlerNext()
		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}
		certDER := tcertHandler.GetCertificate()
		return &pb.Response{Status: pb.Response_SUCCESS, Msg: certDER}, nil
	}
	devopsLogger.Warning("Security NOT enabled")
	return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte("Security NOT enabled")}, nil
	// TODO: Handle timeout and expiration
}

// EXP_PrepareForTx prepares a binding/TXHandler pair to be used in subsequent TX
func (d *Devops) EXP_PrepareForTx(ctx context.Context, secret *pb.Secret) (*pb.Response, error) {
	var sec crypto.Client
	var err error
	var txHandler crypto.TransactionHandler
	var binding []byte

	if d.isSecurityEnabled {
		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debug("Initializing secure devops using context %s", secret.EnrollId)
		}
		sec, err = crypto.InitClient(secret.EnrollId, nil)
		defer crypto.CloseClient(sec)

		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}

		devopsLogger.Debug("Getting TXHandler for id: %s", secret.EnrollId)
		tcertHandler, err := sec.GetTCertificateHandlerNext()
		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}
		txHandler, err = tcertHandler.GetTransactionHandler()
		binding, err = txHandler.GetBinding()
		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}
		// Now add to binding map
		d.bindingMap.addBinding(binding, txHandler)
		return &pb.Response{Status: pb.Response_SUCCESS, Msg: binding}, nil
	}
	devopsLogger.Warning("Security NOT enabled")
	return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte("Security NOT enabled")}, nil
	// TODO: Handle timeout and expiration
}

// EXP_ProduceSigma produces a sigma as []byte and returns in response
func (d *Devops) EXP_ProduceSigma(ctx context.Context, sigmaInput *pb.SigmaInput) (*pb.Response, error) {
	var sec crypto.Client
	var err error
	var sigma []byte
	secret := sigmaInput.Secret

	type RBACMetatdata struct {
		Cert  []byte
		Sigma []byte
	}

	if d.isSecurityEnabled {
		if devopsLogger.IsEnabledFor(logging.DEBUG) {
			devopsLogger.Debug("Initializing secure devops using context %s", secret.EnrollId)
		}
		sec, err = crypto.InitClient(secret.EnrollId, nil)
		defer crypto.CloseClient(sec)

		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}

		devopsLogger.Debug("Getting TCertHandler for id: %s, from DER = %s", secret.EnrollId, sigmaInput.AppTCert)
		tcertHandler, err := sec.GetTCertificateHandlerFromDER(sigmaInput.AppTCert)
		//tcertHandler, err := sec.GetTCertificateHandlerNext()
		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(fmt.Errorf("Error getting TCertHandler from DER:  %s", err).Error())}, nil
		}
		tcert := sigmaInput.AppTCert //tcertHandler.GetCertificate()
		sigma, err = tcertHandler.Sign(append(tcert, sigmaInput.Data...))
		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(fmt.Errorf("Error signing with TCertHandler from DER:  %s", err).Error())}, nil
		}
		// Produce the SigmaOutput
		asn1Encoding, err := asn1.Marshal(RBACMetatdata{Cert: tcert, Sigma: sigma})
		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}
		sigmaOutput := &pb.SigmaOutput{Tcert: tcert, Sigma: sigma, Asn1Encoding: asn1Encoding}
		sigmaOutputBytes, err := proto.Marshal(sigmaOutput)
		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}
		return &pb.Response{Status: pb.Response_SUCCESS, Msg: sigmaOutputBytes}, nil
	}
	devopsLogger.Warning("Security NOT enabled")
	return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte("Security NOT enabled")}, nil

}

// EXP_ExecuteWithBinding executes a transaction with a specific binding/TXHandler
func (d *Devops) EXP_ExecuteWithBinding(ctx context.Context, executeWithBinding *pb.ExecuteWithBinding) (*pb.Response, error) {

	if d.isSecurityEnabled {
		devopsLogger.Debug("Getting TxHandler for binding")

		txHandler, err := d.bindingMap.getTxHandlerForBinding(executeWithBinding.Binding)

		if nil != err {
			return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
		}

		ctorbytes, merr := asn1.Marshal(*executeWithBinding.ChaincodeInvocationSpec.ChaincodeSpec.CtorMsg)
		if merr != nil {
			return nil, fmt.Errorf("Error marshalling constructor: %s", err)
		}
		tid, generr := util.GenerateIDWithAlg("", ctorbytes)
		if generr != nil {
			return nil, fmt.Errorf("Error: cannot generate TX ID (executing with binding)")
		}

		tx, err := txHandler.NewChaincodeExecute(executeWithBinding.ChaincodeInvocationSpec, tid)
		if err != nil {
			return nil, fmt.Errorf("Error creating executing with binding:  %s", err)
		}

		return d.coord.ExecuteTransaction(tx), nil
		//return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte("NOT IMPLEMENTED")}, nil

		//return &pb.Response{Status: pb.Response_SUCCESS, Msg: sigmaOutputBytes}, nil
	}
	devopsLogger.Warning("Security NOT enabled")
	return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte("Security NOT enabled")}, nil
}
