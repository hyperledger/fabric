/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policy

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/stretchr/testify/require"
)

func TestNewLocalMSPPrincipalGetter(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	require.NotNil(t, NewLocalMSPPrincipalGetter(mgmt.GetLocalMSP(cryptoProvider)))
}

func TestLocalMSPPrincipalGetter_Get(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	localMSP := mgmt.GetLocalMSP(cryptoProvider)
	localMSPID, err := localMSP.GetIdentifier()
	require.NoError(t, err)
	g := NewLocalMSPPrincipalGetter(localMSP)

	_, err = g.Get("")
	require.Error(t, err)

	p, err := g.Get(Admins)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, msp.MSPPrincipal_ROLE, p.PrincipalClassification)
	role := &msp.MSPRole{}
	proto.Unmarshal(p.Principal, role)
	require.Equal(t, localMSPID, role.MspIdentifier)
	require.Equal(t, msp.MSPRole_ADMIN, role.Role)

	p, err = g.Get(Members)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, msp.MSPPrincipal_ROLE, p.PrincipalClassification)
	role = &msp.MSPRole{}
	proto.Unmarshal(p.Principal, role)
	require.Equal(t, localMSPID, role.MspIdentifier)
	require.Equal(t, msp.MSPRole_MEMBER, role.Role)
}
