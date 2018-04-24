/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
)

// runningChaincodes contains maps of chaincodeIDs to their chaincodeRTEs
type runningChaincodes struct {
	sync.RWMutex
	// chaincode environment for each chaincode
	chaincodeMap map[string]*chaincodeRTEnv

	//mark the starting of launch of a chaincode so multiple requests
	//do not attempt to start the chaincode at the same time
	launchStarted map[string]bool
}

func (r *runningChaincodes) Contains(key string) bool {
	r.Lock()
	defer r.Unlock()

	return r.contains(key)
}

func (r *runningChaincodes) contains(key string) bool {
	if _, ok := r.chaincodeMap[key]; ok {
		return true
	}
	if _, ok := r.launchStarted[key]; ok {
		return true
	}
	return false
}

func (r *runningChaincodes) SetLaunchStarted(key string) error {
	r.Lock()
	defer r.Unlock()

	if r.contains(key) {
		return fmt.Errorf("key already present: %s", key)
	}

	r.launchStarted[key] = true
	return nil
}

func (r *runningChaincodes) GetChaincode(key string) (*chaincodeRTEnv, bool) {
	r.Lock()
	defer r.Unlock()

	rtenv, ok := r.chaincodeMap[key]
	return rtenv, ok
}

func (r *runningChaincodes) RemoveChaincode(key string) error {
	r.Lock()
	defer r.Unlock()

	_, ok := r.chaincodeMap[key]
	if !ok {
		return errors.Errorf("could not find handler with key: %s", key)
	}

	delete(r.chaincodeMap, key)
	return nil
}

func (r *runningChaincodes) GetLaunchStarted(key string) bool {
	r.Lock()
	defer r.Unlock()

	_, ok := r.launchStarted[key]
	return ok
}

func (r *runningChaincodes) RemoveLaunchStarted(key string) error {
	r.Lock()
	defer r.Unlock()

	_, ok := r.launchStarted[key]
	if !ok {
		return errors.Errorf("could not find handler with key: %s", key)
	}

	delete(r.launchStarted, key)
	return nil
}

func (r *runningChaincodes) RegisterChaincode(key string, chrte *chaincodeRTEnv) error {
	r.Lock()
	defer r.Unlock()

	if r.chaincodeMap[key] != nil {
		return fmt.Errorf("chaincode handler already exists: %s", key)
	}
	r.chaincodeMap[key] = chrte
	return nil
}
