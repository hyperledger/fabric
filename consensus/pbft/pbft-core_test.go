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

package pbft

import (
	"encoding/base64"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/consensus/util/events"
	pb "github.com/hyperledger/fabric/protos"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestConfigSet(t *testing.T) {
	config := loadConfig()

	testKeys := []string{
		"general.mode",
		"general.N",
		"general.f",
		"general.K",
		"general.logmultiplier",
		"general.batchsize",
		"general.byzantine",
		"general.viewchangeperiod",
		"general.timeout.batch",
		"general.timeout.request",
		"general.timeout.viewchange",
		"general.timeout.resendviewchange",
		"general.timeout.nullrequest",
		"general.timeout.broadcast",
		"executor.queuesize",
	}

	for _, key := range testKeys {
		if ok := config.IsSet(key); !ok {
			t.Errorf("Cannot test env override because \"%s\" does not seem to be set", key)
		}
	}
}

func TestEnvOverride(t *testing.T) {
	config := loadConfig()

	key := "general.mode"               // for a key that exists
	envName := "CORE_PBFT_GENERAL_MODE" // env override name
	overrideValue := "overide_test"     // value to override default value with

	os.Setenv(envName, overrideValue)
	// The override config value will cause other calls to fail unless unset.
	defer func() {
		os.Unsetenv(envName)
	}()

	if ok := config.IsSet("general.mode"); !ok {
		t.Fatalf("Env override in place, and key \"%s\" is not set", key)
	}

	// read key
	configVal := config.GetString("general.mode")
	if configVal != overrideValue {
		t.Fatalf("Env override in place, expected key \"%s\" to be \"%s\" but instead got \"%s\"", key, overrideValue, configVal)
	}

}

func TestIntEnvOverride(t *testing.T) {
	config := loadConfig()

	tests := []struct {
		key           string
		envName       string
		overrideValue string
		expectValue   int
	}{
		{"general.N", "CORE_PBFT_GENERAL_N", "8", 8},
		{"general.f", "CORE_PBFT_GENERAL_F", "2", 2},
		{"general.K", "CORE_PBFT_GENERAL_K", "20", 20},
		{"general.logmultiplier", "CORE_PBFT_GENERAL_LOGMULTIPLIER", "6", 6},
		{"general.batchsize", "CORE_PBFT_GENERAL_BATCHSIZE", "200", 200},
		{"general.viewchangeperiod", "CORE_PBFT_GENERAL_VIEWCHANGEPERIOD", "5", 5},
		{"executor.queuesize", "CORE_PBFT_EXECUTOR_QUEUESIZE", "50", 50},
	}

	for _, test := range tests {
		os.Setenv(test.envName, test.overrideValue)

		if ok := config.IsSet(test.key); !ok {
			t.Errorf("Env override in place, and key \"%s\" is not set", test.key)
		}

		configVal := config.GetInt(test.key)
		if configVal != test.expectValue {
			t.Errorf("Env override in place, expected key \"%s\" to be \"%v\" but instead got \"%d\"", test.key, test.expectValue, configVal)
		}

		os.Unsetenv(test.envName)
	}
}

func TestDurationEnvOverride(t *testing.T) {
	config := loadConfig()

	tests := []struct {
		key           string
		envName       string
		overrideValue string
		expectValue   time.Duration
	}{
		{"general.timeout.batch", "CORE_PBFT_GENERAL_TIMEOUT_BATCH", "2s", 2 * time.Second},
		{"general.timeout.request", "CORE_PBFT_GENERAL_TIMEOUT_REQUEST", "4s", 4 * time.Second},
		{"general.timeout.viewchange", "CORE_PBFT_GENERAL_TIMEOUT_VIEWCHANGE", "5s", 5 * time.Second},
		{"general.timeout.resendviewchange", "CORE_PBFT_GENERAL_TIMEOUT_RESENDVIEWCHANGE", "200ms", 200 * time.Millisecond},
		{"general.timeout.nullrequest", "CORE_PBFT_GENERAL_TIMEOUT_NULLREQUEST", "1s", time.Second},
		{"general.timeout.broadcast", "CORE_PBFT_GENERAL_TIMEOUT_BROADCAST", "1m", time.Minute},
	}

	for _, test := range tests {
		os.Setenv(test.envName, test.overrideValue)

		if ok := config.IsSet(test.key); !ok {
			t.Errorf("Env override in place, and key \"%s\" is not set", test.key)
		}

		configVal := config.GetDuration(test.key)
		if configVal != test.expectValue {
			t.Errorf("Env override in place, expected key \"%s\" to be \"%v\" but instead got \"%v\"", test.key, test.expectValue, configVal)
		}

		os.Unsetenv(test.envName)
	}
}

func TestBoolEnvOverride(t *testing.T) {
	config := loadConfig()

	tests := []struct {
		key           string
		envName       string
		overrideValue string
		expectValue   bool
	}{
		{"general.byzantine", "CORE_PBFT_GENERAL_BYZANTINE", "false", false},
		{"general.byzantine", "CORE_PBFT_GENERAL_BYZANTINE", "0", false},
		{"general.byzantine", "CORE_PBFT_GENERAL_BYZANTINE", "true", true},
		{"general.byzantine", "CORE_PBFT_GENERAL_BYZANTINE", "1", true},
	}

	for i, test := range tests {
		os.Setenv(test.envName, test.overrideValue)

		if ok := config.IsSet(test.key); !ok {
			t.Errorf("Env override in place, and key \"%s\" is not set", test.key)
		}

		configVal := config.GetBool(test.key)
		if configVal != test.expectValue {
			t.Errorf("Test %d Env override in place, expected key \"%s\" to be \"%v\" but instead got \"%v\"", i, test.key, test.expectValue, configVal)
		}

		os.Unsetenv(test.envName)
	}
}

func TestMaliciousPrePrepare(t *testing.T) {
	mock := &omniProto{
		broadcastImpl: func(msgPayload []byte) {
			t.Fatalf("Expected to ignore malicious pre-prepare")
		},
	}
	instance := newPbftCore(1, loadConfig(), mock, &inertTimerFactory{})
	defer instance.close()
	instance.replicaCount = 5

	pbftMsg := &Message_PrePrepare{&PrePrepare{
		View:           0,
		SequenceNumber: 1,
		BatchDigest:    hash(createPbftReqBatch(1, 1)),
		RequestBatch:   createPbftReqBatch(1, 2),
		ReplicaId:      0,
	}}
	events.SendEvent(instance, pbftMsg)
}

func TestWrongReplicaID(t *testing.T) {
	mock := &omniProto{}
	instance := newPbftCore(0, loadConfig(), mock, &inertTimerFactory{})
	defer instance.close()

	reqBatch := createPbftReqBatch(1, 1)
	pbftMsg := &Message{Payload: &Message_PrePrepare{PrePrepare: &PrePrepare{
		View:           0,
		SequenceNumber: 1,
		BatchDigest:    hash(reqBatch),
		RequestBatch:   reqBatch,
		ReplicaId:      1,
	}}}
	next, err := instance.recvMsg(pbftMsg, 2)

	if next != nil || err == nil {
		t.Fatalf("Shouldn't have processed message with incorrect replica ID")
	}
	if err != nil {
		rightError := strings.HasPrefix(err.Error(), "Sender ID")
		if !rightError {
			t.Fatalf("Should have returned error about incorrect replica ID on the incoming message")
		}
	}
}

func TestIncompletePayload(t *testing.T) {
	mock := &omniProto{}
	instance := newPbftCore(1, loadConfig(), mock, &inertTimerFactory{})
	defer instance.close()
	instance.replicaCount = 5

	broadcaster := uint64(generateBroadcaster(instance.replicaCount))

	checkMsg := func(msg *Message, errMsg string, args ...interface{}) {
		mock.broadcastImpl = func(msgPayload []byte) {
			t.Errorf(errMsg, args...)
		}
		events.SendEvent(instance, pbftMessageEvent{msg: msg, sender: broadcaster})
	}

	checkMsg(&Message{}, "Expected to reject empty message")
	checkMsg(&Message{Payload: &Message_PrePrepare{PrePrepare: &PrePrepare{ReplicaId: broadcaster}}}, "Expected to reject empty pre-prepare")
}

func TestNetwork(t *testing.T) {
	validatorCount := 7
	net := makePBFTNetwork(validatorCount, nil)

	reqBatch := createPbftReqBatch(1, uint64(generateBroadcaster(validatorCount)))
	net.pbftEndpoints[0].manager.Queue() <- reqBatch

	err := net.process()
	if err != nil {
		t.Fatalf("Processing failed: %s", err)
	}

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions <= 0 {
			t.Errorf("Instance %d did not execute transaction", pep.id)
			continue
		}
		if pep.sc.executions != 1 {
			t.Errorf("Instance %d executed more than one transaction", pep.id)
			continue
		}
		if !reflect.DeepEqual(pep.sc.lastExecution, hash(reqBatch.GetBatch()[0])) {
			t.Errorf("Instance %d executed wrong transaction, %x should be %x",
				pep.id, pep.sc.lastExecution, hash(reqBatch.GetBatch()[0]))
		}
	}
}

