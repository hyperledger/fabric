/*
Copyright Hitachi, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	. "github.com/hyperledger/fabric/examples/chaincode/go/eventsender"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

const EventTimeout = time.Second * 10

func checkInit(t *testing.T, stub *shim.MockStub, args [][]byte) {
	res := stub.MockInit("1", args)
	assert.NotNil(t, res)
	assert.Equal(t, int32(shim.OK), res.Status)
}

func checkState(t *testing.T, stub *shim.MockStub, name string, value string) {
	bytes := stub.State[name]
	assert.NotNil(t, bytes)
	assert.Equal(t, value, string(bytes))
}

func checkQuery(t *testing.T, stub *shim.MockStub, value string) {
	res := stub.MockInvoke("1", [][]byte{[]byte("query")})
	expectedRes := "{\"NoEvents\":\"" + string(value) + "\"}"

	assert.NotNil(t, res)
	assert.Equal(t, int32(shim.OK), res.Status)
	assert.NotNil(t, res.Payload)
	assert.JSONEq(t, expectedRes, string(res.Payload))
}

func checkInvoke(t *testing.T, stub *shim.MockStub, args [][]byte) {
	res := stub.MockInvoke("1", args)
	assert.NotNil(t, res)
	assert.Equal(t, int32(shim.OK), res.Status)
}

func checkChaincodeEvent(t *testing.T, stub *shim.MockStub, expected *pb.ChaincodeEvent) {
	select {
	case actual := <-stub.ChaincodeEventsChannel:
		assert.Equal(t, expected, actual)
	case <-time.After(EventTimeout):
		fmt.Printf("Failed to chaincode event timeout.\n")
		t.FailNow()
	}
}

func TestEventSender_Init(t *testing.T) {
	cc := new(EventSender)
	stub := shim.NewMockStub("eventsender", cc)

	// Init []
	checkInit(t, stub, [][]byte{[]byte("init")})

	checkState(t, stub, "noevents", "0")
}

func TestEventSender_InvokeMissingFunc(t *testing.T) {
	cc := new(EventSender)
	stub := shim.NewMockStub("eventsender", cc)

	// Init []
	checkInit(t, stub, [][]byte{[]byte("init")})

	// Missing function
	res := stub.MockInvoke("1", [][]byte{[]byte("missingFunc")})
	assert.NotNil(t, res)
	assert.Equal(t, int32(shim.ERROR), res.Status)
	assert.Equal(t, "Invalid invoke function name. Expecting \"invoke\" \"query\"", string(res.Message))
}

func TestEventSender_Query(t *testing.T) {
	cc := new(EventSender)
	stub := shim.NewMockStub("eventsender", cc)

	// Init []
	checkInit(t, stub, [][]byte{[]byte("init")})

	// Query noevents
	checkQuery(t, stub, "0")
}

func TestEventSender_Invoke(t *testing.T) {
	cc := new(EventSender)
	stub := shim.NewMockStub("eventsender", cc)

	// Init []
	checkInit(t, stub, [][]byte{[]byte("init")})

	// Query noevents
	checkQuery(t, stub, "0")

	// Invoke (1st time, without args)
	checkInvoke(t, stub, [][]byte{[]byte("invoke")})

	// Check chaincode event
	expectedEvent := &pb.ChaincodeEvent{
		ChaincodeId: "",
		TxId:        "",
		EventName:   "evtsender",
		Payload:     []byte("Event 0"),
	}
	checkChaincodeEvent(t, stub, expectedEvent)

	// Query noevents
	checkQuery(t, stub, "1")

	// Invoke (2nd time, with args)
	checkInvoke(t, stub, [][]byte{[]byte("invoke"), []byte("a"), []byte("b")})

	// Check chaincode event
	expectedEvent = &pb.ChaincodeEvent{
		ChaincodeId: "",
		TxId:        "",
		EventName:   "evtsender",
		Payload:     []byte("Event 1,a,b"),
	}
	checkChaincodeEvent(t, stub, expectedEvent)

	// Query noevents
	checkQuery(t, stub, "2")
}
