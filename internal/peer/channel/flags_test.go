/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"testing"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestOrdererFlags(t *testing.T) {
	var (
		ca       = "root.crt"
		key      = "client.key"
		cert     = "client.crt"
		endpoint = "orderer.example.com:7050"
		sn       = "override.example.com"
	)

	testCmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {
			t.Logf("rootcert: %s", viper.GetString("orderer.tls.rootcert.file"))
			require.Equal(t, ca, viper.GetString("orderer.tls.rootcert.file"))
			require.Equal(t, key, viper.GetString("orderer.tls.clientKey.file"))
			require.Equal(t, cert, viper.GetString("orderer.tls.clientCert.file"))
			require.Equal(t, endpoint, viper.GetString("orderer.address"))
			require.Equal(t, sn, viper.GetString("orderer.tls.serverhostoverride"))
			require.Equal(t, true, viper.GetBool("orderer.tls.enabled"))
			require.Equal(t, true, viper.GetBool("orderer.tls.clientAuthRequired"))
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			common.SetOrdererEnv(cmd, args)
		},
	}

	runCmd := Cmd(nil)

	runCmd.AddCommand(testCmd)

	runCmd.SetArgs([]string{
		"test", "--cafile", ca, "--keyfile", key,
		"--certfile", cert, "--orderer", endpoint, "--tls", "--clientauth",
		"--ordererTLSHostnameOverride", sn,
	})
	err := runCmd.Execute()
	require.NoError(t, err)

	// check env one more time
	t.Logf("address: %s", viper.GetString("orderer.address"))
	require.Equal(t, ca, viper.GetString("orderer.tls.rootcert.file"))
	require.Equal(t, key, viper.GetString("orderer.tls.clientKey.file"))
	require.Equal(t, cert, viper.GetString("orderer.tls.clientCert.file"))
	require.Equal(t, endpoint, viper.GetString("orderer.address"))
	require.Equal(t, sn, viper.GetString("orderer.tls.serverhostoverride"))
	require.Equal(t, true, viper.GetBool("orderer.tls.enabled"))
	require.Equal(t, true, viper.GetBool("orderer.tls.clientAuthRequired"))
}