type checkpointConsumer struct {
	simpleConsumer
	execWait *sync.WaitGroup
}

func (cc *checkpointConsumer) execute(seqNo uint64, tx []byte) {
}

func TestCheckpoint(t *testing.T) {
	execWait := &sync.WaitGroup{}
	finishWait := &sync.WaitGroup{}

	validatorCount := 4
	config := loadConfig()
	config.Set("general.K", 2)
	config.Set("general.logmultiplier", 2)
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	execReqBatch := func(tag int64) {
		net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(tag, uint64(generateBroadcaster(validatorCount)))
		net.process()
	}

	// execWait is 0, and execute will proceed
	execReqBatch(1)
	execReqBatch(2)
	finishWait.Wait()
	net.process()

	for _, pep := range net.pbftEndpoints {
		if len(pep.pbft.chkpts) != 1 {
			t.Errorf("Expected 1 checkpoint, found %d", len(pep.pbft.chkpts))
			continue
		}

		if _, ok := pep.pbft.chkpts[2]; !ok {
			t.Errorf("Expected checkpoint for seqNo 2")
			continue
		}

		if pep.pbft.h != 2 {
			t.Errorf("Expected low water mark to be 2, got %d", pep.pbft.h)
			continue
		}
	}

	// this will block executes for now
	execWait.Add(1)
	execReqBatch(3)
	execReqBatch(4)
	execReqBatch(5)
	execReqBatch(6)

	// by now the requests should have committed, but not yet executed
	// we also reached the high water mark by now.

	execReqBatch(7)

	// request 7 should not have committed, because no more free seqNo
	// could be assigned.

	// unblock executes.
	execWait.Add(-1)

	net.process()
	finishWait.Wait() // Decoupling the execution thread makes this nastiness necessary
	net.process()

	// by now request 7 should have been confirmed and executed

	for _, pep := range net.pbftEndpoints {
		expectedExecutions := uint64(7)
		if pep.sc.executions != expectedExecutions {
			t.Errorf("Should have executed %d, got %d instead for replica %d", expectedExecutions, pep.sc.executions, pep.id)
		}
	}
}

func TestLostPrePrepare(t *testing.T) {
	validatorCount := 4
	net := makePBFTNetwork(validatorCount, nil)
	defer net.stop()

	net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(1, uint64(generateBroadcaster(validatorCount)))

	// clear all messages sent by primary
	msg := <-net.msgs
	prePrep := &Message{}
	err := proto.Unmarshal(msg.msg, prePrep)
	if err != nil {
		t.Fatalf("Error unmarshaling message")
	}
	net.clearMessages()

	// deliver pre-prepare to subset of replicas
	for _, pep := range net.pbftEndpoints[1 : len(net.pbftEndpoints)-1] {
		pep.manager.Queue() <- prePrep.GetPrePrepare()
	}

	err = net.process()
	if err != nil {
		t.Fatalf("Processing failed: %s", err)
	}

	for _, pep := range net.pbftEndpoints {
		if pep.id != 3 && pep.sc.executions != 1 {
			t.Errorf("Expected execution on replica %d", pep.id)
			continue
		}
		if pep.id == 3 && pep.sc.executions > 0 {
			t.Errorf("Expected no execution")
			continue
		}
	}
}

func TestInconsistentPrePrepare(t *testing.T) {
	validatorCount := 4
	net := makePBFTNetwork(validatorCount, nil)
	defer net.stop()

	makePP := func(tag int64) *PrePrepare {
		reqBatch := createPbftReqBatch(tag, uint64(generateBroadcaster(validatorCount)))
		preprep := &PrePrepare{
			View:           0,
			SequenceNumber: 1,
			BatchDigest:    hash(reqBatch),
			RequestBatch:   reqBatch,
			ReplicaId:      0,
		}
		return preprep
	}

	net.pbftEndpoints[0].manager.Queue() <- makePP(1).GetRequestBatch()

	// clear all messages sent by primary
	net.clearMessages()

	// replace with fake messages
	net.pbftEndpoints[1].manager.Queue() <- makePP(1)
	net.pbftEndpoints[2].manager.Queue() <- makePP(2)
	net.pbftEndpoints[3].manager.Queue() <- makePP(3)

	net.process()

	for n, pep := range net.pbftEndpoints {
		if pep.sc.executions < 1 || pep.sc.executions > 3 {
			t.Errorf("Replica %d expected [1,3] executions, got %d", n, pep.sc.executions)
			continue
		}
	}
}

