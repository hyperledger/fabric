/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package genesisconfig

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/stretchr/testify/require"
)

func TestLoadProfile(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	pNames := []string{
		SampleDevModeKafkaProfile,
		SampleDevModeSoloProfile,
		SampleSingleMSPChannelProfile,
		SampleSingleMSPKafkaProfile,
		SampleSingleMSPSoloProfile,
	}
	for _, pName := range pNames {
		t.Run(pName, func(t *testing.T) {
			p := Load(pName)
			require.NotNil(t, p, "profile should not be nil")
		})
	}
}

func TestLoadProfileWithPath(t *testing.T) {
	devConfigDir := configtest.GetDevConfigDir()

	pNames := []string{
		SampleDevModeKafkaProfile,
		SampleDevModeSoloProfile,
		SampleSingleMSPChannelProfile,
		SampleSingleMSPKafkaProfile,
		SampleSingleMSPSoloProfile,
	}
	for _, pName := range pNames {
		t.Run(pName, func(t *testing.T) {
			p := Load(pName, devConfigDir)
			require.NotNil(t, p, "profile should not be nil")
		})
	}
}

func TestLoadTopLevel(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	topLevel := LoadTopLevel()
	require.NotNil(t, topLevel.Application, "application should not be nil")
	require.NotNil(t, topLevel.Capabilities, "capabilities should not be nil")
	require.NotNil(t, topLevel.Orderer, "orderer should not be nil")
	require.NotNil(t, topLevel.Organizations, "organizations should not be nil")
	require.NotNil(t, topLevel.Profiles, "profiles should not be nil")
}

func TestLoadTopLevelWithPath(t *testing.T) {
	devConfigDir := configtest.GetDevConfigDir()

	topLevel := LoadTopLevel(devConfigDir)
	require.NotNil(t, topLevel.Application, "application should not be nil")
	require.NotNil(t, topLevel.Capabilities, "capabilities should not be nil")
	require.NotNil(t, topLevel.Orderer, "orderer should not be nil")
	require.NotNil(t, topLevel.Organizations, "organizations should not be nil")
	require.NotNil(t, topLevel.Profiles, "profiles should not be nil")
}

