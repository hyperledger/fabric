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

package msp

import (
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/stretchr/testify/require"
)

func TestInvalidAdminNodeOU(t *testing.T) {
	// testdata/nodeous1:
	// the configuration enables NodeOUs but the administrator does not carry
	// any valid NodeOUS. Therefore MSP initialization must fail
	thisMSP, err := getLocalMSPWithVersionAndError(t, "testdata/nodeous1", MSPv1_1)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)
	require.Error(t, err)

	// MSPv1_0 should not fail
	thisMSP, err = getLocalMSPWithVersionAndError(t, "testdata/nodeous1", MSPv1_0)
	require.False(t, thisMSP.(*bccspmsp).ouEnforcement)
	require.NoError(t, err)
}

func TestInvalidSigningIdentityNodeOU(t *testing.T) {
	t.Run("signing_identity_validation_fails_with_MSPv1_4_3", func(t *testing.T) {
		// testdata/nodeous2:
		// the configuration enables NodeOUs but the signing identity does not carry
		// any valid NodeOUS. Therefore signing identity validation should fail
		thisMSP := getLocalMSPWithVersion(t, "testdata/nodeous2", MSPv1_4_3)
		require.True(t, thisMSP.(*bccspmsp).ouEnforcement)

		id, err := thisMSP.GetDefaultSigningIdentity()
		require.NoError(t, err)

		err = id.Validate()
		require.EqualError(t, err, "could not validate identity's OUs: the identity does not have an OU that resolves to client, peer, orderer, or admin role. OUs: [], MSP: [SampleOrg]")
	})

	t.Run("signing_identity_validation_fails_with_MSPv1_1", func(t *testing.T) {
		// testdata/nodeous2:
		// the configuration enables NodeOUs but the signing identity does not carry
		// any valid NodeOUS. Therefore signing identity validation should fail
		thisMSP := getLocalMSPWithVersion(t, "testdata/nodeous2", MSPv1_1)
		require.True(t, thisMSP.(*bccspmsp).ouEnforcement)

		id, err := thisMSP.GetDefaultSigningIdentity()
		require.NoError(t, err)

		err = id.Validate()
		require.EqualError(t, err, "could not validate identity's OUs: the identity does not have an OU that resolves to client or peer. OUs: [], MSP: [SampleOrg]")
	})

	t.Run("signing_identity_validation_succeeds_with_MSPv1_0", func(t *testing.T) {
		// MSPv1_0 should not fail, node OUs not yet implemented in 1_0
		thisMSP, err := getLocalMSPWithVersionAndError(t, "testdata/nodeous1", MSPv1_0)
		require.False(t, thisMSP.(*bccspmsp).ouEnforcement)
		require.NoError(t, err)

		id, err := thisMSP.GetDefaultSigningIdentity()
		require.NoError(t, err)

		err = id.Validate()
		require.NoError(t, err)
	})
}

func TestValidMSPWithNodeOU(t *testing.T) {
	// testdata/nodeous3:
	// the configuration enables NodeOUs and admin and signing identity are valid
	thisMSP := getLocalMSPWithVersion(t, "testdata/nodeous3", MSPv1_1)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)

	id, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)

	err = id.Validate()
	require.NoError(t, err)

	// MSPv1_0 should not fail as well
	thisMSP = getLocalMSPWithVersion(t, "testdata/nodeous3", MSPv1_0)
	require.False(t, thisMSP.(*bccspmsp).ouEnforcement)

	id, err = thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)

	err = id.Validate()
	require.NoError(t, err)
}

func TestValidMSPWithNodeOUAndOrganizationalUnits(t *testing.T) {
	// testdata/nodeous6:
	// the configuration enables NodeOUs and OrganizationalUnits, and admin and signing identity are valid
	thisMSP := getLocalMSPWithVersion(t, "testdata/nodeous6", MSPv1_1)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)

	id, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)

	err = id.Validate()
	require.NoError(t, err)

	// MSPv1_0 should not fail as well
	thisMSP = getLocalMSPWithVersion(t, "testdata/nodeous6", MSPv1_0)
	require.False(t, thisMSP.(*bccspmsp).ouEnforcement)

	id, err = thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)

	err = id.Validate()
	require.NoError(t, err)
}

