/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
)

type VMProvider interface {
	NewVM() VM
}

type Builder interface {
	Build() (io.Reader, error)
}

//VM is an abstract virtual image for supporting arbitrary virtual machines
type VM interface {
	Start(ccid ccintf.CCID, args []string, env []string, filesToUpload map[string][]byte, builder Builder) error
	Stop(ccid ccintf.CCID, timeout uint, dontkill bool, dontremove bool) error
	Wait(ccid ccintf.CCID) (int, error)
	HealthCheck(context.Context) error
}

type refCountedLock struct {
	refCount int
	lock     *sync.RWMutex
}

//VMController - manages VMs
//   . abstract construction of different types of VMs (we only care about Docker for now)
//   . manage lifecycle of VM (start with build, start, stop ...
//     eventually probably need fine grained management)
type VMController struct {
	sync.RWMutex
	containerLocks map[string]*refCountedLock
	vmProviders    map[string]VMProvider
}

var vmLogger = flogging.MustGetLogger("container")

// NewVMController creates a new instance of VMController
func NewVMController(vmProviders map[string]VMProvider) *VMController {
	return &VMController{
		containerLocks: make(map[string]*refCountedLock),
		vmProviders:    vmProviders,
	}
}

func (vmc *VMController) newVM(typ string) VM {
	v, ok := vmc.vmProviders[typ]
	if !ok {
		vmLogger.Panicf("Programming error: unsupported VM type: %s", typ)
	}
	return v.NewVM()
}

func (vmc *VMController) lockContainer(id string) {
	//get the container lock under global lock
	vmc.Lock()
	var refLck *refCountedLock
	var ok bool
	if refLck, ok = vmc.containerLocks[id]; !ok {
		refLck = &refCountedLock{refCount: 1, lock: &sync.RWMutex{}}
		vmc.containerLocks[id] = refLck
	} else {
		refLck.refCount++
		vmLogger.Debugf("refcount %d (%s)", refLck.refCount, id)
	}
	vmc.Unlock()
	vmLogger.Debugf("waiting for container(%s) lock", id)
	refLck.lock.Lock()
	vmLogger.Debugf("got container (%s) lock", id)
}

func (vmc *VMController) unlockContainer(id string) {
	vmc.Lock()
	if refLck, ok := vmc.containerLocks[id]; ok {
		if refLck.refCount <= 0 {
			panic("refcnt <= 0")
		}
		refLck.lock.Unlock()
		if refLck.refCount--; refLck.refCount == 0 {
			vmLogger.Debugf("container lock deleted(%s)", id)
			delete(vmc.containerLocks, id)
		}
	} else {
		vmLogger.Debugf("no lock to unlock(%s)!!", id)
	}
	vmc.Unlock()
}

//VMCReq - all requests should implement this interface.
//The context should be passed and tested at each layer till we stop
//note that we'd stop on the first method on the stack that does not
//take context
type VMCReq interface {
	Do(v VM) error
	GetCCID() ccintf.CCID
}

//StartContainerReq - properties for starting a container.
type StartContainerReq struct {
	ccintf.CCID
	Builder       Builder
	Args          []string
	Env           []string
	FilesToUpload map[string][]byte
}

// PlatformBuilder implements the Build interface using
// the platforms package GenerateDockerBuild function.
// XXX This is a pretty awkward spot for the builder, it should
// really probably be pushed into the dockercontroller, as it only
// builds docker images, but, doing so would require contaminating
// the dockercontroller package with the CDS, which is also
// undesirable.
type PlatformBuilder struct {
	Type             string
	Path             string
	Name             string
	Version          string
	CodePackage      []byte
	PlatformRegistry *platforms.Registry
}

// Build a tar stream based on the CDS
func (b *PlatformBuilder) Build() (io.Reader, error) {
	return b.PlatformRegistry.GenerateDockerBuild(
		b.Type,
		b.Path,
		b.Name,
		b.Version,
		b.CodePackage,
	)
}

func (si StartContainerReq) Do(v VM) error {
	return v.Start(si.CCID, si.Args, si.Env, si.FilesToUpload, si.Builder)
}

func (si StartContainerReq) GetCCID() ccintf.CCID {
	return si.CCID
}

//StopContainerReq - properties for stopping a container.
type StopContainerReq struct {
	ccintf.CCID
	Timeout uint
	//by default we will kill the container after stopping
	Dontkill bool
	//by default we will remove the container after killing
	Dontremove bool
}

func (si StopContainerReq) Do(v VM) error {
	return v.Stop(si.CCID, si.Timeout, si.Dontkill, si.Dontremove)
}

func (si StopContainerReq) GetCCID() ccintf.CCID {
	return si.CCID
}

//go:generate counterfeiter -o mock/exitedfunc.go --fake-name ExitedFunc ExitedFunc

// ExitedFunc is the prototype for the function called when a container exits.
type ExitedFunc func(exitCode int, err error)

// WaitContainerReq provides the chaincode ID of the container to wait on and a
// callback to call upon chaincode termination.
type WaitContainerReq struct {
	CCID   ccintf.CCID
	Exited ExitedFunc
}

func (w WaitContainerReq) Do(v VM) error {
	exited := w.Exited
	go func() {
		exitCode, err := v.Wait(w.CCID)
		exited(exitCode, err)
	}()
	return nil
}

func (w WaitContainerReq) GetCCID() ccintf.CCID {
	return w.CCID
}

func (vmc *VMController) Process(vmtype string, req VMCReq) error {
	v := vmc.newVM(vmtype)
	ccid := req.GetCCID()
	id := ccid.GetName()

	vmc.lockContainer(id)
	defer vmc.unlockContainer(id)
	return req.Do(v)
}

// GetChaincodePackageBytes creates bytes for docker container generation using the supplied chaincode specification
func GetChaincodePackageBytes(pr *platforms.Registry, spec *pb.ChaincodeSpec) ([]byte, error) {
	if spec == nil || spec.ChaincodeId == nil {
		return nil, fmt.Errorf("invalid chaincode spec")
	}

	return pr.GetDeploymentPayload(spec.CCType(), spec.Path())
}