// This test is designed to detect a conflation of S and S' from the paper in the view change
func TestViewChangeWatermarksMovement(t *testing.T) {
	instance := newPbftCore(0, loadConfig(), &omniProto{
		viewChangeImpl: func(v uint64) {},
		skipToImpl: func(s uint64, id []byte, replicas []uint64) {
			t.Fatalf("Should not have attempted to initiate state transfer")
		},
		broadcastImpl: func(b []byte) {},
	}, &inertTimerFactory{})
	instance.activeView = false
	instance.view = 1
	instance.lastExec = 10

	vset := make([]*ViewChange, 3)

	// Replica 0 sent checkpoints for 10
	vset[0] = &ViewChange{
		H: 5,
		Cset: []*ViewChange_C{
			{
				SequenceNumber: 10,
				Id:             "ten",
			},
		},
	}

	// Replica 1 sent checkpoints for 10
	vset[1] = &ViewChange{
		H: 5,
		Cset: []*ViewChange_C{
			{
				SequenceNumber: 10,
				Id:             "ten",
			},
		},
	}

	// Replica 2 sent checkpoints for 10
	vset[2] = &ViewChange{
		H: 5,
		Cset: []*ViewChange_C{
			{
				SequenceNumber: 10,
				Id:             "ten",
			},
		},
	}

	xset := make(map[uint64]string)
	xset[11] = ""

	instance.newViewStore[1] = &NewView{
		View:      1,
		Vset:      vset,
		Xset:      xset,
		ReplicaId: 1,
	}

	if _, ok := instance.processNewView().(viewChangedEvent); !ok {
		t.Fatalf("Failed to successfully process new view")
	}

	expected := uint64(10)
	if instance.h != expected {
		t.Fatalf("Expected to move high watermark to %d, but picked %d", expected, instance.h)
	}
}

// This test is designed to detect a conflation of S and S' from the paper in the view change
func TestViewChangeCheckpointSelection(t *testing.T) {
	instance := &pbftCore{
		f:  1,
		N:  4,
		id: 0,
	}

	vset := make([]*ViewChange, 3)

	// Replica 0 sent checkpoints for 5
	vset[0] = &ViewChange{
		H: 5,
		Cset: []*ViewChange_C{
			{
				SequenceNumber: 10,
				Id:             "ten",
			},
		},
	}

	// Replica 1 sent checkpoints for 5
	vset[1] = &ViewChange{
		H: 5,
		Cset: []*ViewChange_C{
			{
				SequenceNumber: 10,
				Id:             "ten",
			},
		},
	}

	// Replica 2 sent checkpoints for 15
	vset[2] = &ViewChange{
		H: 10,
		Cset: []*ViewChange_C{
			{
				SequenceNumber: 15,
				Id:             "fifteen",
			},
		},
	}

	checkpoint, ok, _ := instance.selectInitialCheckpoint(vset)

	if !ok {
		t.Fatalf("Failed to pick correct a checkpoint for view change")
	}

	expected := uint64(10)
	if checkpoint.SequenceNumber != expected {
		t.Fatalf("Expected to pick checkpoint %d, but picked %d", expected, checkpoint.SequenceNumber)
	}
}

func TestViewChange(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.K", 2)
	config.Set("general.logmultiplier", 2)
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	execReqBatch := func(tag int64) {
		net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(tag, uint64(generateBroadcaster(validatorCount)))
		net.process()
	}

	execReqBatch(1)
	execReqBatch(2)
	execReqBatch(3)

	for i := 2; i < len(net.pbftEndpoints); i++ {
		net.pbftEndpoints[i].pbft.sendViewChange()
	}

	err := net.process()
	if err != nil {
		t.Fatalf("Processing failed: %s", err)
	}

	if net.pbftEndpoints[1].pbft.view != 1 || net.pbftEndpoints[0].pbft.view != 1 {
		t.Fatalf("Replicas did not follow f+1 crowd to trigger view-change")
	}

	cp, ok, _ := net.pbftEndpoints[1].pbft.selectInitialCheckpoint(net.pbftEndpoints[1].pbft.getViewChanges())
	if !ok || cp.SequenceNumber != 2 {
		t.Fatalf("Wrong new initial checkpoint: %+v",
			net.pbftEndpoints[1].pbft.viewChangeStore)
	}

	msgList := net.pbftEndpoints[1].pbft.assignSequenceNumbers(net.pbftEndpoints[1].pbft.getViewChanges(), cp.SequenceNumber)
	if msgList[4] != "" || msgList[5] != "" || msgList[3] == "" {
		t.Fatalf("Wrong message list: %+v", msgList)
	}
}

func TestInconsistentDataViewChange(t *testing.T) {
	validatorCount := 4
	net := makePBFTNetwork(validatorCount, nil)
	defer net.stop()

	makePP := func(tag int64) *PrePrepare {
		reqBatch := createPbftReqBatch(tag, uint64(generateBroadcaster(validatorCount)))
		preprep := &PrePrepare{
			View:           0,
			SequenceNumber: 1,
			BatchDigest:    hash(reqBatch),
			RequestBatch:   reqBatch,
			ReplicaId:      0,
		}
		return preprep
	}

	net.pbftEndpoints[0].manager.Queue() <- makePP(0).GetRequestBatch()

	// clear all messages sent by primary
	net.clearMessages()

	// replace with fake messages
	net.pbftEndpoints[1].manager.Queue() <- makePP(1)
	net.pbftEndpoints[2].manager.Queue() <- makePP(1)
	net.pbftEndpoints[3].manager.Queue() <- makePP(0)

	err := net.process()
	if err != nil {
		t.Fatalf("Processing failed: %s", err)
	}

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions < 1 {
			t.Errorf("Expected execution")
			continue
		}
	}
}

func TestViewChangeWithStateTransfer(t *testing.T) {
	validatorCount := 4
	net := makePBFTNetwork(validatorCount, nil)
	defer net.stop()

	var err error

	for _, pep := range net.pbftEndpoints {
		pep.pbft.K = 2
		pep.pbft.L = 6
		pep.pbft.requestTimeout = 500 * time.Millisecond
	}

	broadcaster := uint64(generateBroadcaster(validatorCount))

	makePP := func(tag int64) *PrePrepare {
		reqBatch := createPbftReqBatch(tag, broadcaster)
		preprep := &PrePrepare{
			View:           0,
			SequenceNumber: uint64(tag),
			BatchDigest:    hash(reqBatch),
			RequestBatch:   reqBatch,
			ReplicaId:      0,
		}
		return preprep
	}

	// Have primary advance the sequence number past a checkpoint for replicas 0,1,2
	for i := int64(1); i <= 3; i++ {
		net.pbftEndpoints[0].manager.Queue() <- makePP(i).GetRequestBatch()

		// clear all messages sent by primary
		net.clearMessages()

		net.pbftEndpoints[0].manager.Queue() <- makePP(i)
		net.pbftEndpoints[1].manager.Queue() <- makePP(i)
		net.pbftEndpoints[2].manager.Queue() <- makePP(i)

		err = net.process()
		if err != nil {
			t.Fatalf("Processing failed: %s", err)
		}

	}

	fmt.Println("Done with stage 1")

	// Add to replica 3's complaint, cause a view change
	net.pbftEndpoints[1].pbft.sendViewChange()
	net.pbftEndpoints[2].pbft.sendViewChange()
	err = net.process()
	if err != nil {
		t.Fatalf("Processing failed: %s", err)
	}

	fmt.Println("Done with stage 3")

	net.pbftEndpoints[1].manager.Queue() <- makePP(5).GetRequestBatch()
	err = net.process()
	if err != nil {
		t.Fatalf("Processing failed: %s", err)
	}

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions != 4 {
			t.Errorf("Replica %d expected execution through seqNo 5 with one null exec, got %d executions", pep.pbft.id, pep.sc.executions)
			continue
		}
	}
	fmt.Println("Done with stage 3")
}

