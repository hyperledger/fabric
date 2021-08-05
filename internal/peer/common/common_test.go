/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/msp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitConfig(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	type args struct {
		cmdRoot string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "Empty command root",
			args:    args{cmdRoot: ""},
			wantErr: true,
		},
		{
			name:    "Bad command root",
			args:    args{cmdRoot: "cre"},
			wantErr: true,
		},
		{
			name:    "Good command root",
			args:    args{cmdRoot: "core"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := common.InitConfig(tt.args.cmdRoot); (err != nil) != tt.wantErr {
				t.Errorf("InitConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInitCryptoMissingDir(t *testing.T) {
	dir := path.Join(os.TempDir(), util.GenerateUUID())
	err := common.InitCrypto(dir, "SampleOrg", msp.ProviderTypeToString(msp.FABRIC))
	assert.Error(t, err, "Should not be able to initialize crypto with non-existing directory")
	assert.Contains(t, err.Error(), fmt.Sprintf("specified path \"%s\" does not exist", dir))
}

func TestInitCryptoFileNotDir(t *testing.T) {
	file := path.Join(os.TempDir(), util.GenerateUUID())
	err := ioutil.WriteFile(file, []byte{}, 0644)
	assert.Nil(t, err, "Failed to create test file")
	defer os.Remove(file)
	err = common.InitCrypto(file, "SampleOrg", msp.ProviderTypeToString(msp.FABRIC))
	assert.Error(t, err, "Should not be able to initialize crypto with a file instead of a directory")
	assert.Contains(t, err.Error(), fmt.Sprintf("specified path \"%s\" is not a directory", file))
}

func TestInitCrypto(t *testing.T) {
	mspConfigPath := configtest.GetDevMspDir()
	localMspId := "SampleOrg"
	err := common.InitCrypto(mspConfigPath, localMspId, msp.ProviderTypeToString(msp.FABRIC))
	assert.NoError(t, err, "Unexpected error [%s] calling InitCrypto()", err)
	localMspId = ""
	err = common.InitCrypto(mspConfigPath, localMspId, msp.ProviderTypeToString(msp.FABRIC))
	assert.Error(t, err, fmt.Sprintf("Expected error [%s] calling InitCrypto()", err))
}

func TestSetBCCSPKeystorePath(t *testing.T) {
	cfgKey := "peer.BCCSP.SW.FileKeyStore.KeyStore"
	cfgPath := "./testdata"
	absPath, _ := filepath.Abs(cfgPath)
	keystorePath := "/msp/keystore"

	os.Setenv("FABRIC_CFG_PATH", cfgPath)
	viper.Reset()
	_ = common.InitConfig("notset")
	common.SetBCCSPKeystorePath()
	t.Log(viper.GetString(cfgKey))
	assert.Equal(t, "", viper.GetString(cfgKey))

	viper.Reset()
	_ = common.InitConfig("absolute")
	common.SetBCCSPKeystorePath()
	t.Log(viper.GetString(cfgKey))
	assert.Equal(t, keystorePath, viper.GetString(cfgKey))

	viper.Reset()
	_ = common.InitConfig("relative")
	common.SetBCCSPKeystorePath()
	t.Log(viper.GetString(cfgKey))
	assert.Equal(t, filepath.Join(absPath, keystorePath),
		viper.GetString(cfgKey))

	viper.Reset()
	os.Unsetenv("FABRIC_CFG_PATH")
}

func TestCheckLogLevel(t *testing.T) {
	type args struct {
		level string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "Empty level",
			args:    args{level: ""},
			wantErr: true,
		},
		{
			name:    "Valid level",
			args:    args{level: "warning"},
			wantErr: false,
		},
		{
			name:    "Invalid level",
			args:    args{level: "foobaz"},
			wantErr: true,
		},
		{
			name:    "Valid level",
			args:    args{level: "error"},
			wantErr: false,
		},
		{
			name:    "Valid level",
			args:    args{level: "info"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := common.CheckLogLevel(tt.args.level); (err != nil) != tt.wantErr {
				t.Errorf("CheckLogLevel() args = %v error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestGetDefaultSigner(t *testing.T) {
	tests := []struct {
		name    string
		want    msp.SigningIdentity
		wantErr bool
	}{
		{
			name:    "Should return DefaultSigningIdentity",
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := common.GetDefaultSigner()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDefaultSigner() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestInitCmd(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	defer viper.Reset()

	// test that InitCmd doesn't remove existing loggers from the logger levels map
	flogging.MustGetLogger("test")
	flogging.ActivateSpec("test=error")
	assert.Equal(t, "error", flogging.LoggerLevel("test"))
	flogging.MustGetLogger("chaincode")
	assert.Equal(t, flogging.DefaultLevel(), flogging.LoggerLevel("chaincode"))
	flogging.MustGetLogger("test.test2")
	flogging.ActivateSpec("test.test2=warn")
	assert.Equal(t, "warn", flogging.LoggerLevel("test.test2"))

	origEnvValue := os.Getenv("FABRIC_LOGGING_SPEC")
	os.Setenv("FABRIC_LOGGING_SPEC", "chaincode=debug:test.test2=fatal:abc=error")
	common.InitCmd(&cobra.Command{}, nil)
	assert.Equal(t, "debug", flogging.LoggerLevel("chaincode"))
	assert.Equal(t, "info", flogging.LoggerLevel("test"))
	assert.Equal(t, "fatal", flogging.LoggerLevel("test.test2"))
	assert.Equal(t, "error", flogging.LoggerLevel("abc"))
	os.Setenv("FABRIC_LOGGING_SPEC", origEnvValue)
}

func TestInitCmdWithoutInitCrypto(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	defer viper.Reset()

	peerCmd := &cobra.Command{
		Use: "peer",
	}
	lifecycleCmd := &cobra.Command{
		Use: "lifecycle",
	}
	chaincodeCmd := &cobra.Command{
		Use: "chaincode",
	}
	packageCmd := &cobra.Command{
		Use: "package",
	}
	// peer lifecycle chaincode package
	chaincodeCmd.AddCommand(packageCmd)
	lifecycleCmd.AddCommand(chaincodeCmd)
	peerCmd.AddCommand(lifecycleCmd)

	// MSPCONFIGPATH is default value
	common.InitCmd(packageCmd, nil)

	// set MSPCONFIGPATH to be a missing dir, the function InitCrypto will fail
	// confirm that 'peer lifecycle chaincode package' mandates does not require MSPCONFIG information
	viper.SetEnvPrefix("core")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	dir := os.TempDir() + "/" + util.GenerateUUID()
	os.Setenv("CORE_PEER_MSPCONFIGPATH", dir)

	common.InitCmd(packageCmd, nil)
}

func TestConfigFromEnv(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "peer-clientcert")
	require.NoError(t, err)
	defer os.RemoveAll(tempdir)

	// peer client config
	address, _, clientConfig, err := common.ConfigFromEnv("peer")
	require.NoError(t, err)
	require.Equal(t, "", address, "ClientConfig.address by default not set")
	require.Equal(t, common.DefaultConnTimeout, clientConfig.Timeout, "ClientConfig.Timeout should be set to default value of %v", common.DefaultConnTimeout)
	require.Equal(t, false, clientConfig.SecOpts.UseTLS, "ClientConfig.SecOpts.UseTLS default value should be false")
	require.Equal(t, comm.DefaultMaxRecvMsgSize, clientConfig.MaxRecvMsgSize, "ServerConfig.MaxRecvMsgSize should be set to default value %v", comm.DefaultMaxRecvMsgSize)
	require.Equal(t, comm.DefaultMaxSendMsgSize, clientConfig.MaxSendMsgSize, "ServerConfig.MaxSendMsgSize should be set to default value %v", comm.DefaultMaxSendMsgSize)

	viper.Set("peer.address", "127.0.0.1")
	viper.Set("peer.client.connTimeout", "30s")
	viper.Set("peer.maxRecvMsgSize", "1024")
	viper.Set("peer.maxSendMsgSize", "2048")
	address, _, clientConfig, err = common.ConfigFromEnv("peer")
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", address, "ClientConfig.address should be set to 127.0.0.1")
	require.Equal(t, 30*time.Second, clientConfig.Timeout, "ClientConfig.Timeout should be set to default value of 30s")
	require.Equal(t, 1024, clientConfig.MaxRecvMsgSize, "ClientConfig.MaxRecvMsgSize should be set to 1024")
	require.Equal(t, 2048, clientConfig.MaxSendMsgSize, "ClientConfig.maxSendMsgSize should be set to 2048")

	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.rootcert.file", "./filenotfound.pem")
	_, _, _, err = common.ConfigFromEnv("peer")
	require.Error(t, err, "ClientConfig should return with bad root cert file path")

	viper.Set("peer.tls.enabled", false)
	viper.Set("peer.tls.clientAuthRequired", true)
	viper.Set("peer.tls.clientKey.file", "./filenotfound.pem")
	_, _, clientConfig, err = common.ConfigFromEnv("peer")
	require.Equal(t, false, clientConfig.SecOpts.UseTLS, "ClientConfig.SecOpts.UseTLS should be false")
	require.Error(t, err, "ClientConfig should return with client key file path")

	org1CA, err := tlsgen.NewCA()
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "org1-ca-cert.pem"), org1CA.CertBytes(), 0o644)
	require.NoError(t, err)
	org1ServerKP, err := org1CA.NewServerCertKeyPair("localhost")
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "org1-peer1-cert.pem"), org1ServerKP.Cert, 0o644)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempdir, "org1-peer1-key.pem"), org1ServerKP.Key, 0o600)
	require.NoError(t, err)

	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.clientAuthRequired", true)
	viper.Set("peer.tls.rootcert.file", filepath.Join(tempdir, "org1-ca-cert.pem"))
	viper.Set("peer.tls.clientCert.file", filepath.Join(tempdir, "org1-peer1-cert.pem"))
	viper.Set("peer.tls.clientKey.file", filepath.Join(tempdir, "org1-peer1-key.pem"))
	_, _, clientConfig, err = common.ConfigFromEnv("peer")
	require.NoError(t, err)
	require.Equal(t, 1, len(clientConfig.SecOpts.ServerRootCAs), "ClientConfig.SecOpts.ServerRootCAs should contain 1 entries")
	require.Equal(t, org1ServerKP.Key, clientConfig.SecOpts.Key, "Client.SecOpts.Key should be set to configured key")
	require.Equal(t, org1ServerKP.Cert, clientConfig.SecOpts.Certificate, "Client.SecOpts.Certificate shoulbe bet set to configured certificate")
}
