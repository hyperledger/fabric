/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container

import (
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"

	"github.com/pkg/errors"
)

var vmLogger = flogging.MustGetLogger("container")

//go:generate counterfeiter -o mock/vm.go --fake-name VM . VM

//VM is an abstract virtual image for supporting arbitrary virual machines
type VM interface {
	Build(ccci *ccprovider.ChaincodeContainerInfo, codePackage []byte) (Instance, error)
}

//go:generate counterfeiter -o mock/instance.go --fake-name Instance . Instance

// Instance represents a built chaincode instance, because of the docker legacy, calling this a
// built 'container' would be very misleading, and going forward with the external launcher
// 'image' also seemed inappropriate.  So, the vague 'Instance' is used here.
type Instance interface {
	Start(peerConnection *ccintf.PeerConnection) error
	Stop() error
	Wait() (int, error)
}

type UninitializedInstance struct{}

func (UninitializedInstance) Start(peerConnection *ccintf.PeerConnection) error {
	return errors.Errorf("instance has not yet been built, cannot be started")
}

func (UninitializedInstance) Stop() error {
	return errors.Errorf("instance has not yet been built, cannot be stopped")
}

func (UninitializedInstance) Wait() (int, error) {
	return 0, errors.Errorf("instance has not yet been built, cannot wait")
}

//go:generate counterfeiter -o mock/package_provider.go --fake-name PackageProvider . PackageProvider

// PackageProvider gets chaincode packages from the filesystem.
type PackageProvider interface {
	GetChaincodeCodePackage(ccci *ccprovider.ChaincodeContainerInfo) ([]byte, error)
}

type Router struct {
	ExternalVM      VM
	DockerVM        VM
	containers      map[ccintf.CCID]Instance
	PackageProvider PackageProvider
	mutex           sync.Mutex
}

func (r *Router) getInstance(ccid ccintf.CCID) Instance {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	// Note, to resolve the locking problem which existed in the previous code, we never delete
	// references from the map.  In this way, it is safe to release the lock and operate
	// on the returned reference

	if r.containers == nil {
		r.containers = map[ccintf.CCID]Instance{}
	}

	vm, ok := r.containers[ccid]
	if !ok {
		return UninitializedInstance{}
	}

	return vm
}

func (r *Router) Build(ccci *ccprovider.ChaincodeContainerInfo) error {
	codePackage, err := r.PackageProvider.GetChaincodeCodePackage(ccci)
	if err != nil {
		return errors.WithMessage(err, "get chaincode package failed")
	}

	var instance Instance
	var externalErr error
	if r.ExternalVM != nil {
		instance, externalErr = r.ExternalVM.Build(ccci, codePackage)
	}

	if r.ExternalVM == nil || externalErr != nil {
		instance, err = r.DockerVM.Build(ccci, codePackage)
	}

	if err != nil {
		return errors.WithMessagef(err, "failed external (%s) and docker build", externalErr)
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.containers == nil {
		r.containers = map[ccintf.CCID]Instance{}
	}

	r.containers[ccintf.CCID(ccci.PackageID)] = instance

	return nil
}

func (r *Router) Start(ccid ccintf.CCID, peerConnection *ccintf.PeerConnection) error {
	return r.getInstance(ccid).Start(peerConnection)
}

func (r *Router) Stop(ccid ccintf.CCID) error {
	return r.getInstance(ccid).Stop()
}

func (r *Router) Wait(ccid ccintf.CCID) (int, error) {
	return r.getInstance(ccid).Wait()
}