func TestNewViewTimeout(t *testing.T) {
	millisUntilTimeout := time.Duration(800)
	validatorCount := 4
	config := loadConfig()
	config.Set("general.timeout.request", "400ms")
	config.Set("general.timeout.viewchange", "800ms")
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	replica1Disabled := false
	net.filterFn = func(src int, dst int, msg []byte) []byte {
		if dst == -1 && src == 1 && replica1Disabled {
			return nil
		}
		return msg
	}

	go net.processContinually()

	reqBatch := createPbftReqBatch(1, uint64(generateBroadcaster(validatorCount)))

	// This will eventually trigger 1's request timeout
	// We check that one single timed out replica will not keep trying to change views by itself
	net.pbftEndpoints[1].manager.Queue() <- reqBatch
	fmt.Println("Debug: Sleeping 1")
	time.Sleep(5 * millisUntilTimeout * time.Millisecond)
	fmt.Println("Debug: Waking 1")

	// This will eventually trigger 3's request timeout, which will lead to a view change to 1.
	// However, we disable 1, which will disable the new-view going through.
	// This checks that replicas will automatically move to view 2 when the view change times out.
	// However, 2 does not know about the missing request, and therefore the request will not be
	// pre-prepared and finally executed.
	replica1Disabled = true
	net.pbftEndpoints[3].manager.Queue() <- reqBatch
	fmt.Println("Debug: Sleeping 2")
	time.Sleep(5 * millisUntilTimeout * time.Millisecond)
	fmt.Println("Debug: Waking 2")

	// So far, we are in view 2, and replica 1 and 3 (who got the request) in view change to view 3.
	// Submitting the request to 0 will eventually trigger its view-change timeout, which will make
	// all replicas move to view 3 and finally process the request.
	net.pbftEndpoints[0].manager.Queue() <- reqBatch
	fmt.Println("Debug: Sleeping 3")
	time.Sleep(5 * millisUntilTimeout * time.Millisecond)
	fmt.Println("Debug: Waking 3")

	for i, pep := range net.pbftEndpoints {
		if pep.pbft.view < 3 {
			t.Errorf("Should have reached view 3, got %d instead for replica %d", pep.pbft.view, i)
		}
		executionsExpected := uint64(1)
		if pep.sc.executions != executionsExpected {
			t.Errorf("Should have executed %d, got %d instead for replica %d", executionsExpected, pep.sc.executions, i)
		}
	}
}

func TestViewChangeUpdateSeqNo(t *testing.T) {
	millisUntilTimeout := 400 * time.Millisecond
	validatorCount := 4
	config.Set("general.timeout.request", "400ms")
	config.Set("general.timeout.viewchange", "400ms")
	net := makePBFTNetwork(validatorCount, config)
	for _, pe := range net.pbftEndpoints {
		pe.pbft.lastExec = 99
		pe.pbft.h = 99 / pe.pbft.K * pe.pbft.K
	}
	net.pbftEndpoints[0].pbft.seqNo = 99

	go net.processContinually()

	broadcaster := uint64(generateBroadcaster(validatorCount))

	reqBatch := createPbftReqBatch(1, broadcaster)
	net.pbftEndpoints[0].manager.Queue() <- reqBatch
	time.Sleep(5 * millisUntilTimeout)
	// Now we all have executed seqNo 100.  After triggering a
	// view change, the new primary should pick up right after
	// that.

	net.pbftEndpoints[0].pbft.sendViewChange()
	net.pbftEndpoints[1].pbft.sendViewChange()
	time.Sleep(5 * millisUntilTimeout)

	reqBatch = createPbftReqBatch(2, broadcaster)
	net.pbftEndpoints[1].manager.Queue() <- reqBatch
	time.Sleep(5 * millisUntilTimeout)

	net.stop()
	for i, pep := range net.pbftEndpoints {
		if pep.pbft.view < 1 {
			t.Errorf("Should have reached view 3, got %d instead for replica %d", pep.pbft.view, i)
		}
		executionsExpected := uint64(2)
		if pep.sc.executions != executionsExpected {
			t.Errorf("Should have executed %d, got %d instead for replica %d", executionsExpected, pep.sc.executions, i)
		}
	}
}

// Test for issue #1119
func TestSendQueueThrottling(t *testing.T) {
	prePreparesSent := 0

	mock := &omniProto{}
	instance := newPbftCore(0, loadConfig(), mock, &inertTimerFactory{})
	instance.f = 1
	instance.K = 2
	instance.L = 4
	instance.consumer = &omniProto{
		broadcastImpl: func(p []byte) {
			prePreparesSent++
		},
	}
	defer instance.close()

	for j := 0; j < 4; j++ {
		events.SendEvent(instance, createPbftReqBatch(int64(j), 0)) // replica ID for req doesn't matter
	}

	expected := 2
	if prePreparesSent != expected {
		t.Fatalf("Expected to send only %d pre-prepares, but got %d messages", expected, prePreparesSent)
	}
}

// From issue #687
func TestWitnessCheckpointOutOfBounds(t *testing.T) {
	mock := &omniProto{}
	instance := newPbftCore(1, loadConfig(), mock, &inertTimerFactory{})
	instance.f = 1
	instance.K = 2
	instance.L = 4
	defer instance.close()

	events.SendEvent(instance, &Checkpoint{
		SequenceNumber: 6,
		ReplicaId:      0,
	})

	instance.moveWatermarks(6)

	// This causes the list of high checkpoints to grow to be f+1
	// even though there are not f+1 checkpoints witnessed outside our range
	// historically, this caused an index out of bounds error
	events.SendEvent(instance, &Checkpoint{
		SequenceNumber: 10,
		ReplicaId:      3,
	})
}

// From issue #687
func TestWitnessFallBehindMissingPrePrepare(t *testing.T) {
	mock := &omniProto{}
	instance := newPbftCore(1, loadConfig(), mock, &inertTimerFactory{})
	instance.f = 1
	instance.K = 2
	instance.L = 4
	defer instance.close()

	events.SendEvent(instance, &Commit{
		SequenceNumber: 2,
		ReplicaId:      0,
	})

	// Historically, the lack of prePrepare associated with the commit would cause
	// a nil pointer reference
	instance.moveWatermarks(6)
}

func TestFallBehind(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.K", 2)
	config.Set("general.logmultiplier", 2)
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	execReqBatch := func(tag int64, skipThree bool) {
		net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(tag, uint64(generateBroadcaster(validatorCount)))

		if skipThree {
			// Send the request for consensus to everone but replica 3
			net.filterFn = func(src, replica int, msg []byte) []byte {
				if src != -1 && replica == 3 {
					return nil
				}
				return msg
			}
		} else {
			// Send the request for consensus to everone
			net.filterFn = nil
		}
		err := net.process()
		if err != nil {
			t.Fatalf("Processing failed: %s", err)
		}
	}

	pep := net.pbftEndpoints[3]
	pbft := pep.pbft

	// Send enough requests to get to a checkpoint quorum certificate with sequence number L+K
	execReqBatch(1, true)
	for i := int64(2); uint64(i) <= pbft.L+pbft.K; i++ {
		execReqBatch(i, false)
	}

	if !pbft.skipInProgress {
		t.Fatalf("Replica did not detect that it has fallen behind.")
	}

	if len(pbft.chkpts) != 0 {
		t.Fatalf("Expected no checkpoints, found %d", len(pbft.chkpts))
	}

	if pbft.h != pbft.L+pbft.K {
		t.Fatalf("Expected low water mark to be %d, got %d", pbft.L+pbft.K, pbft.h)
	}

	// Send enough requests to get to a weak checkpoint certificate certain with sequence number L+K*2
	for i := int64(pbft.L + pbft.K + 1); uint64(i) <= pbft.L+pbft.K*2; i++ {
		execReqBatch(i, false)
	}

	if !pep.sc.skipOccurred {
		t.Fatalf("Request failed to kick off state transfer")
	}

	execReqBatch(int64(pbft.L+pbft.K*2+1), false)

	if pep.sc.executions < pbft.L+pbft.K*2 {
		t.Fatalf("Replica did not perform state transfer")
	}

	// XXX currently disabled, need to resync view# during/after state transfer
	// if pep.sc.executions != pbft.L+pbft.K*2+1 {
	// 	t.Fatalf("Replica did not begin participating normally after state transfer completed")
	//}
}

