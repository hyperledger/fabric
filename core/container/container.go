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
)

var vmLogger = flogging.MustGetLogger("container")

//VM is an abstract virtual image for supporting arbitrary virual machines
type VM interface {
	Build(ccci *ccprovider.ChaincodeContainerInfo, codePackage []byte) error
	Start(ccid ccintf.CCID, ccType string, tlsConfig *ccintf.TLSConfig) error
	Stop(ccid ccintf.CCID) error
	Wait(ccid ccintf.CCID) (int, error)
}

type Router struct {
	DockerVM   VM
	containers map[ccintf.CCID]VM
	mutex      sync.Mutex
}

func (r *Router) getVM(ccid ccintf.CCID) VM {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	// Note, to resolve the locking problem which existed in the previous code, we never delete
	// references from the map.  In this way, it is safe to release the lock and operate
	// on the returned reference

	if r.containers == nil {
		r.containers = map[ccintf.CCID]VM{}
	}

	vm, ok := r.containers[ccid]
	if !ok {
		// TODO the detect logic for the chaincode launcher injects here
		vm = r.DockerVM
		r.containers[ccid] = vm
	}

	return vm
}

func (r *Router) Build(ccci *ccprovider.ChaincodeContainerInfo, codePackage []byte) error {
	return r.getVM(ccintf.New(ccci.PackageID)).Build(ccci, codePackage)
}

func (r *Router) Start(ccid ccintf.CCID, ccType string, tlsConfig *ccintf.TLSConfig) error {
	return r.getVM(ccid).Start(ccid, ccType, tlsConfig)
}

func (r *Router) Stop(ccid ccintf.CCID) error {
	return r.getVM(ccid).Stop(ccid)
}

func (r *Router) Wait(ccid ccintf.CCID) (int, error) {
	return r.getVM(ccid).Wait(ccid)
}
