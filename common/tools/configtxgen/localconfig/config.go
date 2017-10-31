/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localconfig

import (
	"fmt"

	"strings"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	logging "github.com/op/go-logging"

	"github.com/spf13/viper"

	"path/filepath"

	cf "github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/msp"
)

const (
	pkgLogID = "common/tools/configtxgen/localconfig"

	// Prefix identifies the prefix for the configtxgen-related ENV vars.
	Prefix string = "CONFIGTX"
)

var (
	logger *logging.Logger

	configName string
)

func init() {
	logger = flogging.MustGetLogger(pkgLogID)
	flogging.SetModuleLevel(pkgLogID, "error")

	configName = strings.ToLower(Prefix)
}

const (
	// TestChainID is the channel name used for testing purposes when one is not given
	TestChainID = "testchainid"

	// SampleInsecureSoloProfile references the sample profile which does not include any MSPs and uses solo for ordering.
	SampleInsecureSoloProfile = "SampleInsecureSolo"
	// SampleDevModeSoloProfile references the sample profile which requires only basic membership for admin privileges and uses solo for ordering.
	SampleDevModeSoloProfile = "SampleDevModeSolo"
	// SampleSingleMSPSoloProfile references the sample profile which includes only the sample MSP and uses solo for ordering.
	SampleSingleMSPSoloProfile = "SampleSingleMSPSolo"
	// SampleSingleMSPSoloV11Profile references the sample profile which includes only the sample MSP with v1.1 capabilities defined and uses solo for ordering.
	SampleSingleMSPSoloV11Profile = "SampleSingleMSPSoloV1_1"

	// SampleInsecureKafkaProfile references the sample profile which does not include any MSPs and uses Kafka for ordering.
	SampleInsecureKafkaProfile = "SampleInsecureKafka"
	// SampleDevModeKafkaProfile references the sample profile which requires only basic membership for admin privileges and uses Kafka for ordering.
	SampleDevModeKafkaProfile = "SampleDevModeKafka"
	// SampleSingleMSPKafkaProfile references the sample profile which includes only the sample MSP and uses Kafka for ordering.
	SampleSingleMSPKafkaProfile = "SampleSingleMSPKafka"
	// SampleSingleMSPKafkaV11Profile references the sample profile which includes only the sample MSP with v1.1 capabilities defined and uses Kafka for ordering.
	SampleSingleMSPKafkaV11Profile = "SampleSingleMSPKafkaV1_1"

	// SampleSingleMSPChannelProfile references the sample profile which includes only the sample MSP and is used to create a channel
	SampleSingleMSPChannelProfile = "SampleSingleMSPChannel"
	// SampleSingleMSPChannelV11Profile references the sample profile which includes only the sample MSP with v1.1 capabilities and is used to create a channel
	SampleSingleMSPChannelV11Profile = "SampleSingleMSPChannelV1_1"

	// SampleConsortiumName is the sample consortium from the sample configtx.yaml
	SampleConsortiumName = "SampleConsortium"
	// SampleOrgName is the name of the sample org in the sample profiles
	SampleOrgName = "SampleOrg"

	// AdminRoleAdminPrincipal is set as AdminRole to cause the MSP role of type Admin to be used as the admin principal default
	AdminRoleAdminPrincipal = "Role.ADMIN"
	// MemberRoleAdminPrincipal is set as AdminRole to cause the MSP role of type Member to be used as the admin principal default
	MemberRoleAdminPrincipal = "Role.MEMBER"
)

// TopLevel consists of the structs used by the configtxgen tool.
type TopLevel struct {
	Profiles      map[string]*Profile        `yaml:"Profiles"`
	Organizations []*Organization            `yaml:"Organizations"`
	Application   *Application               `yaml:"Application"`
	Orderer       *Orderer                   `yaml:"Orderer"`
	Capabilities  map[string]map[string]bool `yaml:"Capabilities"`
}

// Profile encodes orderer/application configuration combinations for the configtxgen tool.
type Profile struct {
	Consortium   string                 `yaml:"Consortium"`
	Application  *Application           `yaml:"Application"`
	Orderer      *Orderer               `yaml:"Orderer"`
	Consortiums  map[string]*Consortium `yaml:"Consortiums"`
	Capabilities map[string]bool        `yaml:"Capabilities"`
}

// Consortium represents a group of organizations which may create channels with eachother
type Consortium struct {
	Organizations []*Organization `yaml:"Organizations"`
}

// Application encodes the application-level configuration needed in config transactions.
type Application struct {
	Organizations []*Organization `yaml:"Organizations"`
	Capabilities  map[string]bool `yaml:"Capabilities"`
}

// Organization encodes the organization-level configuration needed in config transactions.
type Organization struct {
	Name           string `yaml:"Name"`
	ID             string `yaml:"ID"`
	MSPDir         string `yaml:"MSPDir"`
	MSPType        string `yaml:"MSPType"`
	AdminPrincipal string `yaml:"AdminPrincipal"`

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
	Capabilities  map[string]bool `yaml:"Capabilities"`
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
			AbsoluteMaxBytes:  10 * 1024 * 1024,
			PreferredMaxBytes: 512 * 1024,
		},
		Kafka: Kafka{
			Brokers: []string{"127.0.0.1:9092"},
		},
	},
}