func TestInvalidMSPWithNodeOUAndOrganizationalUnits(t *testing.T) {
	// testdata/nodeous6:
	// the configuration enables NodeOUs and OrganizationalUnits,
	// and admin and signing identity are not valid because they don't have
	// OU_common in their OUs.
	thisMSP, err := getLocalMSPWithVersionAndError(t, "testdata/nodeous7", MSPv1_1)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not validate identity's OUs: none of the identity's organizational units")

	// MSPv1_0 should fail as well
	thisMSP, err = getLocalMSPWithVersionAndError(t, "testdata/nodeous7", MSPv1_0)
	require.False(t, thisMSP.(*bccspmsp).ouEnforcement)
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not validate identity's OUs: none of the identity's organizational units")
}

func TestInvalidAdminOU(t *testing.T) {
	// testdata/nodeous4:
	// the configuration enables NodeOUs and admin does not match the certifier chain specified at config
	thisMSP, err := getLocalMSPWithVersionAndError(t, "testdata/nodeous4", MSPv1_1)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)
	require.Error(t, err)
	require.Contains(t, err.Error(), "admin 0 is invalid: The identity is not valid under this MSP [SampleOrg]: could not validate identity's OUs: certifiersIdentifier does not match")

	// MSPv1_0 should not fail as well
	thisMSP, err = getLocalMSPWithVersionAndError(t, "testdata/nodeous4", MSPv1_0)
	require.False(t, thisMSP.(*bccspmsp).ouEnforcement)
	require.NoError(t, err)
}

func TestInvalidAdminOUNotAClient(t *testing.T) {
	// testdata/nodeous4:
	// the configuration enables NodeOUs and admin is not a client
	thisMSP, err := getLocalMSPWithVersionAndError(t, "testdata/nodeous8", MSPv1_1)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)
	require.Error(t, err)
	require.Contains(t, err.Error(), "The identity does not contain OU [CLIENT]")

	// MSPv1_0 should not fail
	thisMSP, err = getLocalMSPWithVersionAndError(t, "testdata/nodeous8", MSPv1_0)
	require.False(t, thisMSP.(*bccspmsp).ouEnforcement)
	require.NoError(t, err)
}

func TestSatisfiesPrincipalPeer(t *testing.T) {
	// testdata/nodeous3:
	// the configuration enables NodeOUs and admin and signing identity are valid
	thisMSP := getLocalMSPWithVersion(t, "testdata/nodeous3", MSPv1_1)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)

	// The default signing identity is a peer
	id, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)

	err = id.Validate()
	require.NoError(t, err)

	require.True(t, t.Run("Check that id is a peer", func(t *testing.T) {
		// Check that id is a peer
		mspID, err := thisMSP.GetIdentifier()
		require.NoError(t, err)
		principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: mspID})
		require.NoError(t, err)
		principal := &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               principalBytes,
		}
		err = id.SatisfiesPrincipal(principal)
		require.NoError(t, err)
	}))

	require.True(t, t.Run("Check that id is not a client", func(t *testing.T) {
		// Check that id is not a client
		mspID, err := thisMSP.GetIdentifier()
		require.NoError(t, err)
		principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: mspID})
		require.NoError(t, err)
		principal := &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               principalBytes,
		}
		err = id.SatisfiesPrincipal(principal)
		require.Error(t, err)
		require.Contains(t, err.Error(), "The identity is not a [CLIENT] under this MSP [SampleOrg]")
	}))
}

func TestSatisfiesPrincipalClient(t *testing.T) {
	// testdata/nodeous3:
	// the configuration enables NodeOUs and admin and signing identity are valid
	thisMSP := getLocalMSPWithVersion(t, "testdata/nodeous3", MSPv1_1)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)

	// The admin of this msp is a client
	require.Equal(t, 1, len(thisMSP.(*bccspmsp).admins))
	id := thisMSP.(*bccspmsp).admins[0]

	err := id.Validate()
	require.NoError(t, err)

	// Check that id is a client
	require.True(t, t.Run("Check that id is a client", func(t *testing.T) {
		mspID, err := thisMSP.GetIdentifier()
		require.NoError(t, err)
		principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: mspID})
		require.NoError(t, err)
		principal := &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               principalBytes,
		}
		err = id.SatisfiesPrincipal(principal)
		require.NoError(t, err)
	}))

	require.True(t, t.Run("Check that id is not a peer", func(t *testing.T) {
		// Check that id is not a peer
		mspID, err := thisMSP.GetIdentifier()
		require.NoError(t, err)
		principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: mspID})
		require.NoError(t, err)
		principal := &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               principalBytes,
		}
		err = id.SatisfiesPrincipal(principal)
		require.Error(t, err)
		require.Contains(t, err.Error(), "The identity is not a [PEER] under this MSP [SampleOrg]")
	}))
}