func TestPbftF0(t *testing.T) {
	net := makePBFTNetwork(1, nil)
	defer net.stop()

	reqBatch := createPbftReqBatch(1, 0)
	net.pbftEndpoints[0].manager.Queue() <- reqBatch

	err := net.process()
	if err != nil {
		t.Fatalf("Processing failed: %s", err)
	}

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions < 1 {
			t.Errorf("Instance %d did not execute transaction", pep.id)
			continue
		}
		if pep.sc.executions >= 2 {
			t.Errorf("Instance %d executed more than one transaction", pep.id)
			continue
		}
		if !reflect.DeepEqual(pep.sc.lastExecution, hash(reqBatch.GetBatch()[0])) {
			t.Errorf("Instance %d executed wrong transaction, %x should be %x",
				pep.id, pep.sc.lastExecution, hash(reqBatch.GetBatch()[0]))
		}
	}
}

// Make sure the request timer doesn't inflate the view timeout by firing during view change
func TestRequestTimerDuringViewChange(t *testing.T) {
	mock := &omniProto{
		signImpl:   func(msg []byte) ([]byte, error) { return msg, nil },
		verifyImpl: func(senderID uint64, signature []byte, message []byte) error { return nil },
		broadcastImpl: func(msg []byte) {
			t.Errorf("Should not send the view change message during a view change")
		},
	}
	instance, manager := createRunningPbftWithManager(1, loadConfig(), mock)
	defer manager.Halt()
	instance.f = 1
	instance.K = 2
	instance.L = 4
	instance.requestTimeout = time.Millisecond
	instance.activeView = false
	defer instance.close()

	manager.Queue() <- createPbftReqBatch(1, 1) // replica ID should not correspond to the primary

	time.Sleep(100 * time.Millisecond)
}

// TestReplicaCrash1 simulates the restart of replicas 0 and 1 after
// some state has been built (one request executed).  At the time of
// the restart, replica 0 is also the primary.  All three requests
// submitted should also be executed on all replicas.
func TestReplicaCrash1(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.K", 2)
	config.Set("general.logmultiplier", 2)
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(1, uint64(generateBroadcaster(validatorCount)))
	net.process()

	for id := 0; id < 2; id++ {
		pe := net.pbftEndpoints[id]
		pe.pbft = newPbftCore(uint64(id), loadConfig(), pe.sc, events.NewTimerFactoryImpl(pe.manager))
		pe.manager.SetReceiver(pe.pbft)
		pe.pbft.N = 4
		pe.pbft.f = (4 - 1) / 3
		pe.pbft.K = 2
		pe.pbft.L = 2 * pe.pbft.K
	}

	net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(2, uint64(generateBroadcaster(validatorCount)))
	net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(3, uint64(generateBroadcaster(validatorCount)))
	net.process()

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions != 3 {
			t.Errorf("Expected 3 executions on replica %d, got %d", pep.id, pep.sc.executions)
			continue
		}

		if pep.pbft.view != 0 {
			t.Errorf("Replica %d should still be in view 0, is %v %d", pep.id, pep.pbft.activeView, pep.pbft.view)
		}
	}
}

// TestReplicaCrash2 is a misnomer.  It simulates a situation where
// one replica (#3) is byzantine and does not participate at all.
// Additionally, for view<2 and seqno=1, the network drops commit
// messages to all but replica 1.
func TestReplicaCrash2(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.timeout.request", "800ms")
	config.Set("general.timeout.viewchange", "800ms")
	config.Set("general.K", 2)
	config.Set("general.logmultiplier", 2)
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	filterMsg := true
	net.filterFn = func(src int, dst int, msg []byte) []byte {
		if dst == 3 { // 3 is byz
			return nil
		}
		pm := &Message{}
		err := proto.Unmarshal(msg, pm)
		if err != nil {
			t.Fatal(err)
		}
		// filter commits to all but 1
		commit := pm.GetCommit()
		if filterMsg && dst != -1 && dst != 1 && commit != nil && commit.View < 2 {
			logger.Infof("filtering commit message from %d to %d", src, dst)
			return nil
		}
		return msg
	}

	net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(1, uint64(generateBroadcaster(validatorCount)))
	net.process()

	logger.Info("stopping filtering")
	filterMsg = false
	primary := net.pbftEndpoints[0].pbft.primary(net.pbftEndpoints[0].pbft.view)
	net.pbftEndpoints[primary].manager.Queue() <- createPbftReqBatch(2, uint64(generateBroadcaster(validatorCount)))
	net.pbftEndpoints[primary].manager.Queue() <- createPbftReqBatch(3, uint64(generateBroadcaster(validatorCount)))
	net.pbftEndpoints[primary].manager.Queue() <- createPbftReqBatch(4, uint64(generateBroadcaster(validatorCount)))
	go net.processContinually()
	time.Sleep(5 * time.Second)

	for _, pep := range net.pbftEndpoints {
		if pep.id != 3 && pep.sc.executions != 4 {
			t.Errorf("Expected 4 executions on replica %d, got %d", pep.id, pep.sc.executions)
			continue
		}
		if pep.id == 3 && pep.sc.executions > 0 {
			t.Errorf("Expected no execution")
			continue
		}
	}
}

