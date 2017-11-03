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
package rscc

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// ------- mocks ---------

//mock rscc provider
type mockRsccProvider struct {
	//resource to policy
	resMap     map[string]string
	throwPanic string
}

//GetPolicyName returns the policy name given the resource string
func (mp *mockRsccProvider) GetPolicyName(resName string) string {
	if mp.resMap == nil {
		return ""
	}

	return mp.resMap[resName]
}

func (mp *mockRsccProvider) CheckACL(polName string, idinfo interface{}) error {
	//if it got here, it must have worked, all allowed..throw panic to signify
	//it was called from here
	if mp.throwPanic != "" {
		panic(mp.throwPanic)
	}
	return nil
}

//mock default provider
type mockDefaultACLProvider struct {
	//resource to policy
	resMap     map[string]string
	throwPanic string
}

//CheckACL rscc implements AClProvider's CheckACL interface so it can be registered
//as a provider with aclmgmt
func (mp *mockDefaultACLProvider) CheckACL(resName string, channelID string, idinfo interface{}) error {
	if mp.resMap[resName] == "" {
		return fmt.Errorf("[defaultprovider]no resource %s", resName)
	}

	//if it got here, it must have worked, all allowed..throw panic to signify
	//it was called from here
	if mp.throwPanic != "" {
		panic(mp.throwPanic)
	}

	return nil
}

func (mp *mockDefaultACLProvider) GenerateSimulationResults(txEnv *common.Envelope, sim ledger.TxSimulator) error {
	return nil
}

//barebones configuration for sticking into "rscc_seed_data"
func createConfig() []byte {
	b := utils.MarshalOrPanic(&common.Config{
		Type: int32(common.ConfigType_RESOURCE),
		ChannelGroup: &common.ConfigGroup{
			Groups: map[string]*common.ConfigGroup{
				"APIs": &common.ConfigGroup{
					// All of the default seed data values would inside this ConfigGroup
					Values: map[string]*common.ConfigValue{
						"res": &common.ConfigValue{
							Value: utils.MarshalOrPanic(&pb.APIResource{
								PolicyRef: "respol",
							}),
							ModPolicy: "resmodpol",
						},
					},
					Policies: map[string]*common.ConfigPolicy{
						"respol": &common.ConfigPolicy{
							Policy: &common.Policy{
								Type:  int32(common.Policy_SIGNATURE),
								Value: utils.MarshalOrPanic(cauthdsl.AcceptAllPolicy),
							},
							ModPolicy: "Example",
						},
					},
				},
			},
			ModPolicy: "adminpol",
		},
	})
	return b
}

//-------- misc funcs -----------
func setupGood(t *testing.T, path string) (*Rscc, *shim.MockStub) {
	viper.Set("peer.fileSystemPath", path)
	peer.MockInitialize()

	mspGetter := func(cid string) []string {
		return []string{"DEFAULT"}
	}

	peer.MockSetMSPIDGetter(mspGetter)

	if err := peer.MockCreateChain("myc"); err != nil {
		t.FailNow()
	}

	rscc := NewRscc()
	stub := shim.NewMockStub("rscc", rscc)
	stub.State[CHANNEL] = []byte("myc")
	b := createConfig()
	stub.State[POLICY] = b

	res := stub.MockInit("1", [][]byte{})
	if res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	if len(res.Payload) != 0 {
		fmt.Printf("Init failed expected nil payload, found %s", string(res.Payload))
		t.FailNow()
	}

	assert.NotNil(t, rscc.policyCache["myc"], "Expected non-nil cache")
	assert.NotNil(t, rscc.policyCache["myc"].defaultProvider, "Expected non-nil default provider")
	assert.NotNil(t, rscc.policyCache["myc"].rsccProvider, "Expected non-nil rscc provider")

	return rscc, stub
}

//-------- Tests -----------
func TestInit(t *testing.T) {
	path := "/var/hyperledger/rscctest/"
	setupGood(t, path)
	defer os.RemoveAll(path)
}

