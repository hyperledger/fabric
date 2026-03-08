/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

// BasicConfig is a configuration with two organizations and one peer per org.
// This configuration does not specify a consensus type.
func BasicConfig() *Config {
	return &Config{
		Organizations: []*Organization{{
			Name:          "OrdererOrg",
			MSPID:         "OrdererMSP",
			Domain:        "example.com",
			EnableNodeOUs: false,
			Users:         0,
			CA:            &CA{Hostname: "ca"},
		}, {
			Name:          "Org1",
			MSPID:         "Org1MSP",
			Domain:        "org1.example.com",
			EnableNodeOUs: true,
			Users:         2,
			CA:            &CA{Hostname: "ca"},
		}, {
			Name:          "Org2",
			MSPID:         "Org2MSP",
			Domain:        "org2.example.com",
			EnableNodeOUs: true,
			Users:         2,
			CA:            &CA{Hostname: "ca"},
		}},
		Consensus: &Consensus{},
		Orderers: []*Orderer{
			{Name: "orderer", Organization: "OrdererOrg"},
		},
		Channels: []*Channel{
			{Name: "testchannel", Profile: "TwoOrgsChannel"},
		},
		Peers: []*Peer{{
			Name:         "peer0",
			Organization: "Org1",
			Channels: []*PeerChannel{
				{Name: "testchannel", Anchor: true},
			},
		}, {
			Name:         "peer0",
			Organization: "Org2",
			Channels: []*PeerChannel{
				{Name: "testchannel", Anchor: true},
			},
		}},
		Profiles: []*Profile{
			{
				Name:     "TwoOrgsOrdererGenesis",
				Orderers: []string{"orderer"},
			},
			{
				Name:          "TwoOrgsChannel",
				Consortium:    "SampleConsortium",
				Organizations: []string{"Org1", "Org2"},
			},
		},
	}
}

// Utility methods for tests without the system channel.
// These methods start from BasicConfig() and only use each other progressively.

func BasicEtcdRaft() *Config {
	config := BasicConfig()

	config.Consensus.Type = "etcdraft"
	config.Profiles = []*Profile{
		{
			Name:          "TwoOrgsAppChannelEtcdRaft",
			Consortium:    "SampleConsortium",
			Orderers:      []string{"orderer"},
			Organizations: []string{"Org1", "Org2"},
		},
	}
	config.Channels = []*Channel{{Name: "testchannel", Profile: "TwoOrgsAppChannelEtcdRaft"}}

	return config
}

func MultiChannelEtcdRaft() *Config {
	config := BasicConfig()

	config.Consensus.Type = "etcdraft"
	config.Profiles = []*Profile{
		{
			Name:          "TwoOrgsAppChannelEtcdRaft",
			Consortium:    "SampleConsortium",
			Orderers:      []string{"orderer"},
			Organizations: []string{"Org1", "Org2"},
		},
	}
	config.Channels = []*Channel{
		{Name: "testchannel", Profile: "TwoOrgsAppChannelEtcdRaft"},
		{Name: "testchannel2", Profile: "TwoOrgsAppChannelEtcdRaft"},
	}

	for _, peer := range config.Peers {
		peer.Channels = []*PeerChannel{
			{Name: "testchannel", Anchor: true},
			{Name: "testchannel2", Anchor: true},
		}
	}

	return config
}

func MinimalRaft() *Config {
	config := BasicEtcdRaft()

	config.Peers[1].Channels = nil
	config.Channels = []*Channel{
		{Name: "testchannel", Profile: "OneOrgChannelEtcdRaft"},
	}
	config.Profiles = []*Profile{{
		Name:          "OneOrgChannelEtcdRaft",
		Orderers:      []string{"orderer"},
		Consortium:    "SampleConsortium",
		Organizations: []string{"Org1"},
	}}

	return config
}

// FullEtcdRaft is a configuration with two organizations and two peers per org.
func FullEtcdRaft() *Config {
	config := BasicEtcdRaft()

	config.Peers = append(
		config.Peers,
		&Peer{
			Name:         "peer1",
			Organization: "Org1",
			Channels: []*PeerChannel{
				{Name: "testchannel", Anchor: false},
			},
		},
		&Peer{
			Name:         "peer1",
			Organization: "Org2",
			Channels: []*PeerChannel{
				{Name: "testchannel", Anchor: false},
			},
		},
	)

	return config
}

