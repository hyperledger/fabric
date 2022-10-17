//go:build pkcs11
// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestBCCSPPKCS11EnvVars(t *testing.T) {
	t.Setenv("CORE_PEER_BCCSP_DEFAULT", "PKCS11")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_HASH", "SHA2")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_SECURITY", "384")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_LIBRARY", "lib.so")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_LABEL", "token")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_PIN", "password")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_SOFTWAREVERIFY", "1")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_IMMUTABLE", "true")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_ALTID", "01234567890abcdef")
	t.Setenv("CORE_PEER_BCCSP_PKCS11_KEYIDS", "1 2,3 4")

	viper.SetConfigName(common.CmdRoot)
	viper.SetEnvPrefix(common.CmdRoot)
	configtest.AddDevConfigPath(nil)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("could not read config %s\n", err)
		os.Exit(-1)
	}

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
}
