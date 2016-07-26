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

// Private type and variables

type validatorEntry struct {
	validator Peer
	counter   int64
}

var (
	// Map of initialized validators
	validators = make(map[string]validatorEntry)

	// Sync
	mutex sync.Mutex
)

// Public Methods

// RegisterValidator registers a validator to the PKI infrastructure
func RegisterValidator(name string, pwd []byte, enrollID, enrollPWD string) error {
	mutex.Lock()
	defer mutex.Unlock()

	log.Infof("Registering validator [%s] with name [%s]...", enrollID, name)

	if _, ok := validators[name]; ok {
		log.Infof("Registering validator [%s] with name [%s]...done. Already initialized.", enrollID, name)

		return nil
	}

	validator := newValidator()
	if err := validator.register(name, pwd, enrollID, enrollPWD, nil); err != nil {
		if err != utils.ErrAlreadyRegistered && err != utils.ErrAlreadyInitialized {
			log.Errorf("Failed registering validator [%s] with name [%s] [%s].", enrollID, name, err)
			return err
		}
		log.Infof("Registering validator [%s] with name [%s]...done. Already registered or initiliazed.", enrollID, name)
	}
	err := validator.close()
	if err != nil {
		// It is not necessary to report this error to the caller
		log.Warningf("Registering validator [%s] with name [%s]. Failed closing [%s].", enrollID, name, err)
	}

	log.Infof("Registering validator [%s] with name [%s]...done!", enrollID, name)

	return nil
}

// InitValidator initializes a validator named name with password pwd
func InitValidator(name string, pwd []byte) (Peer, error) {
	mutex.Lock()
	defer mutex.Unlock()

	log.Infof("Initializing validator [%s]...", name)

	if entry, ok := validators[name]; ok {
		log.Infof("Validator already initiliazied [%s]. Increasing counter from [%d]", name, validators[name].counter)
		entry.counter++
		validators[name] = entry

		return validators[name].validator, nil
	}

	validator := newValidator()
	if err := validator.init(name, pwd, nil); err != nil {
		log.Errorf("Failed validator initialization [%s]: [%s]", name, err)

		return nil, err
	}

	validators[name] = validatorEntry{validator, 1}
	log.Infof("Initializing validator [%s]...done!", name)

	return validator, nil
}

// CloseValidator releases all the resources allocated by the validator
func CloseValidator(peer Peer) error {
	mutex.Lock()
	defer mutex.Unlock()

	return closeValidatorInternal(peer, false)
}

// CloseAllValidators closes all the validators initialized so far
func CloseAllValidators() (bool, []error) {
	mutex.Lock()
	defer mutex.Unlock()

	log.Info("Closing all validators...")

	errs := make([]error, len(validators))
	for _, value := range validators {
		err := closeValidatorInternal(value.validator, true)

		errs = append(errs, err)
	}

	log.Info("Closing all validators...done!")

	return len(errs) != 0, errs
}

// Private Methods

func newValidator() *validatorImpl {
	return &validatorImpl{&peerImpl{&nodeImpl{}, sync.RWMutex{}, nil}, nil}
}

func closeValidatorInternal(peer Peer, force bool) error {
	if peer == nil {
		return utils.ErrNilArgument
	}

	name := peer.GetName()
	log.Infof("Closing validator [%s]...", name)
	entry, ok := validators[name]
	if !ok {
		return utils.ErrInvalidReference
	}
	if entry.counter == 1 || force {
		defer delete(validators, name)
		err := validators[name].validator.(*validatorImpl).close()
		log.Infof("Closing validator [%s]...done! [%s].", name, utils.ErrToString(err))

		return err
	}

	// decrease counter
	entry.counter--
	validators[name] = entry
	log.Infof("Closing validator [%s]...decreased counter at [%d].", name, validators[name].counter)

	return nil
}
