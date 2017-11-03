/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rscc

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const (
	//CHANNEL name
	CHANNEL = "channel"

	//POLICY for the channel
	POLICY = "policy"

	//Init Errors (for UT)

	//NOCHANNEL channel not found
	NOCHANNEL = "nochannel"

	//NOPOLICY policy not found for channel
	NOPOLICY = "nopolicy"

	//BADPOLICY bad policy
	BADPOLICY = "badpolicy"
)

//the basic policyProvider for the channel, consists of a
//default and rscc policy providers
type policyProvider struct {
	//maybe we should use a separate default policy provider
	//than the only from aclmgmt (which is 1.0 defaults)
	defaultProvider aclmgmt.ACLProvider

	rsccProvider rsccPolicyProvider
}

//Rscc SCC implementing resouce->Policy mapping for the fabric
type Rscc struct {
	sync.RWMutex

	//the cache of RSCC policies for all the channels
	policyCache map[string]*policyProvider
}

var rsccLogger = flogging.MustGetLogger("rscc")

//NewRscc get an initialzed new Rscc
func NewRscc() *Rscc {
	return &Rscc{policyCache: make(map[string]*policyProvider)}
}

//--------- errors ---------

//NoPolicyProviderInCache in cache for channel
type NoPolicyProviderInCache string

func (e NoPolicyProviderInCache) Error() string {
	return fmt.Sprintf("cannot find policy provider in cache for channel %s", string(e))
}

//PolicyProviderNotFound for channel
type PolicyProviderNotFound string

func (e PolicyProviderNotFound) Error() string {
	return fmt.Sprintf("cannot find policy provider for channel %s", string(e))
}

//-------- ACLProvider interface ------

//CheckACL rscc implements AClProvider's CheckACL interface. This is the key interface
//   . CheckACL works off the cache
//   . CheckACL uses two providers - the RSCC provider from channel config and default provider
//     that implements 1.0 functions
//   . If a resource in RSCC Provider it'll use the policy defined there. Otherwise it'll defer
//     to default provider
func (rscc *Rscc) CheckACL(resName string, channelID string, idinfo interface{}) error {
	rsccLogger.Debugf("rscc acl check(%s, %s)", resName, channelID)
	rscc.RLock()
	defer rscc.RUnlock()
	pp := rscc.policyCache[channelID]
	if pp == nil {
		return NoPolicyProviderInCache(channelID)
	}

	//found policyProvider
	if pp.rsccProvider != nil {
		//get the policy mapping if any
		if policyName := pp.rsccProvider.GetPolicyName(resName); policyName != "" {
			return pp.rsccProvider.CheckACL(policyName, idinfo)
		}
	}

	//try default provider
	if pp.defaultProvider != nil {
		return pp.defaultProvider.CheckACL(resName, channelID, idinfo)
	}

	return PolicyProviderNotFound(channelID)
}

//GenerateSimulationResults called to add config state. Currently only handles "join" requests.
//Note that this is just a ledger hook and does not modify RSCC data structures
func (rscc *Rscc) GenerateSimulationResults(txEnv *common.Envelope, sim ledger.TxSimulator) error {
	//should never happen, but check anyway
	if txEnv == nil || sim == nil {
		return fmt.Errorf("nil parameters")
	}

	payl := &common.Payload{}
	if err := proto.Unmarshal(txEnv.Payload, payl); err != nil {
		rsccLogger.Errorf("error on Payload unmarshal  %s", err)
		return err
	}

	cenv := &common.ConfigEnvelope{}
	if err := proto.Unmarshal(payl.Data, cenv); err != nil {
		rsccLogger.Errorf("error on ConfigEnvelope unmarshal  %s", err)
		return err
	}

	if cenv.Config.Sequence != 1 {
		rsccLogger.Errorf("ignore non genesis block config updates (%d) for modifying resource policies", cenv.Config.Sequence)
		return nil
	}

	if cenv.LastUpdate == nil {
		rsccLogger.Errorf("nil LastUpdate")
		return fmt.Errorf("nil LastUpdate")
	}

	if err := proto.Unmarshal(cenv.LastUpdate.Payload, payl); err != nil {
		rsccLogger.Errorf("error on LastUpdate  unmarshal  %s", err)
		return err
	}

	coe := &common.ConfigUpdateEnvelope{}
	if err := proto.Unmarshal(payl.Data, coe); err != nil {
		rsccLogger.Errorf("error on ConfigUpdateEnvelope unmarshal  %s", err)
		return err
	}

	cup := &common.ConfigUpdate{}
	if err := proto.Unmarshal(coe.ConfigUpdate, cup); err != nil {
		rsccLogger.Errorf("error on ConfigUpdate unmarshal  %s", err)
		return err
	}

	rsccLogger.Debugf("RSCC processing config tx for channel %s", cup.ChannelId)

	//preparation for extracting RWSet from transaction
	if err := sim.SetState("rscc", CHANNEL, []byte(cup.ChannelId)); err != nil {
		rsccLogger.Errorf("error on setting channel state %s", err)
		return err
	}

	iso := cup.IsolatedData
	if err := sim.SetState("rscc", POLICY, iso[pb.RSCCSeedDataKey]); err != nil {
		rsccLogger.Errorf("error on setting policy state %s", err)
		return err
	}

	return nil
}

