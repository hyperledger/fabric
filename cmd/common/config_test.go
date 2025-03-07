/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	crand "crypto/rand"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/cmd/common/comm"
	"github.com/hyperledger/fabric/cmd/common/signer"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	var seed [32]byte
	_, _ = crand.Read(seed[:])
	r := rand.New(rand.NewChaCha8(seed))
	configFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("config-%d.yaml", r.Int()))
	fmt.Println(configFilePath)
	t.Run("save and load a config", func(t *testing.T) {
		c := Config{
			TLSConfig: comm.Config{
				CertPath:       "foo",
				KeyPath:        "foo",
				PeerCACertPath: "foo",
				Timeout:        time.Second * 3,
			},
			SignerConfig: signer.Config{
				KeyPath:      "foo",
				IdentityPath: "foo",
				MSPID:        "foo",
			},
		}

		err := c.ToFile(configFilePath)
		defer os.RemoveAll(configFilePath)
		require.NoError(t, err)

		c2, err := ConfigFromFile(configFilePath)
		require.NoError(t, err)
		require.Equal(t, c, c2)
	})

	t.Run("bad config isn't saved", func(t *testing.T) {
		c := Config{}
		err := c.ToFile(configFilePath)
		require.Contains(t, err.Error(), "config isn't valid")
	})

	t.Run("bad config isn't loaded", func(t *testing.T) {
		_, err := ConfigFromFile(filepath.Join("testdata", "not_a_yaml.yaml"))
		require.Contains(t, err.Error(), "error unmarshalling YAML file")
	})

	t.Run("file that doesn't exist isn't loaded", func(t *testing.T) {
		_, err := ConfigFromFile(filepath.Join("testdata", "not_a_file.yaml"))
		require.Contains(t, err.Error(), "no such file or directory")
	})
}
