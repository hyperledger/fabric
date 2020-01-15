/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"sync"

	"github.com/hyperledger/fabric/core/container"
)

//go:generate counterfeiter -o mock/chaincode_launcher.go --fake-name ChaincodeLauncher . ChaincodeLauncher
type ChaincodeLauncher interface {
	Launch(ccid string) error
	Stop(ccid string) error
}

// ChaincodeCustodian is responsible for enqueuing builds and launches
// of chaincodes as they become available and stops when chaincodes
// are no longer referenced by an active chaincode definition.
type ChaincodeCustodian struct {
	cond       *sync.Cond
	mutex      sync.Mutex
	choreQueue []*chaincodeChore
	halt       bool
}

// chaincodeChore represents a unit of work to be performed by the worker
// routine. It identifies the chaincode the work is associated with.  If
// the work is to launch, then runnable is true (and stoppable is false).
// If the work is to stop, then stoppable is true (and runnable is false).
// If the work is simply to build, then runnable and stoppable are false.
type chaincodeChore struct {
	chaincodeID string
	runnable    bool
	stoppable   bool
}

// NewChaincodeCustodian creates an instance of a chaincode custodian.  It is the
// instantiator's responsibility to spawn a go routine to service the Work routine
// along with the appropriate dependencies.
func NewChaincodeCustodian() *ChaincodeCustodian {
	cc := &ChaincodeCustodian{}
	cc.cond = sync.NewCond(&cc.mutex)
	return cc
}

func (cc *ChaincodeCustodian) NotifyInstalled(chaincodeID string) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.choreQueue = append(cc.choreQueue, &chaincodeChore{
		chaincodeID: chaincodeID,
	})
	cc.cond.Signal()
}

func (cc *ChaincodeCustodian) NotifyInstalledAndRunnable(chaincodeID string) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.choreQueue = append(cc.choreQueue, &chaincodeChore{
		chaincodeID: chaincodeID,
		runnable:    true,
	})
	cc.cond.Signal()
}

func (cc *ChaincodeCustodian) NotifyStoppable(chaincodeID string) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.choreQueue = append(cc.choreQueue, &chaincodeChore{
		chaincodeID: chaincodeID,
		stoppable:   true,
	})
	cc.cond.Signal()
}

func (cc *ChaincodeCustodian) Close() {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.halt = true
	cc.cond.Signal()
}

func (cc *ChaincodeCustodian) Work(buildRegistry *container.BuildRegistry, builder ChaincodeBuilder, launcher ChaincodeLauncher) {
	for {
		cc.mutex.Lock()
		if len(cc.choreQueue) == 0 && !cc.halt {
			cc.cond.Wait()
		}
		if cc.halt {
			cc.mutex.Unlock()
			return
		}
		chore := cc.choreQueue[0]
		cc.choreQueue = cc.choreQueue[1:]
		cc.mutex.Unlock()

		if chore.runnable {
			if err := launcher.Launch(chore.chaincodeID); err != nil {
				logger.Warningf("could not launch chaincode '%s': %s", chore.chaincodeID, err)
			}
			continue
		}

		if chore.stoppable {
			if err := launcher.Stop(chore.chaincodeID); err != nil {
				logger.Warningf("could not stop chaincode '%s': %s", chore.chaincodeID, err)
			}
			continue
		}

		buildStatus, ok := buildRegistry.BuildStatus(chore.chaincodeID)
		if ok {
			logger.Debugf("skipping build of chaincode '%s' as it is already in progress", chore.chaincodeID)
			continue
		}
		err := builder.Build(chore.chaincodeID)
		if err != nil {
			logger.Warningf("could not build chaincode '%s': %s", chore.chaincodeID, err)
		}
		buildStatus.Notify(err)
	}
}