func TestConsensusSpecificInit(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	devConfigDir := configtest.GetDevConfigDir()

	t.Run("nil orderer type", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "",
			},
		}
		profile.completeInitialization(devConfigDir)

		require.Equal(t, profile.Orderer.OrdererType, genesisDefaults.Orderer.OrdererType)
	})

	t.Run("unknown orderer type", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "unknown",
			},
		}

		require.Panics(t, func() {
			profile.completeInitialization(devConfigDir)
		})
	})

	t.Run("solo", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "solo",
			},
		}
		profile.completeInitialization(devConfigDir)
		require.Nil(t, profile.Orderer.Kafka.Brokers, "Kafka config settings should not be set")
	})

	t.Run("kafka", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "kafka",
			},
		}
		profile.completeInitialization(devConfigDir)
		require.NotNil(t, profile.Orderer.Kafka.Brokers, "Kafka config settings should be set")
	})

	t.Run("raft", func(t *testing.T) {
		makeProfile := func(consenters []*etcdraft.Consenter, options *etcdraft.Options) *Profile {
			return &Profile{
				Orderer: &Orderer{
					OrdererType: "etcdraft",
					EtcdRaft: &etcdraft.ConfigMetadata{
						Consenters: consenters,
						Options:    options,
					},
				},
			}
		}
		t.Run("EtcdRaft section not specified in profile", func(t *testing.T) {
			profile := &Profile{
				Orderer: &Orderer{
					OrdererType: "etcdraft",
				},
			}

			require.Panics(t, func() {
				profile.completeInitialization(devConfigDir)
			})
		})

		t.Run("nil consenter set", func(t *testing.T) { // should panic
			profile := makeProfile(nil, nil)

			require.Panics(t, func() {
				profile.completeInitialization(devConfigDir)
			})
		})

		t.Run("single consenter", func(t *testing.T) {
			consenters := []*etcdraft.Consenter{
				{
					Host:          "node-1.example.com",
					Port:          7050,
					ClientTlsCert: []byte("path/to/client/cert"),
					ServerTlsCert: []byte("path/to/server/cert"),
				},
			}

			t.Run("invalid consenters specification", func(t *testing.T) {
				failingConsenterSpecifications := []*etcdraft.Consenter{
					{ // missing Host
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
						ServerTlsCert: []byte("path/to/server/cert"),
					},
					{ // missing Port
						Host:          "node-1.example.com",
						ClientTlsCert: []byte("path/to/client/cert"),
						ServerTlsCert: []byte("path/to/server/cert"),
					},
					{ // missing ClientTlsCert
						Host:          "node-1.example.com",
						Port:          7050,
						ServerTlsCert: []byte("path/to/server/cert"),
					},
					{ // missing ServerTlsCert
						Host:          "node-1.example.com",
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
					},
				}

				for _, consenter := range failingConsenterSpecifications {
					profile := makeProfile([]*etcdraft.Consenter{consenter}, nil)

					require.Panics(t, func() {
						profile.completeInitialization(devConfigDir)
					})
				}
			})

			t.Run("nil Options", func(t *testing.T) {
				profile := makeProfile(consenters, nil)
				profile.completeInitialization(devConfigDir)

				// need not be tested in subsequent tests
				require.NotNil(t, profile.Orderer.EtcdRaft, "EtcdRaft config settings should be set")
				require.Equal(t, profile.Orderer.EtcdRaft.Consenters[0].ClientTlsCert, consenters[0].ClientTlsCert,
					"Client TLS cert path should be correctly set")

				// specific assertion for this test context
				require.Equal(t, profile.Orderer.EtcdRaft.Options, genesisDefaults.Orderer.EtcdRaft.Options,
					"Options should be set to the default value")
			})

			t.Run("heartbeat tick specified in Options", func(t *testing.T) {
				heartbeatTick := uint32(2)
				options := &etcdraft.Options{ // partially set so that we can check that the other members are set to defaults
					HeartbeatTick: heartbeatTick,
				}
				profile := makeProfile(consenters, options)
				profile.completeInitialization(devConfigDir)

				// specific assertions for this test context
				require.Equal(t, profile.Orderer.EtcdRaft.Options.HeartbeatTick, heartbeatTick,
					"HeartbeatTick should be set to the specified value")
				require.Equal(t, profile.Orderer.EtcdRaft.Options.ElectionTick, genesisDefaults.Orderer.EtcdRaft.Options.ElectionTick,
					"ElectionTick should be set to the default value")
			})

			t.Run("election tick specified in Options", func(t *testing.T) {
				electionTick := uint32(20)
				options := &etcdraft.Options{ // partially set so that we can check that the other members are set to defaults
					ElectionTick: electionTick,
				}
				profile := makeProfile(consenters, options)
				profile.completeInitialization(devConfigDir)

				// specific assertions for this test context
				require.Equal(t, profile.Orderer.EtcdRaft.Options.ElectionTick, electionTick,
					"ElectionTick should be set to the specified value")
				require.Equal(t, profile.Orderer.EtcdRaft.Options.HeartbeatTick, genesisDefaults.Orderer.EtcdRaft.Options.HeartbeatTick,
					"HeartbeatTick should be set to the default value")
			})

			t.Run("panic on invalid Heartbeat and Election tick", func(t *testing.T) {
				options := &etcdraft.Options{
					HeartbeatTick: 2,
					ElectionTick:  1,
				}
				profile := makeProfile(consenters, options)

				require.Panics(t, func() {
					profile.completeInitialization(devConfigDir)
				})
			})

			t.Run("panic on invalid TickInterval", func(t *testing.T) {
				options := &etcdraft.Options{
					TickInterval: "500",
				}
				profile := makeProfile(consenters, options)

				require.Panics(t, func() {
					profile.completeInitialization(devConfigDir)
				})
			})
		})
	})
}

func TestLoadConfigCache(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	cfg := viperutil.New()
	devConfigDir := configtest.GetDevConfigDir()
	cfg.AddConfigPaths(devConfigDir)
	cfg.SetConfigName("configtx")
	err := cfg.ReadInConfig()
	require.NoError(t, err)

	configPath := cfg.ConfigFileUsed()
	c := &configCache{
		cache: make(map[string][]byte),
	}

	// Load the initial config, update the environment, and load again.
	// With the caching behavior, the update should not be reflected.
	initial, err := c.load(cfg, configPath)
	require.NoError(t, err)
	os.Setenv("ORDERER_KAFKA_RETRY_SHORTINTERVAL", "120s")
	updated, err := c.load(cfg, configPath)
	require.NoError(t, err)
	require.Equal(t, initial, updated, "expected %#v to equal %#v", updated, initial)

	// Change the configuration we got back and load again.
	// The  new value should not contain the updated to the initial
	initial.Orderer.OrdererType = "bad-Orderer-Type"
	updated, err = c.load(cfg, configPath)
	require.NoError(t, err)
	require.NotEqual(t, initial, updated, "expected %#v to not equal %#v", updated, initial)
}
