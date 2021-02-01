/*
Copyright State Street Corp. 2018 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/stretchr/testify/require"
)

const (
	sampleAPI1Name      = "Foo"
	sampleAPI1PolicyRef = "foo"

	sampleAPI2Name      = "Bar"
	sampleAPI2PolicyRef = "/Channel/foo"
)

var sampleAPIsProvider = map[string]*pb.APIResource{
	sampleAPI1Name: {PolicyRef: sampleAPI1PolicyRef},
	sampleAPI2Name: {PolicyRef: sampleAPI2PolicyRef},
}

func TestGreenAPIsPath(t *testing.T) {
	ag := newAPIsProvider(sampleAPIsProvider)
	require.NotNil(t, ag)

	t.Run("PresentAPIs", func(t *testing.T) {
		require.Equal(t, "/Channel/Application/"+sampleAPI1PolicyRef, ag.PolicyRefForAPI(sampleAPI1Name))
		require.Equal(t, sampleAPI2PolicyRef, ag.PolicyRefForAPI(sampleAPI2Name))
	})

	t.Run("MissingAPIs", func(t *testing.T) {
		require.Empty(t, ag.PolicyRefForAPI("missing"))
	})
}

func TestNilACLs(t *testing.T) {
	ccg := newAPIsProvider(nil)

	require.NotNil(t, ccg)
	require.NotNil(t, ccg.aclPolicyRefs)
	require.Empty(t, ccg.aclPolicyRefs)
}

func TestEmptyACLs(t *testing.T) {
	ccg := newAPIsProvider(map[string]*pb.APIResource{})

	require.NotNil(t, ccg)
	require.NotNil(t, ccg.aclPolicyRefs)
	require.Empty(t, ccg.aclPolicyRefs)
}

func TestEmptyPolicyRef(t *testing.T) {
	ars := map[string]*pb.APIResource{
		"unsetAPI": {PolicyRef: ""},
	}

	ccg := newAPIsProvider(ars)

	require.NotNil(t, ccg)
	require.NotNil(t, ccg.aclPolicyRefs)
	require.Empty(t, ccg.aclPolicyRefs)

	ars = map[string]*pb.APIResource{
		"unsetAPI": {PolicyRef: ""},
		"setAPI":   {PolicyRef: sampleAPI2PolicyRef},
	}

	ccg = newAPIsProvider(ars)

	require.NotNil(t, ccg)
	require.NotNil(t, ccg.aclPolicyRefs)
	require.NotEmpty(t, ccg.aclPolicyRefs)
	require.NotContains(t, ccg.aclPolicyRefs, sampleAPI1Name)
}