// LoadTopLevel simply loads the configtx.yaml file into the structs above
// and completes their initialization.  Note, for environment overrides to work properly
// within a profile, Load(profile string) should be called when attempting to work within
// a particular profile.
func LoadTopLevel() *TopLevel {
	config := viper.New()
	cf.InitViper(config, configName)

	// For environment variables
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()

	replacer := strings.NewReplacer(".", "_")
	config.SetEnvKeyReplacer(replacer)

	err := config.ReadInConfig()
	if err != nil {
		logger.Panic("Error reading configuration: ", err)
	}
	logger.Debugf("Using config file: %s", config.ConfigFileUsed())

	var uconf TopLevel
	err = viperutil.EnhancedExactUnmarshal(config, &uconf)
	if err != nil {
		logger.Panic("Error unmarshaling config into struct: ", err)
	}

	(&uconf).completeInitialization(filepath.Dir(config.ConfigFileUsed()))

	logger.Infof("Loaded configuration: %s", config.ConfigFileUsed())

	return &uconf
}

// Load returns the orderer/application config combination that corresponds to a given profile.
func Load(profile string) *Profile {
	config := viper.New()
	cf.InitViper(config, configName)

	// For environment variables
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()

	// This replacer allows substitution within the particular profile without having to fully qualify the name
	replacer := strings.NewReplacer(strings.ToUpper(fmt.Sprintf("profiles.%s.", profile)), "", ".", "_")
	config.SetEnvKeyReplacer(replacer)

	err := config.ReadInConfig()
	if err != nil {
		logger.Panic("Error reading configuration: ", err)
	}
	logger.Debugf("Using config file: %s", config.ConfigFileUsed())

	var uconf TopLevel
	err = viperutil.EnhancedExactUnmarshal(config, &uconf)
	if err != nil {
		logger.Panic("Error unmarshaling config into struct: ", err)
	}

	result, ok := uconf.Profiles[profile]
	if !ok {
		logger.Panic("Could not find profile: ", profile)
	}

	result.completeInitialization(filepath.Dir(config.ConfigFileUsed()))

	logger.Infof("Loaded configuration: %s", config.ConfigFileUsed())

	return result
}

func (t *TopLevel) completeInitialization(configDir string) {
	for _, org := range t.Organizations {
		org.completeInitialization(configDir)
	}

	if t.Orderer != nil {
		t.Orderer.completeInitialization()
	}
}

func (p *Profile) completeInitialization(configDir string) {
	if p.Orderer != nil {
		for _, org := range p.Orderer.Organizations {
			org.completeInitialization(configDir)
		}
	}

	if p.Application != nil {
		for _, org := range p.Application.Organizations {
			org.completeInitialization(configDir)
		}
	}

	if p.Consortiums != nil {
		for _, consortium := range p.Consortiums {
			for _, org := range consortium.Organizations {
				org.completeInitialization(configDir)
			}
		}
	}

	// Some profiles will not define orderer parameters
	if p.Orderer != nil {
		p.Orderer.completeInitialization()
	}
}

func (org *Organization) completeInitialization(configDir string) {
	// set the MSP type; if none is specified we assume BCCSP
	if org.MSPType == "" {
		org.MSPType = msp.ProviderTypeToString(msp.FABRIC)
	}

	if org.AdminPrincipal == "" {
		org.AdminPrincipal = AdminRoleAdminPrincipal
	}
	translatePaths(configDir, org)
}

func (oc *Orderer) completeInitialization() {
	for {
		switch {
		case oc.OrdererType == "":
			logger.Infof("Orderer.OrdererType unset, setting to %s", genesisDefaults.Orderer.OrdererType)
			oc.OrdererType = genesisDefaults.Orderer.OrdererType
		case oc.Addresses == nil:
			logger.Infof("Orderer.Addresses unset, setting to %s", genesisDefaults.Orderer.Addresses)
			oc.Addresses = genesisDefaults.Orderer.Addresses
		case oc.BatchTimeout == 0:
			logger.Infof("Orderer.BatchTimeout unset, setting to %s", genesisDefaults.Orderer.BatchTimeout)
			oc.BatchTimeout = genesisDefaults.Orderer.BatchTimeout
		case oc.BatchSize.MaxMessageCount == 0:
			logger.Infof("Orderer.BatchSize.MaxMessageCount unset, setting to %s", genesisDefaults.Orderer.BatchSize.MaxMessageCount)
			oc.BatchSize.MaxMessageCount = genesisDefaults.Orderer.BatchSize.MaxMessageCount
		case oc.BatchSize.AbsoluteMaxBytes == 0:
			logger.Infof("Orderer.BatchSize.AbsoluteMaxBytes unset, setting to %s", genesisDefaults.Orderer.BatchSize.AbsoluteMaxBytes)
			oc.BatchSize.AbsoluteMaxBytes = genesisDefaults.Orderer.BatchSize.AbsoluteMaxBytes
		case oc.BatchSize.PreferredMaxBytes == 0:
			logger.Infof("Orderer.BatchSize.PreferredMaxBytes unset, setting to %s", genesisDefaults.Orderer.BatchSize.PreferredMaxBytes)
			oc.BatchSize.PreferredMaxBytes = genesisDefaults.Orderer.BatchSize.PreferredMaxBytes
		case oc.Kafka.Brokers == nil:
			logger.Infof("Orderer.Kafka.Brokers unset, setting to %v", genesisDefaults.Orderer.Kafka.Brokers)
			oc.Kafka.Brokers = genesisDefaults.Orderer.Kafka.Brokers
		default:
			return
		}
	}
}

func translatePaths(configDir string, org *Organization) {
	cf.TranslatePathInPlace(configDir, &org.MSPDir)
}
