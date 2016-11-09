/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package sbft

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/sbft/crypto"
	"github.com/hyperledger/fabric/orderer/sbft/persist"
)

func ReadJsonConfig(file string) (*ConsensusConfig, error) {
	configData, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	jconfig := &JsonConfig{}
	err = json.Unmarshal(configData, jconfig)
	if err != nil {
		return nil, err
	}

	config := &ConsensusConfig{}
	config.Consensus = jconfig.Consensus
	config.Peers = make(map[string][]byte)
	for n, p := range jconfig.Peers {
		if p.Address == "" {
			return nil, fmt.Errorf("The required peer address is missing (for peer %d)", n)
		}
		cert, err := crypto.ParseCertPEM(p.Cert)
		if err != nil {
			fmt.Println("exiting")
			return nil, err
		}
		config.Peers[p.Address] = cert
	}

	// XXX check for duplicate cert
	if config.Consensus.N != 0 && int(config.Consensus.N) != len(config.Peers) {
		return nil, fmt.Errorf("peer config does not match pbft N")
	}

	config.Consensus.N = uint64(len(config.Peers))

	return config, nil
}

func SaveConfig(p *persist.Persist, c *ConsensusConfig) error {
	craw, err := proto.Marshal(c)
	if err != nil {
		return err
	}
	err = p.StoreState("config", craw)
	return err
}

func RestoreConfig(p *persist.Persist) (*ConsensusConfig, error) {
	raw, err := p.ReadState("config")
	if err != nil {
		return nil, err
	}
	config := &ConsensusConfig{}
	err = proto.Unmarshal(raw, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
