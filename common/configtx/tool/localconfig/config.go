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

var logger = logging.MustGetLogger("configtx/tool/localconfig")

const (
	// SampleInsecureProfile references the sample profile which does not include any MSPs and uses solo for ordering.
	SampleInsecureProfile = "SampleInsecureSolo"
	// SampleSingleMSPSoloProfile references the sample profile which includes only the sample MSP and uses solo for ordering.
	SampleSingleMSPSoloProfile = "SampleSingleMSPSolo"

	// Prefix identifies the prefix for the configtxgen-related ENV vars.
	Prefix string = "CONFIGTX"
)

var (
	configName     string
	configFileName string
)

func init() {
	configName = strings.ToLower(Prefix)
	configFileName = configName + ".yaml"
}

// TopLevel consists of the structs used by the configtxgen tool.
type TopLevel struct {
	Profiles      map[string]*Profile `yaml:"Profiles"`
	Organizations []*Organization     `yaml:"Organizations"`
	Application   *Application        `yaml:"Application"`
	Orderer       *Orderer            `yaml:"Orderer"`
}

// Profile encodes orderer/application configuration combinations for the configtxgen tool.
type Profile struct {
	Application *Application `yaml:"Application"`
	Orderer     *Orderer     `yaml:"Orderer"`
}

// Application encodes the application-level configuration needed in config transactions.
type Application struct {
	Organizations []*Organization `yaml:"Organizations"`
}

// Organization encodes the organization-level configuration needed in config transactions.
type Organization struct {
	Name   string             `yaml:"Name"`
	ID     string             `yaml:"ID"`
	MSPDir string             `yaml:"MSPDir"`
	BCCSP  *bccsp.FactoryOpts `yaml:"BCCSP"`

	// Note: Viper deserialization does not seem to care for
	// embedding of types, so we use one organization struct
	// for both orderers and applications.
	AnchorPeers []*AnchorPeer `yaml:"AnchorPeers"`
}

// AnchorPeer encodes the necessary fields to identify an anchor peer.
type AnchorPeer struct {
	Host string `yaml:"Host"`
	Port int    `yaml:"Port"`
}

// ApplicationOrganization ...
// TODO This should probably be removed
type ApplicationOrganization struct {
	Organization `yaml:"Organization"`
}

// Orderer contains configuration which is used for the
// bootstrapping of an orderer by the provisional bootstrapper.
type Orderer struct {
	OrdererType   string          `yaml:"OrdererType"`
	Addresses     []string        `yaml:"Addresses"`
	BatchTimeout  time.Duration   `yaml:"BatchTimeout"`
	BatchSize     BatchSize       `yaml:"BatchSize"`
	Kafka         Kafka           `yaml:"Kafka"`
	Organizations []*Organization `yaml:"Organizations"`
	MaxChannels   uint64          `yaml:"MaxChannels"`
}

// BatchSize contains configuration affecting the size of batches.
type BatchSize struct {
	MaxMessageCount   uint32 `yaml:"MaxMessageSize"`
	AbsoluteMaxBytes  uint32 `yaml:"AbsoluteMaxBytes"`
	PreferredMaxBytes uint32 `yaml:"PreferredMaxBytes"`
}

// Kafka contains configuration for the Kafka-based orderer.
type Kafka struct {
	Brokers []string `yaml:"Brokers"`
}

