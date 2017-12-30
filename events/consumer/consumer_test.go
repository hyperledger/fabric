/*
Copyright Hitachi America, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consumer

import (
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	coreutil "github.com/hyperledger/fabric/core/testutil"
	"github.com/hyperledger/fabric/events/producer"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	ehpb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type MockAdapter struct {
	sync.RWMutex
	notify chan struct{}
}

type ZeroAdapter struct {
	sync.RWMutex
	notify chan struct{}
}

type BadAdapter struct {
	sync.RWMutex
	notify chan struct{}
}

var peerAddress = "0.0.0.0:7303"
var ies = []*ehpb.Interest{{EventType: ehpb.EventType_CHAINCODE, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event1"}}}}
var testCert = &x509.Certificate{
	Raw: []byte("test"),
}

var adapter *MockAdapter
var obcEHClient *EventsClient

func (a *ZeroAdapter) GetInterestedEvents() ([]*ehpb.Interest, error) {
	return []*ehpb.Interest{}, nil
}
func (a *ZeroAdapter) Recv(msg *ehpb.Event) (bool, error) {
	panic("not implemented")
}
func (a *ZeroAdapter) Disconnected(err error) {
	panic("not implemented")
}

func (a *BadAdapter) GetInterestedEvents() ([]*ehpb.Interest, error) {
	return []*ehpb.Interest{}, fmt.Errorf("Error")
}
func (a *BadAdapter) Recv(msg *ehpb.Event) (bool, error) {
	panic("not implemented")
}
func (a *BadAdapter) Disconnected(err error) {
	panic("not implemented")
}

func (a *MockAdapter) GetInterestedEvents() ([]*ehpb.Interest, error) {
	return []*ehpb.Interest{
		{EventType: ehpb.EventType_BLOCK},
	}, nil
}

func (a *MockAdapter) Recv(msg *ehpb.Event) (bool, error) {
	return true, nil
}

func (a *MockAdapter) Disconnected(err error) {}

func TestNewEventsClient(t *testing.T) {
	var cases = []struct {
		name     string
		time     int
		expected bool
	}{
		{
			name:     "success",
			time:     5,
			expected: true,
		},
		{
			name:     "fail. regTimout < 100ms",
			time:     0,
			expected: false,
		},
		{
			name:     "fail. regTimeout > 60s",
			time:     61,
			expected: false,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Logf("Running test: %s", test.name)
			var regTimeout = time.Duration(test.time) * time.Second
			done := make(chan struct{})
			adapter = &MockAdapter{notify: done}

			_, err := NewEventsClient(peerAddress, regTimeout, adapter)
			if test.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestNewEventsClientConnectionWithAddress(t *testing.T) {
	var cases = []struct {
		name     string
		address  string
		expected bool
	}{
		{
			name:     "success",
			address:  peerAddress,
			expected: true,
		},
		{
			name:     "fail",
			address:  "",
			expected: false,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Logf("Running test: %s", test.name)
			_, err := newEventsClientConnectionWithAddress(test.address)
			if test.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestUnregisterAsync(t *testing.T) {
	var err error
	done := make(chan struct{})
	adapter := &MockAdapter{notify: done}

	obcEHClient, _ = NewEventsClient(peerAddress, 5, adapter)

	if err = obcEHClient.Start(); err != nil {
		obcEHClient.Stop()
		t.Fail()
	}

	regConfig := &RegistrationConfig{InterestedEvents: ies, Timestamp: util.CreateUtcTimestamp(), TlsCert: testCert}
	obcEHClient.RegisterAsync(regConfig)
	err = obcEHClient.UnregisterAsync(ies)
	assert.NoError(t, err)

	obcEHClient.Stop()

}

func TestStart(t *testing.T) {
	var err error
	var regTimeout = 5 * time.Second
	done := make(chan struct{})

	var cases = []struct {
		name     string
		address  string
		adapter  EventAdapter
		expected bool
	}{
		{
			name:     "success",
			address:  peerAddress,
			adapter:  &MockAdapter{notify: done},
			expected: true,
		},
		{
			name:     "fail no peerAddress",
			address:  "",
			adapter:  &MockAdapter{notify: done},
			expected: false,
		},
		{
			name:     "fail bad adapter",
			address:  peerAddress,
			adapter:  &BadAdapter{notify: done},
			expected: false,
		},
		{
			name:     "fail zero adapter",
			address:  peerAddress,
			adapter:  &ZeroAdapter{notify: done},
			expected: false,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Logf("Running test: %s", test.name)
			obcEHClient, _ = NewEventsClient(test.address, regTimeout, test.adapter)
			err = obcEHClient.Start()
			if test.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			obcEHClient.Stop()
		})
	}
}

func TestStop(t *testing.T) {
	var err error
	var regTimeout = 5 * time.Second
	done := make(chan struct{})
	adapter := &MockAdapter{notify: done}

	obcEHClient, _ = NewEventsClient(peerAddress, regTimeout, adapter)

	if err = obcEHClient.Start(); err != nil {
		t.Fail()
		t.Logf("Error client start %s", err)
	}
	err = obcEHClient.Stop()
	assert.NoError(t, err)

}

func TestMain(m *testing.M) {
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not initialize msp, err %s", err)
		os.Exit(-1)
		return
	}

	coreutil.SetupTestConfig()
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		fmt.Printf("Error starting events listener %s....not doing tests", err)
		return
	}

	extract := func(msg proto.Message) []byte {
		evt, isEvent := msg.(*ehpb.Event)
		if !isEvent || evt == nil {
			return nil
		}
		return evt.TlsCertHash
	}

	ehConfig := &producer.EventsServerConfig{
		BufferSize:       uint(viper.GetInt("peer.events.buffersize")),
		Timeout:          viper.GetDuration("peer.events.timeout"),
		TimeWindow:       viper.GetDuration("peer.events.timewindow"),
		BindingInspector: comm.NewBindingInspector(false, extract)}
	ehServer := producer.NewEventsServer(ehConfig)

	ehpb.RegisterEventsServer(grpcServer, ehServer)

	go grpcServer.Serve(lis)

	time.Sleep(2 * time.Second)
	os.Exit(m.Run())
}
