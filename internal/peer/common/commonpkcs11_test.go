//go:build pkcs11
// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"os"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestBCCSPPKCS11EnvVars(t *testing.T) {
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	os.Setenv("CORE_PEER_BCCSP_DEFAULT", "PKCS11")
	os.Setenv("CORE_PEER_BCCSP_PKCS11_HASH", "SHA2")
	os.Setenv("CORE_PEER_BCCSP_PKCS11_SECURITY", "384")
	os.Setenv("CORE_PEER_BCCSP_PKCS11_LIBRARY", "lib.so")
	os.Setenv("CORE_PEER_BCCSP_PKCS11_LABEL", "token")
	os.Setenv("CORE_PEER_BCCSP_PKCS11_PIN", "password")
	os.Setenv("CORE_PEER_BCCSP_PKCS11_SOFTWAREVERIFY", "1")
	os.Setenv("CORE_PEER_BCCSP_PKCS11_IMMUTABLE", "true")
	os.Setenv("CORE_PEER_BCCSP_PKCS11_ALTID", "01234567890abcdef")
	os.Setenv("CORE_PEER_BCCSP_PKCS11", "{\"keyids\": [{\"ski\": \"1\", \"id\": \"2\"}, {\"ski\": \"3\", \"id\": \"4\"}]}")
	bccspConfig := factory.GetDefaultOpts()
	err := common.InitBCCSPConfig(bccspConfig)
	require.NoError(t, err)
	require.Equal(t, "PKCS11", bccspConfig.Default)
	require.Equal(t, "SHA2", bccspConfig.PKCS11.Hash)
	require.Equal(t, 384, bccspConfig.PKCS11.Security)
	require.Equal(t, "lib.so", bccspConfig.PKCS11.Library)
	require.Equal(t, "token", bccspConfig.PKCS11.Label)
	require.Equal(t, "password", bccspConfig.PKCS11.Pin)
	require.Equal(t, true, bccspConfig.PKCS11.SoftwareVerify)
	require.Equal(t, true, bccspConfig.PKCS11.Immutable)
	require.Equal(t, "01234567890abcdef", bccspConfig.PKCS11.AltID)
	require.Equal(t, 2, len(bccspConfig.PKCS11.KeyIDs))
	require.Equal(t, "1", bccspConfig.PKCS11.KeyIDs[0].SKI)
	require.Equal(t, "2", bccspConfig.PKCS11.KeyIDs[0].ID)
	require.Equal(t, "3", bccspConfig.PKCS11.KeyIDs[1].SKI)
	require.Equal(t, "4", bccspConfig.PKCS11.KeyIDs[1].ID)
	os.Unsetenv("CORE_PEER_BCCSP_DEFAULT")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11_HASH")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11_SECURITY")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11_LIBRARY")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11_LABEL")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11_PIN")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11_SOFTWAREVERIFY")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11_IMMUTABLE")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11_ALTID")
	os.Unsetenv("CORE_PEER_BCCSP_PKCS11")
}