// TestReplicaCrash3 simulates the restart requiring a view change
// to a checkpoint which was restored from the persistence state
// Replicas 0,1,2 participate up to a checkpoint, then all crash
// Then replicas 0,1,3 start back up, and a view change must be
// triggered to get vp3 up to speed
func TestReplicaCrash3(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.K", 2)
	config.Set("general.logmultiplier", 2)
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	twoOffline := false
	threeOffline := true
	net.filterFn = func(src int, dst int, msg []byte) []byte {
		if twoOffline && dst == 2 { // 2 is 'offline'
			return nil
		}
		if threeOffline && dst == 3 { // 3 is 'offline'
			return nil
		}
		return msg
	}

	for i := int64(1); i <= 8; i++ {
		net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(i, uint64(generateBroadcaster(validatorCount)))
	}
	net.process() // vp0,1,2 should have a stable checkpoint for seqNo 8

	// Create new pbft instances to restore from persistence
	for id := 0; id < 2; id++ {
		pe := net.pbftEndpoints[id]
		config := loadConfig()
		config.Set("general.K", "2")
		pe.pbft.close()
		pe.pbft = newPbftCore(uint64(id), config, pe.sc, events.NewTimerFactoryImpl(pe.manager))
		pe.manager.SetReceiver(pe.pbft)
		pe.pbft.N = 4
		pe.pbft.f = (4 - 1) / 3
		pe.pbft.requestTimeout = 200 * time.Millisecond
	}

	threeOffline = false
	twoOffline = true

	// Because vp2 is 'offline', and vp3 is still at the genesis block, the network needs to make a view change

	net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(9, uint64(generateBroadcaster(validatorCount)))
	net.process()

	// Now vp0,1,3 should be in sync with 9 executions in view 1, and vp2 should be at 8 executions in view 0
	for i, pep := range net.pbftEndpoints {

		if i == 2 {
			// 2 is 'offline'
			if pep.pbft.view != 0 {
				t.Errorf("Expected replica %d to be in view 0, got %d", pep.id, pep.pbft.view)
			}
			expectedExecutions := uint64(8)
			if pep.sc.executions != expectedExecutions {
				t.Errorf("Expected %d executions on replica %d, got %d", expectedExecutions, pep.id, pep.sc.executions)
			}
			continue
		}

		if pep.pbft.view != 1 {
			t.Errorf("Expected replica %d to be in view 1, got %d", pep.id, pep.pbft.view)
		}

		expectedExecutions := uint64(9)
		if pep.sc.executions != expectedExecutions {
			t.Errorf("Expected %d executions on replica %d, got %d", expectedExecutions, pep.id, pep.sc.executions)
		}
	}
}

// TestReplicaCrash4 simulates the restart with no checkpoints
// in the store because they have been garbage collected
// the bug occurs because the low watermark is incorrectly set to
// be zero
func TestReplicaCrash4(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.K", 2)
	config.Set("general.logmultiplier", 2)
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	twoOffline := false
	threeOffline := true
	net.filterFn = func(src int, dst int, msg []byte) []byte {
		if twoOffline && dst == 2 { // 2 is 'offline'
			return nil
		}
		if threeOffline && dst == 3 { // 3 is 'offline'
			return nil
		}
		return msg
	}

	for i := int64(1); i <= 8; i++ {
		net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(i, uint64(generateBroadcaster(validatorCount)))
	}
	net.process() // vp0,1,2 should have a stable checkpoint for seqNo 8
	net.process() // this second time is necessary for garbage collection it seams

	// Now vp0,1,2 should be in sync with 8 executions in view 0, and vp4 should be offline
	for i, pep := range net.pbftEndpoints {

		if i == 3 {
			// 3 is offline for this test
			continue
		}

		if pep.pbft.view != 0 {
			t.Errorf("Expected replica %d to be in view 1, got %d", pep.id, pep.pbft.view)
		}

		expectedExecutions := uint64(8)
		if pep.sc.executions != expectedExecutions {
			t.Errorf("Expected %d executions on replica %d, got %d", expectedExecutions, pep.id, pep.sc.executions)
		}
	}

	// Create new pbft instances to restore from persistence
	for id := 0; id < 3; id++ {
		pe := net.pbftEndpoints[id]
		config := loadConfig()
		config.Set("general.K", "2")
		pe.pbft.close()
		pe.pbft = newPbftCore(uint64(id), config, pe.sc, events.NewTimerFactoryImpl(pe.manager))
		pe.manager.SetReceiver(pe.pbft)
		pe.pbft.N = 4
		pe.pbft.f = (4 - 1) / 3
		pe.pbft.requestTimeout = 200 * time.Millisecond

		expected := uint64(8)
		if pe.pbft.h != expected {
			t.Errorf("Low watermark should have been %d, got %d", expected, pe.pbft.h)
		}
	}

}
func TestReplicaPersistQSet(t *testing.T) {
	persist := make(map[string][]byte)

	stack := &omniProto{
		broadcastImpl: func(msg []byte) {
		},
		StoreStateImpl: func(key string, value []byte) error {
			persist[key] = value
			return nil
		},
		DelStateImpl: func(key string) {
			delete(persist, key)
		},
		ReadStateImpl: func(key string) ([]byte, error) {
			if val, ok := persist[key]; ok {
				return val, nil
			}
			return nil, fmt.Errorf("key not found")
		},
		ReadStateSetImpl: func(prefix string) (map[string][]byte, error) {
			r := make(map[string][]byte)
			for k, v := range persist {
				if len(k) >= len(prefix) && k[0:len(prefix)] == prefix {
					r[k] = v
				}
			}
			return r, nil
		},
	}
	p := newPbftCore(1, loadConfig(), stack, &inertTimerFactory{})
	reqBatch := createPbftReqBatch(1, 0)
	events.SendEvent(p, &PrePrepare{
		View:           0,
		SequenceNumber: 1,
		BatchDigest:    hash(reqBatch),
		RequestBatch:   reqBatch,
		ReplicaId:      uint64(0),
	})
	p.close()

	p = newPbftCore(1, loadConfig(), stack, &inertTimerFactory{})
	if !p.prePrepared(hash(reqBatch), 0, 1) {
		t.Errorf("did not restore qset properly")
	}
}

func TestReplicaPersistDelete(t *testing.T) {
	persist := make(map[string][]byte)

	stack := &omniProto{
		StoreStateImpl: func(key string, value []byte) error {
			persist[key] = value
			return nil
		},
		DelStateImpl: func(key string) {
			delete(persist, key)
		},
	}
	p := newPbftCore(1, loadConfig(), stack, &inertTimerFactory{})
	p.reqBatchStore["a"] = &RequestBatch{}
	p.persistRequestBatch("a")
	if len(persist) != 1 {
		t.Error("expected one persisted entry")
	}
	p.persistDelRequestBatch("a")
	if len(persist) != 0 {
		t.Error("expected no persisted entry")
	}
}

func TestNilCurrentExec(t *testing.T) {
	p := newPbftCore(1, loadConfig(), &omniProto{}, &inertTimerFactory{})
	p.execDoneSync() // Per issue 1538, this would cause a Nil pointer dereference
}

func TestNetworkNullRequests(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.timeout.nullrequest", "200ms")
	config.Set("general.timeout.request", "500ms")
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(1, 0)

	go net.processContinually()
	time.Sleep(3 * time.Second)

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions != 1 {
			t.Errorf("Instance %d executed incorrect number of transactions: %d", pep.id, pep.sc.executions)
		}
		if pep.pbft.lastExec <= 1 {
			t.Errorf("Instance %d: no null requests processed", pep.id)
		}
		if pep.pbft.view != 0 {
			t.Errorf("Instance %d: expected view=0", pep.id)
		}
	}
}

func TestNetworkNullRequestMissing(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.timeout.nullrequest", "200ms")
	config.Set("general.timeout.request", "500ms")
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	net.pbftEndpoints[0].pbft.nullRequestTimeout = 0

	net.pbftEndpoints[0].manager.Queue() <- createPbftReqBatch(1, 0)

	go net.processContinually()
	time.Sleep(3 * time.Second) // Bumped from 2 to 3 seconds because of sporadic CI failures

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions != 1 {
			t.Errorf("Instance %d executed incorrect number of transactions: %d", pep.id, pep.sc.executions)
		}
		if pep.pbft.lastExec <= 1 {
			t.Errorf("Instance %d: no null requests processed", pep.id)
		}
		if pep.pbft.view != 1 {
			t.Errorf("Instance %d: expected view=1", pep.id)
		}
	}
}

