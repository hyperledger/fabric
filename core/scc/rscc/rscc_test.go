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
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/common"
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
type mockDefaulACLProvider struct {
	//resource to policy
	resMap     map[string]string
	throwPanic string
}

//CheckACL rscc implements AClProvider's CheckACL interface so it can be registered
//as a provider with aclmgmt
func (mp *mockDefaulACLProvider) CheckACL(resName string, channelID string, idinfo interface{}) error {
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

//-------- misc funcs -----------
func setupGood(t *testing.T) (*Rscc, *shim.MockStub) {
	rscc := NewRscc()
	stub := shim.NewMockStub("rscc", rscc)
	stub.State[CHANNEL] = []byte("myc")
	b, _ := proto.Marshal(&common.ConfigGroup{Version: 1})
	stub.State[POLICY] = b

	res := stub.MockInit("1", [][]byte{})
	if res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	if len(res.Payload) != 0 {
		fmt.Println("Init failed (expected nil payload)", string(res.Payload))
		t.FailNow()
	}

	assert.NotNil(t, rscc.policyCache["myc"], "Expected non-nil cache")
	assert.NotNil(t, rscc.policyCache["myc"].defaultProvider, "Expected non-nil default provider")
	assert.NotNil(t, rscc.policyCache["myc"].rsccProvider, "Expected non-nil rscc provider")

	return rscc, stub
}

//-------- Tests -----------
func TestInit(t *testing.T) {
	setupGood(t)
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
	//start with a nice setup
	rscc, _ := setupGood(t)

	//setup two resources, "res" in rsccProvider and "defres" in defaultProvider for channel "myc"
	rscc.policyCache["myc"] = &policyProvider{&mockDefaulACLProvider{resMap: map[string]string{"defres": "defresPol"}, throwPanic: "defprovider"}, &mockRsccProvider{resMap: map[string]string{"res": "resPol"}, throwPanic: "rsccprovider"}}

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
	delete(rscc.policyCache["myc"].defaultProvider.(*mockDefaulACLProvider).resMap, "defres")
	err = rscc.CheckACL("defres", "myc", "id")
	assert.Error(t, err, "should have received error")
	assert.Equal(t, err.Error(), "[defaultprovider]no resource defres")

	//remove defaultProvider and test for anyres
	rscc.policyCache["myc"].defaultProvider = nil
	err = rscc.CheckACL("anyres", "myc", "id")
	assert.Error(t, err, "should have received error")
	assert.Equal(t, err.Error(), PolicyProviderNotFound("myc").Error())
}
