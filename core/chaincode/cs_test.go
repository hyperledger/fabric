/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/stretchr/testify/assert"
)

// sleepTime - еhe time that must be set for the Init transaction to execute.
// If you will be debugging, you should increase it.
const sleepTime = time.Second

func TestLaunchTestAndExecInit(t *testing.T) {
	// 1. prepare test
	invokeInfo := &lifecycle.ChaincodeEndorsementInfo{
		Version:     "definition-version",
		ChaincodeID: "test-chaincode-name",
	}

	fakeLifecycle := &mock.Lifecycle{}
	fakeSimulator := &mock.TxSimulator{}
	fakeSimulator.GetStateReturns([]byte("old-cc-version"), nil)
	fakeLifecycle.ChaincodeEndorsementInfoReturns(invokeInfo, nil)

	userRunsCC := true
	chaincodeHandlerRegistry := NewHandlerRegistry(userRunsCC)

	// fakeChatStream := &mock.ChaincodeStream{}

	h := &Handler{
		chaincodeID: invokeInfo.ChaincodeID,
		// chatStream:  fakeChatStream,
	}

	startInitChaincode := make(chan struct{})
	result := make(chan string, 2)

	ctx, cancel := context.WithCancel(context.Background())
	fakeRouter := &mock.ContainerRouter{}
	fakeRouter.WaitCalls(func(_ string) (int, error) {
		// 4. register handler
		err := chaincodeHandlerRegistry.Register(h)
		assert.NoError(t, err)

		h.stateLock.Lock()
		h.state = Established
		h.stateLock.Unlock()

		// 5. send signal for start init chaincode
		close(startInitChaincode)

		time.Sleep(sleepTime)

		// 6. send ready
		result <- "ready"

		h.stateLock.Lock()
		h.state = Ready
		h.stateLock.Unlock()

		chaincodeHandlerRegistry.Ready(invokeInfo.ChaincodeID)

		// wait end of test
		<-ctx.Done()
		return 0, nil
	})

	containerRuntime := &ContainerRuntime{
		BuildRegistry:   &container.BuildRegistry{},
		ContainerRouter: fakeRouter,
	}

	ca, _ := tlsgen.NewCA()
	certGenerator := accesscontrol.NewAuthenticator(ca)

	chaincodeLauncher := &RuntimeLauncher{
		Metrics:        NewLaunchMetrics(&disabled.Provider{}),
		Runtime:        containerRuntime,
		Registry:       chaincodeHandlerRegistry,
		StartupTimeout: 300 * time.Second,
		CACert:         ca.CertBytes(),
		CertGenerator:  certGenerator,
	}

	txParams := &ccprovider.TransactionParams{
		ChannelID:   "channel-id",
		TXSimulator: fakeSimulator,
	}

	cs := &ChaincodeSupport{
		Lifecycle:       fakeLifecycle,
		HandlerRegistry: chaincodeHandlerRegistry,
		Launcher:        chaincodeLauncher,
	}

	wg := &sync.WaitGroup{}

	// 2. launch chaincode
	wg.Add(1)
	go func() {
		defer wg.Done()
		input := &pb.ChaincodeInput{
			Args: [][]byte{[]byte("launch")},
		}

		ccid, cctype, err := cs.CheckInvocation(txParams, invokeInfo.ChaincodeID, input)
		assert.NoError(t, err)
		assert.Equal(t, ccid, invokeInfo.ChaincodeID)
		assert.Equal(t, cctype, pb.ChaincodeMessage_TRANSACTION)

		_, err = cs.Launch(ccid)
		assert.NoError(t, err)
	}()

	// 3. init chaincode
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-startInitChaincode

		input := &pb.ChaincodeInput{
			Args: [][]byte{[]byte("init")},
		}

		ccid, cctype, err := cs.CheckInvocation(txParams, invokeInfo.ChaincodeID, input)
		assert.NoError(t, err)
		assert.Equal(t, ccid, invokeInfo.ChaincodeID)
		assert.Equal(t, cctype, pb.ChaincodeMessage_TRANSACTION)

		_, err = cs.Launch(ccid)
		assert.NoError(t, err)

		result <- "init"
	}()

	// 7. check result
	res := <-result
	assert.Equal(t, "ready", res)

	cancel()
	wg.Wait()
}