func TestSatisfiesPrincipalAdmin(t *testing.T) {
	// testdata/nodeouadmin:
	// the configuration enables NodeOUs (with adminOU) and admin and signing identity are valid
	thisMSP := getLocalMSPWithVersion(t, "testdata/nodeouadmin", MSPv1_4_3)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)

	cert, err := readFile("testdata/nodeouadmin/adm/testadmincert.pem")
	require.NoError(t, err)

	id, _, err := thisMSP.(*bccspmsp).getIdentityFromConf(cert)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: "SampleOrg"})
	require.NoError(t, err)
	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}
	err = id.SatisfiesPrincipal(principal)
	require.NoError(t, err)
}

func TestLoad142MSPWithInvalidAdminConfiguration(t *testing.T) {
	// testdata/nodeouadmin2:
	// the configuration enables NodeOUs (with adminOU) but no valid identifier for the AdminOU
	conf, err := GetLocalMspConfig("testdata/nodeouadmin2", nil, "SampleOrg")
	require.NoError(t, err)

	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join("testdata/nodeouadmin2", "keystore"), true)
	require.NoError(t, err)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	thisMSP, err := NewBccspMspWithKeyStore(MSPv1_4_3, ks, cryptoProvider)
	require.NoError(t, err)

	err = thisMSP.Setup(conf)
	require.Error(t, err)
	require.Equal(t, "administrators must be declared when no admin ou classification is set", err.Error())

	// testdata/nodeouadmin3:
	// the configuration enables NodeOUs (with adminOU) but no valid identifier for the AdminOU
	conf, err = GetLocalMspConfig("testdata/nodeouadmin3", nil, "SampleOrg")
	require.NoError(t, err)

	ks, err = sw.NewFileBasedKeyStore(nil, filepath.Join("testdata/nodeouadmin3", "keystore"), true)
	require.NoError(t, err)
	thisMSP, err = NewBccspMspWithKeyStore(MSPv1_4_3, ks, cryptoProvider)
	require.NoError(t, err)

	err = thisMSP.Setup(conf)
	require.Error(t, err)
	require.Equal(t, "administrators must be declared when no admin ou classification is set", err.Error())
}

func TestAdminInAdmincertsWith143MSP(t *testing.T) {
	// testdata/nodeouadminclient enables NodeOU classification and contains in the admincerts folder
	// a certificate classified as client. This test checks that identity is considered an admin anyway.
	// testdata/nodeouadminclient2 enables NodeOU classification and contains in the admincerts folder
	// a certificate classified as client. This test checks that identity is considered an admin anyway.
	// Notice that the configuration used is one that is usually expected for MSP version < 1.4.3 which
	// only define peer and client OU.
	testFolders := []string{"testdata/nodeouadminclient", "testdata/nodeouadminclient2"}

	for _, testFolder := range testFolders {
		localMSP := getLocalMSPWithVersion(t, testFolder, MSPv1_4_3)

		cert, err := readFile(filepath.Join(testFolder, "admincerts", "admin.pem"))
		require.NoError(t, err)

		id, _, err := localMSP.(*bccspmsp).getIdentityFromConf(cert)
		require.NoError(t, err)
		for _, ou := range id.GetOrganizationalUnits() {
			require.NotEqual(t, "admin", ou.OrganizationalUnitIdentifier)
		}

		principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: "SampleOrg"})
		require.NoError(t, err)
		principal := &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               principalBytes,
		}
		err = id.SatisfiesPrincipal(principal)
		require.NoError(t, err)
	}
}

