/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container

import (
	"io"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container/ccintf"

	"github.com/pkg/errors"
)

var vmLogger = flogging.MustGetLogger("container")

//go:generate counterfeiter -o mock/docker_builder.go --fake-name DockerBuilder . DockerBuilder

// DockerBuilder is what is exposed by the dockercontroller
type DockerBuilder interface {
	Build(ccid string, metadata *persistence.ChaincodePackageMetadata, codePackageStream io.Reader) (Instance, error)
}

//go:generate counterfeiter -o mock/external_builder.go --fake-name ExternalBuilder . ExternalBuilder

// ExternalBuilder is what is exposed by the dockercontroller
type ExternalBuilder interface {
	Build(ccid string, metadata []byte, codePackageStream io.Reader) (Instance, error)
}

//go:generate counterfeiter -o mock/instance.go --fake-name Instance . Instance

// Instance represents a built chaincode instance, because of the docker legacy, calling this a
// built 'container' would be very misleading, and going forward with the external launcher
// 'image' also seemed inappropriate.  So, the vague 'Instance' is used here.
type Instance interface {
	Start(peerConnection *ccintf.PeerConnection) error
	ChaincodeServerInfo() (*ccintf.ChaincodeServerInfo, error)
	Stop() error
	Wait() (int, error)
}

type UninitializedInstance struct{}

func (UninitializedInstance) Start(peerConnection *ccintf.PeerConnection) error {
	return errors.Errorf("instance has not yet been built, cannot be started")
}

func (UninitializedInstance) ChaincodeServerInfo() (*ccintf.ChaincodeServerInfo, error) {
	return nil, errors.Errorf("instance has not yet been built, cannot get chaincode server info")
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
	GetChaincodePackage(packageID string) (md *persistence.ChaincodePackageMetadata, mdBytes []byte, codeStream io.ReadCloser, err error)
}

type Router struct {
	ExternalBuilder ExternalBuilder
	DockerBuilder   DockerBuilder
	containers      map[string]Instance
	PackageProvider PackageProvider
	mutex           sync.Mutex
}

func (r *Router) getInstance(ccid string) Instance {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Note, to resolve the locking problem which existed in the previous code, we never delete
	// references from the map.  In this way, it is safe to release the lock and operate
	// on the returned reference
	vm, ok := r.containers[ccid]
	if !ok {
		return UninitializedInstance{}
	}

	return vm
}

func (r *Router) Build(ccid string) error {
	var instance Instance

	if r.ExternalBuilder != nil {
		// for now, the package ID we retrieve from the FS is always the ccid
		// the chaincode uses for registration
		_, mdBytes, codeStream, err := r.PackageProvider.GetChaincodePackage(ccid)
		if err != nil {
			return errors.WithMessage(err, "failed to get chaincode package for external build")
		}
		defer codeStream.Close()

		instance, err = r.ExternalBuilder.Build(ccid, mdBytes, codeStream)
		if err != nil {
			return errors.WithMessage(err, "external builder failed")
		}
	}

	if instance == nil {
		if r.DockerBuilder == nil {
			return errors.New("no DockerBuilder, cannot build")
		}
		metadata, _, codeStream, err := r.PackageProvider.GetChaincodePackage(ccid)
		if err != nil {
			return errors.WithMessage(err, "failed to get chaincode package for docker build")
		}
		defer codeStream.Close()

		instance, err = r.DockerBuilder.Build(ccid, metadata, codeStream)
		if err != nil {
			return errors.WithMessage(err, "docker build failed")
		}
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.containers == nil {
		r.containers = map[string]Instance{}
	}
	r.containers[ccid] = instance

	return nil
}

func (r *Router) ChaincodeServerInfo(ccid string) (*ccintf.ChaincodeServerInfo, error) {
	return r.getInstance(ccid).ChaincodeServerInfo()
}

func (r *Router) Start(ccid string, peerConnection *ccintf.PeerConnection) error {
	return r.getInstance(ccid).Start(peerConnection)
}

func (r *Router) Stop(ccid string) error {
	return r.getInstance(ccid).Stop()
}

func (r *Router) Wait(ccid string) (int, error) {
	return r.getInstance(ccid).Wait()
}

func (r *Router) Shutdown(timeout time.Duration) {
	var wg sync.WaitGroup
	for ccid := range r.containers {
		wg.Add(1)
		go func(ccid string) {
			defer wg.Done()
			if err := r.Stop(ccid); err != nil {
				vmLogger.Warnw("failed to stop chaincode", "ccid", ccid, "error", err)
			}
		}(ccid)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-time.After(timeout):
		vmLogger.Warning("timeout while stopping external chaincodes")
	case <-done:
	}
}
