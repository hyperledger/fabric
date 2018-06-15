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
	pb "github.com/hyperledger/fabric/protos/peer"
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
var ies = []*pb.Interest{{EventType: pb.EventType_BLOCK}}
var testCert = &x509.Certificate{
	Raw: []byte("test"),
}

var adapter *MockAdapter
var ehClient *EventsClient

func (a *ZeroAdapter) GetInterestedEvents() ([]*pb.Interest, error) {
	return []*pb.Interest{}, nil
}
func (a *ZeroAdapter) Recv(msg *pb.Event) (bool, error) {
	panic("not implemented")
}
func (a *ZeroAdapter) Disconnected(err error) {
	panic("not implemented")
}

func (a *BadAdapter) GetInterestedEvents() ([]*pb.Interest, error) {
	return []*pb.Interest{}, fmt.Errorf("Error")
}
func (a *BadAdapter) Recv(msg *pb.Event) (bool, error) {
	panic("not implemented")
}
func (a *BadAdapter) Disconnected(err error) {
	panic("not implemented")
}

func (a *MockAdapter) GetInterestedEvents() ([]*pb.Interest, error) {
	return ies, nil
}

func (a *MockAdapter) Recv(msg *pb.Event) (bool, error) {
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

	ehClient, _ = NewEventsClient(peerAddress, 5, adapter)

	if err = ehClient.Start(); err != nil {
		ehClient.Stop()
		t.Fail()
	}

	regConfig := &RegistrationConfig{
		InterestedEvents: ies,
		Timestamp:        util.CreateUtcTimestamp(),
		TlsCert:          testCert,
	}
	ehClient.RegisterAsync(regConfig)
	err = ehClient.UnregisterAsync(ies)
	assert.NoError(t, err)

	ehClient.Stop()
}

func TestStart(t *testing.T) {
	var err error
	var regTimeout = 1 * time.Second
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
			ehClient, _ = NewEventsClient(test.address, regTimeout, test.adapter)
			err = ehClient.Start()
			if test.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			ehClient.Stop()
		})
	}
}

func TestStop(t *testing.T) {
	var err error
	var regTimeout = 1 * time.Second
	done := make(chan struct{})
	adapter := &MockAdapter{notify: done}

	ehClient, _ = NewEventsClient(peerAddress, regTimeout, adapter)

	if err = ehClient.Start(); err != nil {
		t.Fail()
		t.Logf("Error starting client: %s", err)
	}
	err = ehClient.Stop()
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
		evt, isEvent := msg.(*pb.Event)
		if !isEvent || evt == nil {
			return nil
		}
		return evt.TlsCertHash
	}

	timeout := 10 * time.Millisecond
	ehConfig := &producer.EventsServerConfig{
		BufferSize:       100,
		Timeout:          timeout,
		SendTimeout:      timeout,
		TimeWindow:       time.Minute,
		BindingInspector: comm.NewBindingInspector(false, extract),
	}
	ehServer := producer.NewEventsServer(ehConfig)
	pb.RegisterEventsServer(grpcServer, ehServer)

	go grpcServer.Serve(lis)

	os.Exit(m.Run())
}