func MultiNodeBFT() *Config {
	config := BasicConfig()

	config.Consensus.Type = "BFT"
	config.Orderers = []*Orderer{
		{Name: "orderer1", Organization: "OrdererOrg"},
		{Name: "orderer2", Organization: "OrdererOrg"},
		{Name: "orderer3", Organization: "OrdererOrg"},
	}
	config.Profiles = []*Profile{
		{
			Name:                "TwoOrgsAppChannelBFT",
			Consortium:          "SampleConsortium",
			Organizations:       []string{"Org1", "Org2"},
			Orderers:            []string{"orderer1", "orderer2", "orderer3"},
			ChannelCapabilities: []string{"V3_0"},
		},
	}
	config.Channels = []*Channel{{Name: "testchannel", Profile: "TwoOrgsAppChannelBFT"}}

	return config
}

// ThreeOrgEtcdRaft returns a simple configuration with three organizations instead of two.
func ThreeOrgEtcdRaft() *Config {
	config := BasicEtcdRaft()
	config.Organizations = append(
		config.Organizations,
		&Organization{
			Name:   "Org3",
			MSPID:  "Org3MSP",
			Domain: "org3.example.com",
			Users:  2,
			CA:     &CA{Hostname: "ca"},
		},
	)

	config.Channels[0].Profile = "ThreeOrgsAppChannel"
	config.Peers = append(
		config.Peers,
		&Peer{
			Name:         "peer0",
			Organization: "Org3",
			Channels: []*PeerChannel{
				{Name: "testchannel", Anchor: true},
			},
		},
	)
	config.Profiles = []*Profile{{
		Name:          "ThreeOrgsAppChannel",
		Orderers:      []string{"orderer"},
		Organizations: []string{"Org1", "Org2", "Org3"},
	}}

	return config
}

func BasicEtcdRaftWithIdemix() *Config {
	config := BasicEtcdRaft()

	// Add idemix organization
	config.Organizations = append(config.Organizations, &Organization{
		Name:          "Org3",
		MSPID:         "Org3MSP",
		MSPType:       "idemix",
		Domain:        "org3.example.com",
		EnableNodeOUs: false,
		Users:         0,
		CA:            &CA{Hostname: "ca"},
	})
	// Add idemix organization
	config.Profiles[0].Organizations = append(config.Profiles[0].Organizations, "Org3")

	return config
}

func MultiNodeEtcdRaft() *Config {
	config := BasicEtcdRaft()
	config.Orderers = []*Orderer{
		{Name: "orderer1", Organization: "OrdererOrg"},
		{Name: "orderer2", Organization: "OrdererOrg"},
		{Name: "orderer3", Organization: "OrdererOrg"},
	}
	config.Profiles[0].Orderers = []string{"orderer1", "orderer2", "orderer3"}

	return config
}

func BasicSmartBFT() *Config {
	config := BasicConfig()
	config.Consensus.Type = "BFT"
	config.Profiles = []*Profile{{
		Name:     "SampleDevModeSmartBFT",
		Orderers: []string{"orderer"},
	}, {
		Name:          "TwoOrgsChannel",
		Consortium:    "SampleConsortium",
		Organizations: []string{"Org1", "Org2"},
	}}
	return config
}

func MultiNodeSmartBFT() *Config {
	config := BasicSmartBFT()
	config.Orderers = []*Orderer{
		{Name: "orderer1", Organization: "OrdererOrg", Id: 1},
		{Name: "orderer2", Organization: "OrdererOrg", Id: 2},
		{Name: "orderer3", Organization: "OrdererOrg", Id: 3},
		{Name: "orderer4", Organization: "OrdererOrg", Id: 4},
	}
	config.Profiles = []*Profile{{
		ChannelCapabilities: []string{"V3_0"},
		Name:                "SampleDevModeSmartBFT",
		Orderers:            []string{"orderer1", "orderer2", "orderer3", "orderer4"},
		Organizations:       []string{"Org1", "Org2"},
		AppCapabilities:     []string{"V2_0"},
	}}

	config.Channels = []*Channel{
		{Name: "testchannel1", Profile: "SampleDevModeSmartBFT"},
		{Name: "testchannel2", Profile: "SampleDevModeSmartBFT"},
	}

	for _, peer := range config.Peers {
		peer.Channels = []*PeerChannel{
			{Name: "testchannel1", Anchor: true},
			{Name: "testchannel2", Anchor: true},
		}
	}
	return config
}