func TestNetworkPeriodicViewChange(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.K", "2")
	config.Set("general.logmultiplier", "2")
	config.Set("general.timeout.request", "500ms")
	config.Set("general.viewchangeperiod", "1")
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	for n := 1; n < 6; n++ {
		for _, pe := range net.pbftEndpoints {
			pe.manager.Queue() <- createPbftReqBatch(int64(n), 0)
		}
		net.process()
	}

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions != 5 {
			t.Errorf("Instance %d executed incorrect number of transactions: %d", pep.id, pep.sc.executions)
		}
		// We should be in view 2, 2 exec, VC, 2 exec, VC, exec
		if pep.pbft.view != 2 {
			t.Errorf("Instance %d: expected view=2", pep.id)
		}
	}
}

func TestNetworkPeriodicViewChangeMissing(t *testing.T) {
	validatorCount := 4
	config := loadConfig()
	config.Set("general.K", "2")
	config.Set("general.logmultiplier", "2")
	config.Set("general.timeout.request", "500ms")
	config.Set("general.viewchangeperiod", "1")
	net := makePBFTNetwork(validatorCount, config)
	defer net.stop()

	net.pbftEndpoints[0].pbft.viewChangePeriod = 0
	net.pbftEndpoints[0].pbft.viewChangeSeqNo = ^uint64(0)

	for n := 1; n < 3; n++ {
		for _, pe := range net.pbftEndpoints {
			pe.manager.Queue() <- createPbftReqBatch(int64(n), 0)
		}
		net.process()
	}

	for _, pep := range net.pbftEndpoints {
		if pep.sc.executions != 2 {
			t.Errorf("Instance %d executed incorrect number of transactions: %d", pep.id, pep.sc.executions)
		}
		if pep.pbft.view != 1 {
			t.Errorf("Instance %d: expected view=1", pep.id)
		}
	}
}

// TestViewChangeCannotExecuteToCheckpoint tests a replica mid-execution, which receives a view change to a checkpoint above its watermarks
// but does _not_ have enough commit certificates to reach the checkpoint. state should transfer
func TestViewChangeCannotExecuteToCheckpoint(t *testing.T) {
	instance := newPbftCore(3, loadConfig(), &omniProto{
		broadcastImpl:       func(b []byte) {},
		getStateImpl:        func() []byte { return []byte("state") },
		signImpl:            func(b []byte) ([]byte, error) { return b, nil },
		verifyImpl:          func(senderID uint64, signature []byte, message []byte) error { return nil },
		invalidateStateImpl: func() {},
	}, &inertTimerFactory{})
	instance.activeView = false
	instance.view = 1
	newViewBaseSeqNo := uint64(10)
	nextExec := uint64(6)
	instance.currentExec = &nextExec

	for i := instance.lastExec; i < newViewBaseSeqNo; i++ {
		commit := &Commit{View: 0, SequenceNumber: i}
		prepare := &Prepare{View: 0, SequenceNumber: i}
		instance.certStore[msgID{v: 0, n: i}] = &msgCert{
			digest:     "", // null request
			prePrepare: &PrePrepare{View: 0, SequenceNumber: i},
			prepare:    []*Prepare{prepare, prepare, prepare},
			commit:     []*Commit{commit, commit, commit},
		}
	}

	vset := make([]*ViewChange, 3)

	cset := []*ViewChange_C{
		{
			SequenceNumber: newViewBaseSeqNo,
			Id:             base64.StdEncoding.EncodeToString([]byte("Ten")),
		},
	}

	for i := 0; i < 3; i++ {
		// Replica 0 sent checkpoints for 100
		vset[i] = &ViewChange{
			H:    newViewBaseSeqNo,
			Cset: cset,
		}
	}

	xset := make(map[uint64]string)
	xset[11] = ""

	instance.lastExec = 9

	instance.newViewStore[1] = &NewView{
		View:      1,
		Vset:      vset,
		Xset:      xset,
		ReplicaId: 1,
	}

	if _, ok := instance.processNewView().(viewChangedEvent); !ok {
		t.Fatalf("Should have processed the new view")
	}

	if !instance.skipInProgress {
		t.Fatalf("Should have done state transfer")
	}
}

// TestViewChangeCanExecuteToCheckpoint tests a replica mid-execution, which receives a view change to a checkpoint above its watermarks
// but which has enough commit certificates to reach the checkpoint. State should not transfer and executions should trigger the view change
func TestViewChangeCanExecuteToCheckpoint(t *testing.T) {
	instance := newPbftCore(3, loadConfig(), &omniProto{
		broadcastImpl: func(b []byte) {},
		getStateImpl:  func() []byte { return []byte("state") },
		signImpl:      func(b []byte) ([]byte, error) { return b, nil },
		verifyImpl:    func(senderID uint64, signature []byte, message []byte) error { return nil },
		skipToImpl: func(s uint64, id []byte, replicas []uint64) {
			t.Fatalf("Should not have performed state transfer, should have caught up via execution")
		},
	}, &inertTimerFactory{})
	instance.activeView = false
	instance.view = 1
	instance.lastExec = 5
	newViewBaseSeqNo := uint64(10)
	nextExec := uint64(6)
	instance.currentExec = &nextExec

	for i := nextExec + 1; i <= newViewBaseSeqNo; i++ {
		commit := &Commit{View: 0, SequenceNumber: i}
		prepare := &Prepare{View: 0, SequenceNumber: i}
		instance.certStore[msgID{v: 0, n: i}] = &msgCert{
			digest:     "", // null request
			prePrepare: &PrePrepare{View: 0, SequenceNumber: i},
			prepare:    []*Prepare{prepare, prepare, prepare},
			commit:     []*Commit{commit, commit, commit},
		}
	}

	vset := make([]*ViewChange, 3)

	cset := []*ViewChange_C{
		{
			SequenceNumber: newViewBaseSeqNo,
			Id:             base64.StdEncoding.EncodeToString([]byte("Ten")),
		},
	}

	for i := 0; i < 3; i++ {
		// Replica 0 sent checkpoints for 100
		vset[i] = &ViewChange{
			H:    newViewBaseSeqNo,
			Cset: cset,
		}
	}

	xset := make(map[uint64]string)
	xset[11] = ""

	instance.lastExec = 9

	instance.newViewStore[1] = &NewView{
		View:      1,
		Vset:      vset,
		Xset:      xset,
		ReplicaId: 1,
	}

	if instance.processNewView() != nil {
		t.Fatalf("Should not have processed the new view")
	}

	events.SendEvent(instance, execDoneEvent{})

	if !instance.activeView {
		t.Fatalf("Should have finished processing new view after executions")
	}
}

