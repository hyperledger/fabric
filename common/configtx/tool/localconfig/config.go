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

package localconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/viperutil"

	"github.com/op/go-logging"
	"github.com/spf13/viper"

	bccsp "github.com/hyperledger/fabric/bccsp/factory"
)

const (
	// SampleInsecureProfile references the sample profile which does not include any MSPs and uses solo for consensus
	SampleInsecureProfile = "SampleInsecureSolo"

	// SampleSingleMSPSoloProfile references the sample profile which includes only the sample MSP and uses solo for consensus
	SampleSingleMSPSoloProfile = "SampleSingleMSPSolo"
)

var logger = logging.MustGetLogger("configtx/tool/localconfig")

// Prefix is the default config prefix for the orderer
const Prefix string = "CONFIGTX"

// TopLevel contains the genesis structures for use by the provisional bootstrapper
type TopLevel struct {
	Profiles      map[string]*Profile `yaml:"Profiles"`
	Organizations []*Organization     `yaml:"Organizations"`
	Application   *Application        `yaml:"Application"`
	Orderer       *Orderer            `yaml:"Orderer"`
}

// TopLevel contains the genesis structures for use by the provisional bootstrapper
type Profile struct {
	Application *Application `yaml:"Application"`
	Orderer     *Orderer     `yaml:"Orderer"`
}

// Application encodes the configuration needed for the config transaction
type Application struct {
	Organizations []*Organization `yaml:"Organizations"`
}

type Organization struct {
	Name   string             `yaml:"Name"`
	ID     string             `yaml:"ID"`
	MSPDir string             `yaml:"MSPDir"`
	BCCSP  *bccsp.FactoryOpts `yaml:"BCCSP"`

	// Note, the viper deserialization does not seem to care for
	// embedding of types, so we use one organization structure for
	// both orderers and applications
	AnchorPeers []*AnchorPeer `yaml:"AnchorPeers"`
}

type AnchorPeer struct {
	Host string `yaml:"Host"`
	Port int    `yaml:"Port"`
}

type ApplicationOrganization struct {
	Organization `yaml:"Organization"`
}

// Orderer contains config which is used for orderer genesis by the provisional bootstrapper
type Orderer struct {
	OrdererType   string          `yaml:"OrdererType"`
	Addresses     []string        `yaml:"Addresses"`
	BatchTimeout  time.Duration   `yaml:"BatchTimeout"`
	BatchSize     BatchSize       `yaml:"BatchSize"`
	Kafka         Kafka           `yaml:"Kafka"`
	Organizations []*Organization `yaml:"Organizations"`
	MaxChannels   uint64          `yaml:"MaxChannels"`
}

// BatchSize contains configuration affecting the size of batches
type BatchSize struct {
	MaxMessageCount   uint32 `yaml:"MaxMessageSize"`
	AbsoluteMaxBytes  uint32 `yaml:"AbsoluteMaxBytes"`
	PreferredMaxBytes uint32 `yaml:"PreferredMaxBytes"`
}

// Kafka contains config for the Kafka orderer
type Kafka struct {
	Brokers []string `yaml:"Brokers"`
}

var genesisDefaults = TopLevel{
	Orderer: &Orderer{
		OrdererType:  "solo",
		Addresses:    []string{"127.0.0.1:7050"},
		BatchTimeout: 10 * time.Second,
		BatchSize: BatchSize{
			MaxMessageCount:   10,
			AbsoluteMaxBytes:  100000000,
			PreferredMaxBytes: 512 * 1024,
		},
		Kafka: Kafka{
			Brokers: []string{"127.0.0.1:9092"},
		},
	},
}

