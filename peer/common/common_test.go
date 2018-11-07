/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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
	dir := os.TempDir() + "/" + util.GenerateUUID()
	err := common.InitCrypto(dir, "SampleOrg", msp.ProviderTypeToString(msp.FABRIC))
	assert.Error(t, err, "Should be able to initialize crypto with non-existing directory")
	assert.Contains(t, err.Error(), fmt.Sprintf("folder \"%s\" does not exist", dir))
}

func TestInitCrypto(t *testing.T) {
	mspConfigPath, err := configtest.GetDevMspDir()
	localMspId := "SampleOrg"
	err = common.InitCrypto(mspConfigPath, localMspId, msp.ProviderTypeToString(msp.FABRIC))
	assert.NoError(t, err, "Unexpected error [%s] calling InitCrypto()", err)
	err = common.InitCrypto("/etc/foobaz", localMspId, msp.ProviderTypeToString(msp.FABRIC))
	assert.Error(t, err, fmt.Sprintf("Expected error [%s] calling InitCrypto()", err))
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
	assert.Equal(t, "error", flogging.Global.Level("test").String())
	flogging.MustGetLogger("chaincode")
	assert.Equal(t, flogging.Global.DefaultLevel().String(), flogging.Global.Level("chaincode").String())
	flogging.MustGetLogger("test.test2")
	flogging.ActivateSpec("test.test2=warn")
	assert.Equal(t, "warn", flogging.Global.Level("test.test2").String())

	origEnvValue := os.Getenv("FABRIC_LOGGING_SPEC")
	os.Setenv("FABRIC_LOGGING_SPEC", "chaincode=debug:test.test2=fatal:abc=error")
	common.InitCmd(nil, nil)
	assert.Equal(t, "debug", flogging.Global.Level("chaincode").String())
	assert.Equal(t, "info", flogging.Global.Level("test").String())
	assert.Equal(t, "fatal", flogging.Global.Level("test.test2").String())
	assert.Equal(t, "error", flogging.Global.Level("abc").String())
	os.Setenv("FABRIC_LOGGING_SPEC", origEnvValue)
}