var genesisDefaults = TopLevel{
	Orderer: &Orderer{
		OrdererType:  "solo",
		Addresses:    []string{"127.0.0.1:7050"},
		BatchTimeout: 2 * time.Second,
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

func (p *Profile) completeInitialization() {
	for {
		switch {
		case p.Orderer.OrdererType == "":
			logger.Infof("Orderer.OrdererType unset, setting to %s", genesisDefaults.Orderer.OrdererType)
			p.Orderer.OrdererType = genesisDefaults.Orderer.OrdererType
		case p.Orderer.Addresses == nil:
			logger.Infof("Orderer.Addresses unset, setting to %s", genesisDefaults.Orderer.Addresses)
			p.Orderer.Addresses = genesisDefaults.Orderer.Addresses
		case p.Orderer.BatchTimeout == 0:
			logger.Infof("Orderer.BatchTimeout unset, setting to %s", genesisDefaults.Orderer.BatchTimeout)
			p.Orderer.BatchTimeout = genesisDefaults.Orderer.BatchTimeout
		case p.Orderer.BatchSize.MaxMessageCount == 0:
			logger.Infof("Orderer.BatchSize.MaxMessageCount unset, setting to %s", genesisDefaults.Orderer.BatchSize.MaxMessageCount)
			p.Orderer.BatchSize.MaxMessageCount = genesisDefaults.Orderer.BatchSize.MaxMessageCount
		case p.Orderer.BatchSize.AbsoluteMaxBytes == 0:
			logger.Infof("Orderer.BatchSize.AbsoluteMaxBytes unset, setting to %s", genesisDefaults.Orderer.BatchSize.AbsoluteMaxBytes)
			p.Orderer.BatchSize.AbsoluteMaxBytes = genesisDefaults.Orderer.BatchSize.AbsoluteMaxBytes
		case p.Orderer.BatchSize.PreferredMaxBytes == 0:
			logger.Infof("Orderer.BatchSize.PreferredMaxBytes unset, setting to %s", genesisDefaults.Orderer.BatchSize.PreferredMaxBytes)
			p.Orderer.BatchSize.PreferredMaxBytes = genesisDefaults.Orderer.BatchSize.PreferredMaxBytes
		case p.Orderer.Kafka.Brokers == nil:
			logger.Infof("Orderer.Kafka.Brokers unset, setting to %v", genesisDefaults.Orderer.Kafka.Brokers)
			p.Orderer.Kafka.Brokers = genesisDefaults.Orderer.Kafka.Brokers
		default:
			return
		}
	}
}

// Load returns the orderer/application config combination that corresponds to a given profile.
func Load(profile string) *Profile {
	config := viper.New()

	config.SetConfigName("configtx")
	var cfgPath string

	// Candidate paths to look for the config file in, based on GOPATH
	searchPath := []string{
		os.Getenv("ORDERER_CFG_PATH"),
		os.Getenv("PEER_CFG_PATH"),
	}

	for _, p := range filepath.SplitList(os.Getenv("GOPATH")) {
		searchPath = append(searchPath, filepath.Join(p, "src/github.com/hyperledger/fabric/common/configtx/tool/"))
	}

	for _, path := range searchPath {
		if len(path) == 0 {
			// No point printing a "checking for" message below for an empty path
			continue
		}
		logger.Infof("Looking for %s in: %s", configFileName, path)
		if _, err := os.Stat(filepath.Join(path, configFileName)); err != nil {
			// The YAML file does not exist in this component of the path
			continue
		}
		cfgPath = path
		logger.Infof("Found %s there", configFileName)
		break
	}

	if cfgPath == "" {
		logger.Fatalf("Could not find %s in paths of %s."+
			"Try setting ORDERER_CFG_PATH, PEER_CFG_PATH, or GOPATH correctly.",
			configFileName, searchPath)
	}

	// Path to look for the config file in
	config.AddConfigPath(cfgPath)

	// For environment variables
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()
	// This replacer allows substitution within the particular profile without having to fully qualify the name
	replacer := strings.NewReplacer(strings.ToUpper(fmt.Sprintf("profiles.%s.", profile)), "", ".", "_")
	config.SetEnvKeyReplacer(replacer)

	err := config.ReadInConfig()
	if err != nil {
		logger.Panicf("Error reading configuration from %s in %s: %s", configFileName, cfgPath, err)
	}

	var uconf TopLevel
	err = viperutil.EnhancedExactUnmarshal(config, &uconf)
	if err != nil {
		logger.Panicf("Error unmarshaling config into struct: %s", err)
	}

	result, ok := uconf.Profiles[profile]
	if !ok {
		logger.Panicf("Could not find profile %s", profile)
	}

	result.completeInitialization()

	return result
}
