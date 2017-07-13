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

package shadow

import (
	"bytes"
	"fmt"
	"sync"
)

// NewKeyPerInvoke is the shadow implementation for  NewKeyPerInvoke in the parent package
// The shadow provides invoke arguments that are guaranteed to be result in unique ledger
// entries as long as the parameters to GetInvokeArgs are unique
type NewKeyPerInvoke struct {
	sync.Mutex
	state map[string][]byte
}

//---------- implements ShadowCCIntf functions -------

//InitShadowCC initializes CC
func (t *NewKeyPerInvoke) InitShadowCC(initArgs []string) {
	t.state = make(map[string][]byte)
}

//invokeSuccessful sets the state and increments succefull invokes counter
func (t *NewKeyPerInvoke) invokeSuccessful(key []byte, val []byte) {
	t.Lock()
	defer t.Unlock()
	t.state[string(key)] = val
}

//getState gets the state
func (t *NewKeyPerInvoke) getState(key []byte) ([]byte, bool) {
	t.Lock()
	defer t.Unlock()
	v, ok := t.state[string(key)]
	return v, ok
}

//OverrideNumInvokes returns the number of invokes shadow wants
//accept users request, no override
func (t *NewKeyPerInvoke) OverrideNumInvokes(numInvokesPlanned int) int {
	return numInvokesPlanned
}

//GetNumQueries returns the number of queries shadow wants ccchecked to do.
//For our purpose, just do as many queries as there were invokes for.
func (t *NewKeyPerInvoke) GetNumQueries(numInvokesCompletedSuccessfully int) int {
	return numInvokesCompletedSuccessfully
}

//GetInvokeArgs get args for invoke based on chaincode ID and iteration num
func (t *NewKeyPerInvoke) GetInvokeArgs(ccnum int, iter int) [][]byte {
	args := make([][]byte, 3)
	args[0] = []byte("put")
	args[1] = []byte(fmt.Sprintf("%d_%d", ccnum, iter))
	args[2] = []byte(fmt.Sprintf("%d", ccnum))

	return args
}

//PostInvoke store the key/val for later verification
func (t *NewKeyPerInvoke) PostInvoke(args [][]byte, resp []byte) error {
	if len(args) < 3 {
		return fmt.Errorf("invalid number of args posted %d", len(args))
	}

	if string(args[0]) != "put" {
		return fmt.Errorf("invalid args posted %s", args[0])
	}

	//the actual CC should have returned OK for success
	if string(resp) != "OK" {
		return fmt.Errorf("invalid response %s", string(resp))
	}

	t.invokeSuccessful(args[1], args[2])

	return nil
}

//Validate the key/val with mem storage
func (t *NewKeyPerInvoke) Validate(args [][]byte, value []byte) error {
	if len(args) < 2 {
		return fmt.Errorf("invalid number of args for validate %d", len(args))
	}

	if string(args[0]) != "get" {
		return fmt.Errorf("invalid validate function %s", args[0])
	}

	if v, ok := t.getState(args[1]); !ok {
		return fmt.Errorf("key not found %s", args[1])
	} else if !bytes.Equal(v, value) {
		return fmt.Errorf("expected(%s) but found (%s)", string(v), string(value))
	}

	return nil
}

//GetQueryArgs returns the query for the iter to test against
func (t *NewKeyPerInvoke) GetQueryArgs(ccnum int, iter int) [][]byte {
	args := make([][]byte, 2)
	args[0] = []byte("get")
	args[1] = []byte(fmt.Sprintf("%d_%d", ccnum, iter))
	return args
}
