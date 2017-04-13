/*
Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package backend

import (
	s "github.com/hyperledger/fabric/orderer/sbft/simplebft"
)

type Timer struct {
	tf      func()
	execute bool
}

func (t *Timer) Cancel() {
	t.execute = false
}

type Executable interface {
	Execute(*Backend)
}

func (t *Timer) Execute(backend *Backend) {
	if t.execute {
		t.tf()
	}
}

type msgEvent struct {
	chainId string
	msg     *s.Msg
	src     uint64
}

func (m *msgEvent) Execute(backend *Backend) {
	backend.consensus[m.chainId].Receive(m.msg, m.src)
}

type requestEvent struct {
	chainId string
	req     []byte
}

func (r *requestEvent) Execute(backend *Backend) {
	backend.consensus[r.chainId].Request(r.req)
}

type connectionEvent struct {
	chainID string
	peerid  uint64
}

func (c *connectionEvent) Execute(backend *Backend) {
	backend.consensus[c.chainID].Connection(c.peerid)
}
