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

package crypto

import (
	"sync"

	"github.com/hyperledger/fabric/core/crypto/utils"
)

// Private types and variables

type peerEntry struct {
	peer    Peer
	counter int64
}

var (
	// Map of initialized peers
	peers = make(map[string]peerEntry)

	// Sync
	peerMutex sync.Mutex
)

// Public Methods

// RegisterPeer registers a peer to the PKI infrastructure
func RegisterPeer(name string, pwd []byte, enrollID, enrollPWD string) error {
	peerMutex.Lock()
	defer peerMutex.Unlock()

	log.Infof("Registering peer [%s] with id [%s]...", enrollID, name)

	if _, ok := peers[name]; ok {
		log.Infof("Registering peer [%s] with id [%s]...done. Already initialized.", enrollID, name)

		return nil
	}

	peer := newPeer()
	if err := peer.register(NodePeer, name, pwd, enrollID, enrollPWD, nil); err != nil {
		if err != utils.ErrAlreadyRegistered && err != utils.ErrAlreadyInitialized {
			log.Errorf("Failed registering peer [%s] with id [%s] [%s].", enrollID, name, err)
			return err
		}
		log.Infof("Registering peer [%s] with id [%s]...done. Already registered or initiliazed.", enrollID, name)
	}
	err := peer.close()
	if err != nil {
		// It is not necessary to report this error to the caller
		log.Warningf("Registering peer [%s] with id [%s]. Failed closing [%s].", enrollID, name, err)
	}

	log.Infof("Registering peer [%s] with id [%s]...done!", enrollID, name)

	return nil
}

// InitPeer initializes a peer named name with password pwd
func InitPeer(name string, pwd []byte) (Peer, error) {
	peerMutex.Lock()
	defer peerMutex.Unlock()

	log.Infof("Initializing peer [%s]...", name)

	if entry, ok := peers[name]; ok {
		log.Infof("Peer  already initiliazied [%s]. Increasing counter from [%d]", name, peers[name].counter)
		entry.counter++
		peers[name] = entry

		return peers[name].peer, nil
	}

	peer := newPeer()
	if err := peer.init(NodePeer, name, pwd, nil); err != nil {
		log.Errorf("Failed peer initialization [%s]: [%s]", name, err)

		return nil, err
	}

	peers[name] = peerEntry{peer, 1}
	log.Infof("Initializing peer [%s]...done!", name)

	return peer, nil
}

// ClosePeer releases all the resources allocated by peers
func ClosePeer(peer Peer) error {
	peerMutex.Lock()
	defer peerMutex.Unlock()

	return closePeerInternal(peer, false)
}

// CloseAllPeers closes all the peers initialized so far
func CloseAllPeers() (bool, []error) {
	peerMutex.Lock()
	defer peerMutex.Unlock()

	log.Info("Closing all peers...")

	errs := make([]error, len(peers))
	for _, value := range peers {
		err := closePeerInternal(value.peer, true)

		errs = append(errs, err)
	}

	log.Info("Closing all peers...done!")

	return len(errs) != 0, errs
}

// Private Methods

func newPeer() *peerImpl {
	return &peerImpl{&nodeImpl{}, sync.RWMutex{}, nil}
}

func closePeerInternal(peer Peer, force bool) error {
	if peer == nil {
		return utils.ErrNilArgument
	}

	name := peer.GetName()
	log.Infof("Closing peer [%s]...", name)
	entry, ok := peers[name]
	if !ok {
		return utils.ErrInvalidReference
	}
	if entry.counter == 1 || force {
		defer delete(peers, name)
		err := peers[name].peer.(*peerImpl).close()
		log.Infof("Closing peer [%s]...done! [%s].", name, utils.ErrToString(err))

		return err
	}

	// decrease counter
	entry.counter--
	peers[name] = entry
	log.Infof("Closing peer [%s]...decreased counter at [%d].", name, peers[name].counter)

	return nil
}
