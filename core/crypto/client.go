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

// Private Variables

type clientEntry struct {
	client  Client
	counter int64
}

var (
	// Map of initialized clients
	clients = make(map[string]clientEntry)

	// Sync
	clientMutex sync.Mutex
)

// Public Methods

// RegisterClient registers a client to the PKI infrastructure
func RegisterClient(name string, pwd []byte, enrollID, enrollPWD string) error {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	log.Infof("Registering client [%s] with name [%s]...", enrollID, name)

	if _, ok := clients[name]; ok {
		log.Infof("Registering client [%s] with name [%s]...done. Already initialized.", enrollID, name)

		return nil
	}

	client := newClient()
	if err := client.register(name, pwd, enrollID, enrollPWD); err != nil {
		if err != utils.ErrAlreadyRegistered && err != utils.ErrAlreadyInitialized {
			log.Errorf("Failed registering client [%s] with name [%s] [%s].", enrollID, name, err)
			return err
		}
		log.Infof("Registering client [%s] with name [%s]...done. Already registered or initiliazed.", enrollID, name)
	}
	err := client.close()
	if err != nil {
		// It is not necessary to report this error to the caller
		log.Warningf("Registering client [%s] with name [%s]. Failed closing [%s].", enrollID, name, err)
	}

	log.Infof("Registering client [%s] with name [%s]...done!", enrollID, name)

	return nil
}

// InitClient initializes a client named name with password pwd
func InitClient(name string, pwd []byte) (Client, error) {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	log.Infof("Initializing client [%s]...", name)

	if entry, ok := clients[name]; ok {
		log.Infof("Client already initiliazied [%s]. Increasing counter from [%d]", name, clients[name].counter)
		entry.counter++
		clients[name] = entry

		return clients[name].client, nil
	}

	client := newClient()
	if err := client.init(name, pwd); err != nil {
		log.Errorf("Failed client initialization [%s]: [%s].", name, err)

		return nil, err
	}

	clients[name] = clientEntry{client, 1}
	log.Infof("Initializing client [%s]...done!", name)

	return client, nil
}

// CloseClient releases all the resources allocated by clients
func CloseClient(client Client) error {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	return closeClientInternal(client, false)
}

// CloseAllClients closes all the clients initialized so far
func CloseAllClients() (bool, []error) {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	log.Info("Closing all clients...")

	errs := make([]error, len(clients))
	for _, value := range clients {
		err := closeClientInternal(value.client, true)

		errs = append(errs, err)
	}

	log.Info("Closing all clients...done!")

	return len(errs) != 0, errs
}

// Private Methods

func newClient() *clientImpl {
	return &clientImpl{&nodeImpl{}, nil, nil, nil, nil}
}

func closeClientInternal(client Client, force bool) error {
	if client == nil {
		return utils.ErrNilArgument
	}

	name := client.GetName()
	log.Infof("Closing client [%s]...", name)
	entry, ok := clients[name]
	if !ok {
		return utils.ErrInvalidReference
	}
	if entry.counter == 1 || force {
		defer delete(clients, name)
		err := clients[name].client.(*clientImpl).close()
		log.Debugf("Closing client [%s]...cleanup! [%s].", name, utils.ErrToString(err))

		return err
	}

	// decrease counter
	entry.counter--
	clients[name] = entry
	log.Debugf("Closing client [%s]...decreased counter at [%d].", name, clients[name].counter)

	return nil
}
