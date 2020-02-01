/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"io/ioutil"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// NetworkConfig provides a static definition of a Hyperledger Fabric network
type NetworkConfig struct {
	Name                   string                          `yaml:"name"`
	Xtype                  string                          `yaml:"x-type"`
	Description            string                          `yaml:"description"`
	Version                string                          `yaml:"version"`
	Channels               map[string]ChannelNetworkConfig `yaml:"channels"`
	Organizations          map[string]OrganizationConfig   `yaml:"organizations"`
	Peers                  map[string]PeerConfig           `yaml:"peers"`
	Client                 ClientConfig                    `yaml:"client"`
	Orderers               map[string]OrdererConfig        `yaml:"orderers"`
	CertificateAuthorities map[string]CAConfig             `yaml:"certificateAuthorities"`
}

// ClientConfig - not currently used by CLI
type ClientConfig struct {
	Organization    string              `yaml:"organization"`
	Logging         LoggingType         `yaml:"logging"`
	CryptoConfig    CCType              `yaml:"cryptoconfig"`
	TLS             TLSType             `yaml:"tls"`
	CredentialStore CredentialStoreType `yaml:"credentialStore"`
}

// LoggingType not currently used by CLI
type LoggingType struct {
	Level string `yaml:"level"`
}

// CCType - not currently used by CLI
type CCType struct {
	Path string `yaml:"path"`
}

// TLSType - not currently used by CLI
type TLSType struct {
	Enabled bool `yaml:"enabled"`
}

// CredentialStoreType - not currently used by CLI
type CredentialStoreType struct {
	Path        string `yaml:"path"`
	CryptoStore struct {
		Path string `yaml:"path"`
	}
	Wallet string `yaml:"wallet"`
}

// ChannelNetworkConfig provides the definition of channels for the network
type ChannelNetworkConfig struct {
	// Orderers list of ordering service nodes
	Orderers []string `yaml:"orderers"`
	// Peers a list of peer-channels that are part of this organization
	// to get the real Peer config object, use the Name field and fetch NetworkConfig.Peers[Name]
	Peers map[string]PeerChannelConfig `yaml:"peers"`
	// Chaincodes list of services
	Chaincodes []string `yaml:"chaincodes"`
}

// PeerChannelConfig defines the peer capabilities
type PeerChannelConfig struct {
	EndorsingPeer  bool `yaml:"endorsingPeer"`
	ChaincodeQuery bool `yaml:"chaincodeQuery"`
	LedgerQuery    bool `yaml:"ledgerQuery"`
	EventSource    bool `yaml:"eventSource"`
}

// OrganizationConfig provides the definition of an organization in the network
// not currently used by CLI
type OrganizationConfig struct {
	MspID                  string    `yaml:"mspid"`
	Peers                  []string  `yaml:"peers"`
	CryptoPath             string    `yaml:"cryptoPath"`
	CertificateAuthorities []string  `yaml:"certificateAuthorities"`
	AdminPrivateKey        TLSConfig `yaml:"adminPrivateKey"`
	SignedCert             TLSConfig `yaml:"signedCert"`
}

// OrdererConfig defines an orderer configuration
// not currently used by CLI
type OrdererConfig struct {
	URL         string                 `yaml:"url"`
	GrpcOptions map[string]interface{} `yaml:"grpcOptions"`
	TLSCACerts  TLSConfig              `yaml:"tlsCACerts"`
}

// PeerConfig defines a peer configuration
type PeerConfig struct {
	URL         string                 `yaml:"url"`
	EventURL    string                 `yaml:"eventUrl"`
	GRPCOptions map[string]interface{} `yaml:"grpcOptions"`
	TLSCACerts  TLSConfig              `yaml:"tlsCACerts"`
}

// CAConfig defines a CA configuration
// not currently used by CLI
type CAConfig struct {
	URL         string                 `yaml:"url"`
	HTTPOptions map[string]interface{} `yaml:"httpOptions"`
	TLSCACerts  MutualTLSConfig        `yaml:"tlsCACerts"`
	Registrar   EnrollCredentials      `yaml:"registrar"`
	CaName      string                 `yaml:"caName"`
}

// EnrollCredentials holds credentials used for enrollment
// not currently used by CLI
type EnrollCredentials struct {
	EnrollID     string `yaml:"enrollId"`
	EnrollSecret string `yaml:"enrollSecret"`
}

// TLSConfig TLS configurations
type TLSConfig struct {
	// the following two fields are interchangeable.
	// If Path is available, then it will be used to load the cert
	// if Pem is available, then it has the raw data of the cert it will be used as-is
	// Certificate root certificate path
	Path string `yaml:"path"`
	// Certificate actual content
	Pem string `yaml:"pem"`
}

// MutualTLSConfig Mutual TLS configurations
// not currently used by CLI
type MutualTLSConfig struct {
	Pem []string `yaml:"pem"`

	// Certfiles root certificates for TLS validation (Comma separated path list)
	Path string `yaml:"path"`

	//Client TLS information
	Client TLSKeyPair `yaml:"client"`
}

// TLSKeyPair contains the private key and certificate for TLS encryption
// not currently used by CLI
type TLSKeyPair struct {
	Key  TLSConfig `yaml:"key"`
	Cert TLSConfig `yaml:"cert"`
}

// GetConfig unmarshals the provided connection profile into a network
// configuration struct
func GetConfig(fileName string) (*NetworkConfig, error) {
	if fileName == "" {
		return nil, errors.New("filename cannot be empty")
	}

	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, errors.Wrap(err, "error reading connection profile")
	}

	configData := string(data)
	config := &NetworkConfig{}
	err = yaml.Unmarshal([]byte(configData), &config)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling YAML")
	}

	return config, nil
}