func TestViewWithOldSeqNos(t *testing.T) {
	instance := newPbftCore(3, loadConfig(), &omniProto{
		broadcastImpl: func(b []byte) {},
		signImpl:      func(b []byte) ([]byte, error) { return b, nil },
		verifyImpl:    func(senderID uint64, signature []byte, message []byte) error { return nil },
	}, &inertTimerFactory{})
	instance.activeView = false
	instance.view = 1

	vset := make([]*ViewChange, 3)

	cset := []*ViewChange_C{
		{
			SequenceNumber: 0,
			Id:             base64.StdEncoding.EncodeToString([]byte("Zero")),
		},
	}

	qset := []*ViewChange_PQ{
		{
			SequenceNumber: 9,
			BatchDigest:    "nine",
			View:           0,
		},
		{
			SequenceNumber: 2,
			BatchDigest:    "two",
			View:           0,
		},
	}

	for i := 0; i < 3; i++ {
		// Replica 0 sent checkpoints for 100
		vset[i] = &ViewChange{
			H:    0,
			Cset: cset,
			Qset: qset,
			Pset: qset,
		}
	}

	xset := instance.assignSequenceNumbers(vset, 0)

	instance.lastExec = 10
	instance.moveWatermarks(instance.lastExec)

	instance.newViewStore[1] = &NewView{
		View:      1,
		Vset:      vset,
		Xset:      xset,
		ReplicaId: 1,
	}

	if _, ok := instance.processNewView().(viewChangedEvent); !ok {
		t.Fatalf("Failed to successfully process new view")
	}

	for idx, val := range instance.certStore {
		if idx.n < instance.h {
			t.Errorf("Found %+v=%+v in certStore who's seqNo < %d", idx, val, instance.h)
		}
	}
}

func TestViewChangeDuringExecution(t *testing.T) {
	skipped := false
	instance := newPbftCore(3, loadConfig(), &omniProto{
		viewChangeImpl: func(v uint64) {},
		skipToImpl: func(s uint64, id []byte, replicas []uint64) {
			skipped = true
		},
		invalidateStateImpl: func() {},
		broadcastImpl:       func(b []byte) {},
		signImpl:            func(b []byte) ([]byte, error) { return b, nil },
		verifyImpl:          func(senderID uint64, signature []byte, message []byte) error { return nil },
	}, &inertTimerFactory{})
	instance.activeView = false
	instance.view = 1
	instance.lastExec = 1
	nextExec := uint64(2)
	instance.currentExec = &nextExec

	vset := make([]*ViewChange, 3)

	cset := []*ViewChange_C{
		{
			SequenceNumber: 100,
			Id:             base64.StdEncoding.EncodeToString([]byte("onehundred")),
		},
	}

	// Replica 0 sent checkpoints for 100
	vset[0] = &ViewChange{
		H:    90,
		Cset: cset,
	}

	// Replica 1 sent checkpoints for 10
	vset[1] = &ViewChange{
		H:    90,
		Cset: cset,
	}

	// Replica 2 sent checkpoints for 10
	vset[2] = &ViewChange{
		H:    90,
		Cset: cset,
	}

	xset := make(map[uint64]string)
	xset[101] = ""

	instance.newViewStore[1] = &NewView{
		View:      1,
		Vset:      vset,
		Xset:      xset,
		ReplicaId: 1,
	}

	if _, ok := instance.processNewView().(viewChangedEvent); !ok {
		t.Fatalf("Failed to successfully process new view")
	}

	if skipped {
		t.Fatalf("Expected state transfer not to be kicked off until execution completes")
	}

	events.SendEvent(instance, execDoneEvent{})

	if !skipped {
		t.Fatalf("Expected state transfer to be kicked off once execution completed")
	}
}

func TestStateTransferCheckpoint(t *testing.T) {
	broadcasts := 0
	instance := newPbftCore(3, loadConfig(), &omniProto{
		broadcastImpl: func(msg []byte) {
			broadcasts++
		},
		validateStateImpl: func() {},
	}, &inertTimerFactory{})

	id := []byte("My ID")
	events.SendEvent(instance, stateUpdatedEvent{
		chkpt: &checkpointMessage{
			seqNo: 10,
			id:    id,
		},
		target: &pb.BlockchainInfo{},
	})

	if broadcasts != 1 {
		t.Fatalf("Should have broadcast a checkpoint after the state transfer finished")
	}

}

func TestStateTransferredToOldPoint(t *testing.T) {
	skipped := false
	instance := newPbftCore(3, loadConfig(), &omniProto{
		skipToImpl: func(s uint64, id []byte, replicas []uint64) {
			skipped = true
		},
		invalidateStateImpl: func() {},
	}, &inertTimerFactory{})
	instance.moveWatermarks(90)
	instance.updateHighStateTarget(&stateUpdateTarget{
		checkpointMessage: checkpointMessage{
			seqNo: 100,
			id:    []byte("onehundred"),
		},
	})

	events.SendEvent(instance, stateUpdatedEvent{
		chkpt: &checkpointMessage{
			seqNo: 10,
		},
	})

	if !skipped {
		t.Fatalf("Expected state transfer to be kicked off once execution completed")
	}
}

func TestStateNetworkMovesOnDuringSlowStateTransfer(t *testing.T) {
	instance := newPbftCore(3, loadConfig(), &omniProto{
		skipToImpl:          func(s uint64, id []byte, replicas []uint64) {},
		invalidateStateImpl: func() {},
	}, &inertTimerFactory{})
	instance.skipInProgress = true

	seqNo := uint64(20)

	for i := uint64(0); i < 3; i++ {
		events.SendEvent(instance, &Checkpoint{
			SequenceNumber: seqNo,
			ReplicaId:      i,
			Id:             base64.StdEncoding.EncodeToString([]byte("twenty")),
		})
	}

	if instance.h != seqNo {
		t.Fatalf("Expected watermark movement to %d because of state transfer, but low watermark is %d", seqNo, instance.h)
	}
}

// This test is designed to ensure the peer panics if the value of the weak cert is different from its own checkpoint
func TestCheckpointDiffersFromWeakCert(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Weak checkpoint certificate different from own, should have panicked.")
		}
	}()

	instance := newPbftCore(3, loadConfig(), &omniProto{}, &inertTimerFactory{})

	badChkpt := &Checkpoint{
		SequenceNumber: 10,
		Id:             "WRONG",
		ReplicaId:      3,
	}
	instance.chkpts[10] = badChkpt.Id // This is done via the exec path, shortcut it here
	events.SendEvent(instance, badChkpt)

	for i := uint64(0); i < 2; i++ {
		events.SendEvent(instance, &Checkpoint{
			SequenceNumber: 10,
			Id:             "CORRECT",
			ReplicaId:      i,
		})
	}

	if instance.highStateTarget != nil {
		t.Fatalf("State target should not have been updated")
	}
}

// This test is designed to ensure the peer panics if it observes > f+1 different checkpoint values for the same seqNo
// This indicates a network that will be unable to move its watermarks and thus progress
func TestNoCheckpointQuorum(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("More than f+1 different checkpoint values found, should have panicked.")
		}
	}()

	instance := newPbftCore(3, loadConfig(), &omniProto{}, &inertTimerFactory{})

	for i := uint64(0); i < 3; i++ {
		events.SendEvent(instance, &Checkpoint{
			SequenceNumber: 10,
			Id:             strconv.FormatUint(i, 10),
			ReplicaId:      i,
		})
	}
}
