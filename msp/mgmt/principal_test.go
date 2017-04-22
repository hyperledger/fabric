/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package mgmt

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

func TestNewLocalMSPPrincipalGetter(t *testing.T) {
	assert.NotNil(t, NewLocalMSPPrincipalGetter())
}

func TestLocalMSPPrincipalGetter_Get(t *testing.T) {
	m := NewDeserializersManager()
	g := NewLocalMSPPrincipalGetter()

	_, err := g.Get("")
	assert.Error(t, err)

	p, err := g.Get(Admins)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, msp.MSPPrincipal_ROLE, p.PrincipalClassification)
	role := &msp.MSPRole{}
	proto.Unmarshal(p.Principal, role)
	assert.Equal(t, m.GetLocalMSPIdentifier(), role.MspIdentifier)
	assert.Equal(t, msp.MSPRole_ADMIN, role.Role)

	p, err = g.Get(Members)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, msp.MSPPrincipal_ROLE, p.PrincipalClassification)
	role = &msp.MSPRole{}
	proto.Unmarshal(p.Principal, role)
	assert.Equal(t, m.GetLocalMSPIdentifier(), role.MspIdentifier)
	assert.Equal(t, msp.MSPRole_MEMBER, role.Role)
}
