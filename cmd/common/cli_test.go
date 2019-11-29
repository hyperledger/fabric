/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/cmd/common/signer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestCLI(t *testing.T) {
	var testCmdInvoked bool
	var exited bool
	// Overwrite exit
	terminate = func(_ int) {
		exited = true
	}
	// Overwrite stdout with writing to this buffer
	testBuff := &bytes.Buffer{}
	outWriter = testBuff

	var returnValue error
	cli := NewCLI("cli", "cli help")
	testCommand := func(conf Config) error {
		// If we exited, the command wasn't executed
		if exited {
			return nil
		}
		// Else, the command was executed - so ensure it was executed
		// with the expected config
		assert.Equal(t, Config{
			SignerConfig: signer.Config{
				MSPID:        "SampleOrg",
				KeyPath:      "key.pem",
				IdentityPath: "cert.pem",
			},
		}, conf)
		testCmdInvoked = true
		return returnValue
	}
	cli.Command("test", "test help", testCommand)

	t.Run("Loading a non existent config", func(t *testing.T) {
		defer testBuff.Reset()
		// Overwrite user home directory with testdata
		dir := filepath.Join("testdata", "non_existent_config")
		cli.Run([]string{"test", "--configFile", filepath.Join(dir, "config.yaml")})
		assert.Contains(t, testBuff.String(), fmt.Sprint("Failed loading config open ", dir))
		assert.Contains(t, testBuff.String(), "config.yaml: no such file or directory")
		assert.True(t, exited)
	})

	t.Run("Loading a valid config and the command succeeds", func(t *testing.T) {
		defer testBuff.Reset()
		testCmdInvoked = false
		exited = false
		// Overwrite user home directory with testdata
		dir := filepath.Join("testdata", "valid_config")
		// Ensure that a valid config results in running our command
		cli.Run([]string{"test", "--configFile", filepath.Join(dir, "config.yaml")})
		assert.True(t, testCmdInvoked)
		assert.False(t, exited)
	})

	t.Run("Loading a valid config but the command fails", func(t *testing.T) {
		returnValue = errors.New("something went wrong")
		defer func() {
			returnValue = nil
		}()
		defer testBuff.Reset()
		testCmdInvoked = false
		exited = false
		// Overwrite user home directory with testdata
		dir := filepath.Join("testdata", "valid_config")
		// Ensure that a valid config results in running our command
		cli.Run([]string{"test", "--configFile", filepath.Join(dir, "config.yaml")})
		assert.True(t, testCmdInvoked)
		assert.True(t, exited)
		assert.Contains(t, testBuff.String(), "something went wrong")
	})

	t.Run("Saving a config", func(t *testing.T) {
		defer testBuff.Reset()
		testCmdInvoked = false
		exited = false
		dir := filepath.Join(os.TempDir(), fmt.Sprintf("config%d", rand.Int()))
		os.Mkdir(dir, 0700)
		defer os.RemoveAll(dir)

		userCert := filepath.Join(dir, "cert.pem")
		userKey := filepath.Join(dir, "key.pem")
		userCertFlag := fmt.Sprintf("--userCert=%s", userCert)
		userKeyFlag := fmt.Sprintf("--userKey=%s", userKey)
		os.Create(userCert)
		os.Create(userKey)

		cli.Run([]string{saveConfigCommand, "--MSP=SampleOrg", userCertFlag, userKeyFlag})
		assert.Contains(t, testBuff.String(), "--configFile must be used to specify the configuration file")
		testBuff.Reset()
		// Persist the config
		cli.Run([]string{saveConfigCommand, "--MSP=SampleOrg", userCertFlag, userKeyFlag, "--configFile", filepath.Join(dir, "config.yaml")})

		// Run a different command and ensure the config was successfully persisted
		cli.Command("assert", "", func(conf Config) error {
			assert.Equal(t, Config{
				SignerConfig: signer.Config{
					MSPID:        "SampleOrg",
					KeyPath:      userKey,
					IdentityPath: userCert,
				},
			}, conf)
			return nil
		})
		cli.Run([]string{"assert", "--configFile", filepath.Join(dir, "config.yaml")})
	})
}
