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

package rawledger

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

var closedChan chan struct{}

func init() {
	closedChan = make(chan struct{})
	close(closedChan)
}

// NotFoundErrorIterator simply always returns an error of cb.Status_NOT_FOUND, and is generally useful for implementations of the Reader interface
type NotFoundErrorIterator struct{}

// Next returns nil, cb.Status_NOT_FOUND
func (nfei *NotFoundErrorIterator) Next() (*cb.Block, cb.Status) {
	return nil, cb.Status_NOT_FOUND
}

// ReadyChan returns a closed channel
func (nfei *NotFoundErrorIterator) ReadyChan() <-chan struct{} {
	return closedChan
}