func (g *Profile) completeInitialization() {
	for {
		switch {
		case g.Orderer.OrdererType == "":
			logger.Infof("Orderer.OrdererType unset, setting to %s", genesisDefaults.Orderer.OrdererType)
			g.Orderer.OrdererType = genesisDefaults.Orderer.OrdererType
		case g.Orderer.Addresses == nil:
			logger.Infof("Orderer.Addresses unset, setting to %s", genesisDefaults.Orderer.Addresses)
			g.Orderer.Addresses = genesisDefaults.Orderer.Addresses
		case g.Orderer.BatchTimeout == 0:
			logger.Infof("Orderer.BatchTimeout unset, setting to %s", genesisDefaults.Orderer.BatchTimeout)
			g.Orderer.BatchTimeout = genesisDefaults.Orderer.BatchTimeout
		case g.Orderer.BatchSize.MaxMessageCount == 0:
			logger.Infof("Orderer.BatchSize.MaxMessageCount unset, setting to %s", genesisDefaults.Orderer.BatchSize.MaxMessageCount)
			g.Orderer.BatchSize.MaxMessageCount = genesisDefaults.Orderer.BatchSize.MaxMessageCount
		case g.Orderer.BatchSize.AbsoluteMaxBytes == 0:
			logger.Infof("Orderer.BatchSize.AbsoluteMaxBytes unset, setting to %s", genesisDefaults.Orderer.BatchSize.AbsoluteMaxBytes)
			g.Orderer.BatchSize.AbsoluteMaxBytes = genesisDefaults.Orderer.BatchSize.AbsoluteMaxBytes
		case g.Orderer.BatchSize.PreferredMaxBytes == 0:
			logger.Infof("Orderer.BatchSize.PreferredMaxBytes unset, setting to %s", genesisDefaults.Orderer.BatchSize.PreferredMaxBytes)
			g.Orderer.BatchSize.PreferredMaxBytes = genesisDefaults.Orderer.BatchSize.PreferredMaxBytes
		case g.Orderer.Kafka.Brokers == nil:
			logger.Infof("Orderer.Kafka.Brokers unset, setting to %v", genesisDefaults.Orderer.Kafka.Brokers)
			g.Orderer.Kafka.Brokers = genesisDefaults.Orderer.Kafka.Brokers
		default:
			return
		}
	}
}

func Load(profile string) *Profile {
	config := viper.New()

	config.SetConfigName("configtx")
	var cfgPath string

	// Path to look for the config file in based on GOPATH
	searchPath := []string{
		os.Getenv("ORDERER_CFG_PATH"),
		os.Getenv("PEER_CFG_PATH"),
	}

	for _, p := range filepath.SplitList(os.Getenv("GOPATH")) {
		searchPath = append(searchPath, filepath.Join(p, "src/github.com/hyperledger/fabric/common/configtx/tool/"))
	}

	for _, genesisPath := range searchPath {
		logger.Infof("Checking for configtx.yaml at: %s", genesisPath)
		if _, err := os.Stat(filepath.Join(genesisPath, "configtx.yaml")); err != nil {
			// The yaml file does not exist in this component of the path
			continue
		}
		cfgPath = genesisPath
	}

	if cfgPath == "" {
		logger.Fatalf("Could not find configtx.yaml in paths of %s.  Try setting ORDERER_CFG_PATH, PEER_CFG_PATH, or GOPATH correctly", searchPath)
	}
	config.AddConfigPath(cfgPath) // Path to look for the config file in

	// for environment variables
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()
	// This replacer allows substitution within the particular profile without having to fully qualify the name
	replacer := strings.NewReplacer(strings.ToUpper(fmt.Sprintf("profiles.%s.", profile)), "", ".", "_")
	config.SetEnvKeyReplacer(replacer)

	err := config.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Error reading %s plugin config from %s: %s", Prefix, cfgPath, err))
	}

	var uconf TopLevel

	err = viperutil.EnhancedExactUnmarshal(config, &uconf)
	if err != nil {
		panic(fmt.Errorf("Error unmarshaling into structure: %s", err))
	}

	result, ok := uconf.Profiles[profile]
	if !ok {
		logger.Panicf("Could not find profile %s", profile)
	}

	result.completeInitialization()

	return result
}
