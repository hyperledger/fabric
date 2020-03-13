/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderers

func (cs *ConnectionSource) Endpoints() []*Endpoint {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	return cs.allEndpoints
}