func TestBadState(t *testing.T) {
	rscc := NewRscc()
	stub := shim.NewMockStub("rscc", rscc)

	res := stub.MockInit("1", [][]byte{})
	if string(res.Payload) != NOCHANNEL {
		fmt.Printf("Init failed (expected %s found %s)", NOCHANNEL, res.Payload)
		t.FailNow()
	}

	stub.State[CHANNEL] = []byte("myc")
	res = stub.MockInit("1", [][]byte{})
	if string(res.Payload) != NOPOLICY {
		fmt.Printf("Init failed (expected %s found %s)", NOPOLICY, res.Payload)
		t.FailNow()
	}

	b := []byte("badbadbadpolicy")
	stub.State[POLICY] = b

	res = stub.MockInit("1", [][]byte{})
	if string(res.Payload) != BADPOLICY {
		fmt.Printf("Init failed (expected %s found %s)", BADPOLICY, res.Payload)
		t.FailNow()
	}
}

func testProviderSource(t *testing.T, rscc *Rscc, res, channel, id, panicMessage string) {
	defer func() {
		r := recover()
		assert.NotNil(t, r, "should have panicked")
		//assert.Equal(t, fmt.Sprintf("%s", r), panicMessage)
		assert.Equal(t, r, panicMessage)
	}()
	rscc.CheckACL(res, channel, id)
}

//tests if the right ACL providers get called
func TestACLPaths(t *testing.T) {
	path := "/var/hyperledger/rscctest/"
	//start with a nice setup
	rscc, _ := setupGood(t, path)
	defer os.RemoveAll(path)

	//setup two resources, "res" in rsccProvider and "defres" in defaultProvider for channel "myc"
	rscc.policyCache["myc"] = &policyProvider{&mockDefaultACLProvider{resMap: map[string]string{"defres": "defresPol"}, throwPanic: "defprovider"}, &mockRsccProvider{resMap: map[string]string{"res": "resPol"}, throwPanic: "rsccprovider"}}

	//bad channel
	err := rscc.CheckACL("res", "badchannel", "id")
	assert.Error(t, err, "should have received error")
	assert.Equal(t, err.Error(), NoPolicyProviderInCache("badchannel").Error())

	//good channel no resource
	err = rscc.CheckACL("nores", "myc", "id")
	assert.Error(t, err, "should have received error")
	assert.Equal(t, err.Error(), "[defaultprovider]no resource nores")

	//resource from rscc provider
	testProviderSource(t, rscc, "res", "myc", "id", "rsccprovider")

	//resource from default provider
	testProviderSource(t, rscc, "defres", "myc", "id", "defprovider")

	//remove rsccProvider and test for res - should go to default provider and receive no resource
	rscc.policyCache["myc"].rsccProvider = nil
	err = rscc.CheckACL("res", "myc", "id")
	assert.Error(t, err, "should have received error")
	assert.Equal(t, err.Error(), "[defaultprovider]no resource res")

	//remove "defres" from default provider - should give defres (was there before)
	delete(rscc.policyCache["myc"].defaultProvider.(*mockDefaultACLProvider).resMap, "defres")
	err = rscc.CheckACL("defres", "myc", "id")
	assert.Error(t, err, "should have received error")
	assert.Equal(t, err.Error(), "[defaultprovider]no resource defres")

	//remove defaultProvider and test for anyres
	rscc.policyCache["myc"].defaultProvider = nil
	err = rscc.CheckACL("anyres", "myc", "id")
	assert.Error(t, err, "should have received error")
	assert.Equal(t, err.Error(), PolicyProviderNotFound("myc").Error())
}