func TestSatisfiesPrincipalOrderer(t *testing.T) {
	// testdata/nodeouorderer:
	// the configuration enables NodeOUs (with orderOU)
	thisMSP := getLocalMSPWithVersion(t, "testdata/nodeouorderer", MSPv1_4_3)
	require.True(t, thisMSP.(*bccspmsp).ouEnforcement)

	id, err := thisMSP.(*bccspmsp).GetDefaultSigningIdentity()
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ORDERER, MspIdentifier: "SampleOrg"})
	require.NoError(t, err)
	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}
	err = id.SatisfiesPrincipal(principal)
	require.NoError(t, err)
}

func TestLoad142MSPWithInvalidOrdererConfiguration(t *testing.T) {
	// testdata/nodeouorderer2:
	// the configuration enables NodeOUs (with orderOU) but no valid identifier for the OrdererOU
	conf, err := GetLocalMspConfig("testdata/nodeouorderer2", nil, "SampleOrg")
	require.NoError(t, err)

	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join("testdata/nodeouorderer2", "keystore"), true)
	require.NoError(t, err)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	thisMSP, err := NewBccspMspWithKeyStore(MSPv1_4_3, ks, cryptoProvider)
	require.NoError(t, err)

	err = thisMSP.Setup(conf)
	require.NoError(t, err)
	id, err := thisMSP.(*bccspmsp).GetDefaultSigningIdentity()
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ORDERER, MspIdentifier: "SampleOrg"})
	require.NoError(t, err)
	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}
	err = id.SatisfiesPrincipal(principal)
	require.Error(t, err)
	require.Equal(t, "The identity is not a [ORDERER] under this MSP [SampleOrg]: cannot test for classification, node ou for type [ORDERER], not defined, msp: [SampleOrg]", err.Error())

	// testdata/nodeouorderer3:
	// the configuration enables NodeOUs (with orderOU) but no valid identifier for the OrdererOU
	conf, err = GetLocalMspConfig("testdata/nodeouorderer3", nil, "SampleOrg")
	require.NoError(t, err)

	ks, err = sw.NewFileBasedKeyStore(nil, filepath.Join("testdata/nodeouorderer3", "keystore"), true)
	require.NoError(t, err)
	thisMSP, err = NewBccspMspWithKeyStore(MSPv1_4_3, ks, cryptoProvider)
	require.NoError(t, err)

	err = thisMSP.Setup(conf)
	require.NoError(t, err)
	id, err = thisMSP.(*bccspmsp).GetDefaultSigningIdentity()
	require.NoError(t, err)

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ORDERER, MspIdentifier: "SampleOrg"})
	require.NoError(t, err)
	principal = &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}
	err = id.SatisfiesPrincipal(principal)
	require.Error(t, err)
	require.Equal(t, "The identity is not a [ORDERER] under this MSP [SampleOrg]: cannot test for classification, node ou for type [ORDERER], not defined, msp: [SampleOrg]", err.Error())
}

func TestValidMSPWithNodeOUMissingClassification(t *testing.T) {
	// testdata/nodeousbadconf1:
	// the configuration enables NodeOUs but client ou identifier is missing
	_, err := getLocalMSPWithVersionAndError(t, "testdata/nodeousbadconf1", MSPv1_3)
	require.Error(t, err)
	require.Equal(t, "Failed setting up NodeOUs. ClientOU must be different from nil.", err.Error())

	_, err = getLocalMSPWithVersionAndError(t, "testdata/nodeousbadconf1", MSPv1_4_3)
	require.Error(t, err)
	require.Equal(t, "admin 0 is invalid [cannot test for classification, node ou for type [CLIENT], not defined, msp: [SampleOrg],The identity does not contain OU [ADMIN], MSP: [SampleOrg]]", err.Error())

	// testdata/nodeousbadconf2:
	// the configuration enables NodeOUs but peer ou identifier is missing
	_, err = getLocalMSPWithVersionAndError(t, "testdata/nodeousbadconf2", MSPv1_3)
	require.Error(t, err)
	require.Equal(t, "Failed setting up NodeOUs. PeerOU must be different from nil.", err.Error())

	_, err = getLocalMSPWithVersionAndError(t, "testdata/nodeousbadconf2", MSPv1_4_3)
	require.NoError(t, err)
}
