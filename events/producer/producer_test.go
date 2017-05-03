/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package producer

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func createEvent() (*peer.Event, error) {
	events := make([]*peer.Interest, 2)
	events[0] = &peer.Interest{
		EventType: peer.EventType_BLOCK,
	}
	events[1] = &peer.Interest{
		EventType: peer.EventType_BLOCK,
		ChainID:   util.GetTestChainID(),
	}

	evt := &peer.Event{
		Event: &peer.Event_Register{
			Register: &peer.Register{
				Events: events,
			},
		},
		Creator: signerSerialized,
	}

	return evt, nil
}

var r *rand.Rand

func corrupt(bytes []byte) {
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().Unix()))
	}

	bytes[r.Int31n(int32(len(bytes)))]--
}

func TestSignedEvent(t *testing.T) {
	// get a test event
	evt, err := createEvent()
	if err != nil {
		t.Fatalf("createEvent failed, err %s", err)
		return
	}

	// sign it
	sEvt, err := utils.GetSignedEvent(evt, signer)
	if err != nil {
		t.Fatalf("GetSignedEvent failed, err %s", err)
		return
	}

	// validate it. Expected to succeed
	_, err = validateEventMessage(sEvt)
	if err != nil {
		t.Fatalf("validateEventMessage failed, err %s", err)
		return
	}

	// mess with the signature
	corrupt(sEvt.Signature)

	// validate it, it should fail
	_, err = validateEventMessage(sEvt)
	if err == nil {
		t.Fatalf("validateEventMessage should have failed")
		return
	}

	// get a bad signing identity
	badSigner, err := mmsp.NewNoopMsp().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatal("couldn't get noop signer")
		return
	}

	// sign it again with the bad signer
	sEvt, err = utils.GetSignedEvent(evt, badSigner)
	if err != nil {
		t.Fatalf("GetSignedEvent failed, err %s", err)
		return
	}

	// validate it, it should fail
	_, err = validateEventMessage(sEvt)
	if err == nil {
		t.Fatalf("validateEventMessage should have failed")
		return
	}
}

func TestNewEventsServer(t *testing.T) {
	viper.Set("peer.events.buffersize", 100)
	viper.Set("peer.events.timeout", "1ms")

	ehServer := NewEventsServer(
		uint(viper.GetInt("peer.events.buffersize")),
		viper.GetDuration("peer.events.timeout"))

	assert.NotNil(t, ehServer, "nil EventServer found")
}

var signer msp.SigningIdentity
var signerSerialized []byte

func TestMain(m *testing.M) {
	// setup crypto algorithms
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not initialize msp, err %s", err)
		os.Exit(-1)
		return
	}

	signer, err = mgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Println("Could not get signer")
		os.Exit(-1)
		return
	}

	signerSerialized, err = signer.Serialize()
	if err != nil {
		fmt.Println("Could not serialize identity")
		os.Exit(-1)
		return
	}

	os.Exit(m.Run())
}
