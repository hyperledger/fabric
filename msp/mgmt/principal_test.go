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
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/stretchr/testify/require"
)

func TestNewLocalMSPPrincipalGetter(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	require.NotNil(t, NewLocalMSPPrincipalGetter(cryptoProvider))
}

func TestLocalMSPPrincipalGetter_Get(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	m := NewDeserializersManager(cryptoProvider)
	g := NewLocalMSPPrincipalGetter(cryptoProvider)

	_, err = g.Get("")
	require.Error(t, err)

	p, err := g.Get(Admins)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, msp.MSPPrincipal_ROLE, p.PrincipalClassification)
	role := &msp.MSPRole{}
	proto.Unmarshal(p.Principal, role)
	require.Equal(t, m.GetLocalMSPIdentifier(), role.MspIdentifier)
	require.Equal(t, msp.MSPRole_ADMIN, role.Role)

	p, err = g.Get(Members)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, msp.MSPPrincipal_ROLE, p.PrincipalClassification)
	role = &msp.MSPRole{}
	proto.Unmarshal(p.Principal, role)
	require.Equal(t, m.GetLocalMSPIdentifier(), role.MspIdentifier)
	require.Equal(t, msp.MSPRole_MEMBER, role.Role)
}