//---------- misc functions ---------
func (rscc *Rscc) putPolicyProvider(channel string, pp *policyProvider) {
	rscc.Lock()
	defer rscc.Unlock()
	rscc.policyCache[channel] = pp
}

func (rscc *Rscc) setPolicyProvider(channel string, defProv aclmgmt.ACLProvider, rsccProv rsccPolicyProvider) {
	pp := &policyProvider{defProv, rsccProv}
	rscc.putPolicyProvider(channel, pp)
}

func (rscc *Rscc) getEnvelopeFromConfig(channel string, cfgb []byte) (*common.Envelope, error) {
	cfg := &common.Config{}
	err := proto.Unmarshal(cfgb, cfg)
	if err != nil {
		return nil, err
	}
	cenv := &common.ConfigEnvelope{Config: cfg}

	data, err := proto.Marshal(cenv)
	if err != nil {
		return nil, err
	}

	chdr, _ := proto.Marshal(&common.ChannelHeader{
		ChannelId: channel,
		Type:      int32(common.HeaderType_CONFIG),
	})
	payl, _ := proto.Marshal(&common.Payload{
		Header: &common.Header{
			ChannelHeader: chdr,
		},
		Data: data,
	})

	return &common.Envelope{Payload: payl}, nil
}

//create the evaluator to provide evaluation services for resources
func (rscc *Rscc) createPolicyEvaluator(env *common.Envelope, chanRes channelconfig.Resources) (policyEvaluator, error) {
	bundle, err := resourcesconfig.NewBundleFromEnvelope(env, chanRes)
	if err != nil {
		return nil, err
	}
	return &policyEvaluatorImpl{bundle}, nil
}

//NOTE - this is the core method that should be called to set the entire
//configuration of the RSCC. For now this is at Join time only but will
//have to export with proper interfacing for update processing via Invoke flow
func (rscc *Rscc) updateConfig(channel string, cfgb []byte, defProv aclmgmt.ACLProvider) error {
	var rsccProv rsccPolicyProvider

	//guranteed to set policy provider at least with the default provider
	defer func() {
		rscc.setPolicyProvider(channel, defProv, rsccProv)
	}()

	env, err := rscc.getEnvelopeFromConfig(channel, cfgb)
	if err != nil {
		return err
	}

	chanRes := peer.GetChannelConfig(channel)
	if chanRes == nil {
		return fmt.Errorf("Channel config not found for %s", channel)
	}

	//associate the evaluator with the managers
	pEvaluator, err := rscc.createPolicyEvaluator(env, chanRes)
	if err != nil {
		return fmt.Errorf("cannot create provider for channel %s(%s)", channel, err)
	}

	//this will get set by the deferred func
	rsccProv = &rsccPolicyProviderImpl{channel, pEvaluator}

	return nil
}

//-------- CC interface ------------

// Init RSCC - Init is just used to initialize policy assuming it is found in the
// channel ledger. Don't return error and get out. For example, we might still serve
// Invokes to set policy
func (rscc *Rscc) Init(stub shim.ChaincodeStubInterface) pb.Response {
	rsccLogger.Info("Init RSCC")

	b, err := stub.GetState(CHANNEL)
	if err != nil || len(b) == 0 {
		rsccLogger.Errorf("cannot find channel name")
		return shim.Success([]byte(NOCHANNEL))
	}

	channel := string(b)

	defProv := aclmgmt.NewDefaultACLProvider()

	b, err = stub.GetState(POLICY)
	if err != nil || len(b) == 0 {
		//we do not have policy, create just defaults
		rscc.setPolicyProvider(channel, defProv, nil)
		rsccLogger.Errorf("cannot find policy for channel %s", channel)
		return shim.Success([]byte(NOPOLICY))
	}

	//update the config
	if err = rscc.updateConfig(channel, b, defProv); err != nil {
		rsccLogger.Errorf("error on update config %s\n", err)
		return shim.Success([]byte(BADPOLICY))
	}

	return shim.Success(nil)
}

//Invoke - update policies
func (rscc *Rscc) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Error("--TBD---")
}