func TestLedgerProcessor(t *testing.T) {
	path := "/var/hyperledger/rscctest/"
	viper.Set("peer.fileSystemPath", path)
	peer.MockInitialize()
	defer os.RemoveAll(path)

	if err := peer.MockCreateChain("myc"); err != nil {
		t.FailNow()
	}

	txid1 := util.GenerateUUID()
	ledger := peer.GetLedger("myc")
	simulator, _ := ledger.NewTxSimulator(txid1)

	rscc := NewRscc()

	//don't accept nil params-1
	err := rscc.GenerateSimulationResults(nil, simulator)
	assert.NotNil(t, err)

	//don't accept nil params-2
	err = rscc.GenerateSimulationResults(&common.Envelope{}, nil)
	assert.NotNil(t, err)

	//bad payload
	txEnv := &common.Envelope{Payload: []byte("bad payload")}
	err = rscc.GenerateSimulationResults(txEnv, simulator)
	assert.NotNil(t, err)

	//bad ConfigEnvelope
	txEnv.Payload = utils.MarshalOrPanic(&common.Payload{Data: []byte("bad ConfigEnvelope")})
	err = rscc.GenerateSimulationResults(txEnv, simulator)
	assert.NotNil(t, err)

	//nil LastUpdate
	txEnv.Payload = utils.MarshalOrPanic(&common.Payload{Data: utils.MarshalOrPanic(&common.ConfigEnvelope{Config: &common.Config{Sequence: 1}, LastUpdate: nil})})
	err = rscc.GenerateSimulationResults(txEnv, simulator)
	assert.NotNil(t, err)

	//bad ConfigUpdateEnvelope
	txEnv.Payload = utils.MarshalOrPanic(&common.Payload{Data: utils.MarshalOrPanic(&common.ConfigEnvelope{Config: &common.Config{Sequence: 1}, LastUpdate: &common.Envelope{Payload: utils.MarshalOrPanic(&common.Payload{Data: []byte("bad ConfigUpdateEnvelope")})}})})
	err = rscc.GenerateSimulationResults(txEnv, simulator)
	assert.NotNil(t, err)

	//bad ConfigUpdate
	txEnv.Payload = utils.MarshalOrPanic(&common.Payload{Data: utils.MarshalOrPanic(&common.ConfigEnvelope{Config: &common.Config{Sequence: 1}, LastUpdate: &common.Envelope{Payload: utils.MarshalOrPanic(&common.Payload{Data: utils.MarshalOrPanic(&common.ConfigUpdateEnvelope{ConfigUpdate: []byte("bad ConfigUpdate")})})}})})
	err = rscc.GenerateSimulationResults(txEnv, simulator)
	assert.NotNil(t, err)

	//ignore config updates
	txEnv.Payload = utils.MarshalOrPanic(&common.Payload{Data: utils.MarshalOrPanic(&common.ConfigEnvelope{Config: &common.Config{Sequence: 2}, LastUpdate: &common.Envelope{Payload: utils.MarshalOrPanic(&common.Payload{Data: utils.MarshalOrPanic(&common.ConfigUpdateEnvelope{ConfigUpdate: utils.MarshalOrPanic(&common.ConfigUpdate{ChannelId: "myc", IsolatedData: map[string][]byte{"rscc_seed_data": createConfig()}})})})}})})
	err = rscc.GenerateSimulationResults(txEnv, simulator)
	res, err := simulator.GetTxSimulationResults()
	//should not error ...
	assert.Nil(t, err)
	//... but sim results should be nil
	assert.Nil(t, res.PubSimulationResults.NsRwset)

	//good processing
	txid1 = util.GenerateUUID()
	simulator, _ = ledger.NewTxSimulator(txid1)
	txEnv.Payload = utils.MarshalOrPanic(&common.Payload{Data: utils.MarshalOrPanic(&common.ConfigEnvelope{Config: &common.Config{Sequence: 1}, LastUpdate: &common.Envelope{Payload: utils.MarshalOrPanic(&common.Payload{Data: utils.MarshalOrPanic(&common.ConfigUpdateEnvelope{ConfigUpdate: utils.MarshalOrPanic(&common.ConfigUpdate{ChannelId: "myc", IsolatedData: map[string][]byte{"rscc_seed_data": createConfig()}})})})}})})
	err = rscc.GenerateSimulationResults(txEnv, simulator)
	res, err = simulator.GetTxSimulationResults()
	//should not error ...
	assert.Nil(t, err)
	//... and sim results should be non-nil
	assert.NotNil(t, res.PubSimulationResults.NsRwset)
}
