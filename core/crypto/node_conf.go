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

package crypto

import (
	"errors"
	"path/filepath"

	"github.com/spf13/viper"
)

func (node *nodeImpl) initConfiguration(name string) (err error) {
	// Set logger
	prefix := eTypeToString(node.eType)

	// Set configuration
	node.conf = &configuration{prefix: prefix, name: name}
	if err = node.conf.init(); err != nil {
		return
	}

	node.Debugf("Data will be stored at [%s]", node.conf.configurationPath)

	return
}

type configuration struct {
	prefix string
	name   string

	logPrefix string

	rootDataPath      string
	configurationPath string
	keystorePath      string
	rawsPath          string
	tCertsPath        string

	configurationPathProperty string
	ecaPAddressProperty       string
	tcaPAddressProperty       string
	tlscaPAddressProperty     string

	securityLevel                  int
	hashAlgorithm                  string
	confidentialityProtocolVersion string

	tlsServerName string

	multiThreading bool
	tCertBatchSize int
}

func (conf *configuration) init() error {
	conf.configurationPathProperty = "peer.fileSystemPath"
	conf.ecaPAddressProperty = "peer.pki.eca.paddr"
	conf.tcaPAddressProperty = "peer.pki.tca.paddr"
	conf.tlscaPAddressProperty = "peer.pki.tlsca.paddr"
	conf.logPrefix = "[" + conf.prefix + "." + conf.name + "] "

	// Check mandatory fields
	if err := conf.checkProperty(conf.configurationPathProperty); err != nil {
		return err
	}
	if err := conf.checkProperty(conf.ecaPAddressProperty); err != nil {
		return err
	}
	if err := conf.checkProperty(conf.tcaPAddressProperty); err != nil {
		return err
	}
	if err := conf.checkProperty(conf.tlscaPAddressProperty); err != nil {
		return err
	}

	conf.configurationPath = viper.GetString(conf.configurationPathProperty)
	conf.rootDataPath = conf.configurationPath

	// Set configuration path
	conf.configurationPath = filepath.Join(
		conf.configurationPath,
		"crypto", conf.prefix, conf.name,
	)

	// Set ks path
	conf.keystorePath = filepath.Join(conf.configurationPath, "ks")

	// Set raws path
	conf.rawsPath = filepath.Join(conf.keystorePath, "raw")

	// Set tCerts path
	conf.tCertsPath = filepath.Join(conf.keystorePath, "tcerts")

	conf.securityLevel = 384
	if viper.IsSet("security.level") {
		ovveride := viper.GetInt("security.level")
		if ovveride != 0 {
			conf.securityLevel = ovveride
		}
	}

	conf.hashAlgorithm = "SHA3"
	if viper.IsSet("security.hashAlgorithm") {
		ovveride := viper.GetString("security.hashAlgorithm")
		if ovveride != "" {
			conf.hashAlgorithm = ovveride
		}
	}

	conf.confidentialityProtocolVersion = "1.2"
	if viper.IsSet("security.confidentialityProtocolVersion") {
		ovveride := viper.GetString("security.confidentialityProtocolVersion")
		if ovveride != "" {
			conf.confidentialityProtocolVersion = ovveride
		}
	}

	// Set TLS host override
	conf.tlsServerName = "tlsca"
	if viper.IsSet("peer.pki.tls.serverhostoverride") {
		ovveride := viper.GetString("peer.pki.tls.serverhostoverride")
		if ovveride != "" {
			conf.tlsServerName = ovveride
		}
	}

	// Set tCertBatchSize
	conf.tCertBatchSize = 200
	if viper.IsSet("security.tcert.batch.size") {
		ovveride := viper.GetInt("security.tcert.batch.size")
		if ovveride != 0 {
			conf.tCertBatchSize = ovveride
		}
	}

	// Set multithread
	conf.multiThreading = false
	if viper.IsSet("security.multithreading.enabled") {
		conf.multiThreading = viper.GetBool("security.multithreading.enabled")
	}

	return nil
}

func (conf *configuration) checkProperty(property string) error {
	res := viper.GetString(property)
	if res == "" {
		return errors.New("Property not specified in configuration file. Please check that property is set: " + property)
	}
	return nil
}

func (conf *configuration) getTCAPAddr() string {
	return viper.GetString(conf.tcaPAddressProperty)
}

func (conf *configuration) getECAPAddr() string {
	return viper.GetString(conf.ecaPAddressProperty)
}

func (conf *configuration) getTLSCAPAddr() string {
	return viper.GetString(conf.tlscaPAddressProperty)
}

func (conf *configuration) getConfPath() string {
	return conf.configurationPath
}

func (conf *configuration) getTCertsPath() string {
	return conf.tCertsPath
}

func (conf *configuration) getKeyStorePath() string {
	return conf.keystorePath
}

func (conf *configuration) getRootDatastorePath() string {
	return conf.rootDataPath
}

func (conf *configuration) getRawsPath() string {
	return conf.rawsPath
}

func (conf *configuration) getKeyStoreFilename() string {
	return "db"
}

func (conf *configuration) getKeyStoreFilePath() string {
	return filepath.Join(conf.getKeyStorePath(), conf.getKeyStoreFilename())
}

func (conf *configuration) getPathForAlias(alias string) string {
	return filepath.Join(conf.getRawsPath(), alias)
}

func (conf *configuration) getQueryStateKeyFilename() string {
	return "query.key"
}

func (conf *configuration) getEnrollmentKeyFilename() string {
	return "enrollment.key"
}

func (conf *configuration) getEnrollmentCertFilename() string {
	return "enrollment.cert"
}

func (conf *configuration) getEnrollmentIDPath() string {
	return filepath.Join(conf.getRawsPath(), conf.getEnrollmentIDFilename())
}

func (conf *configuration) getEnrollmentIDFilename() string {
	return "enrollment.id"
}

func (conf *configuration) getTCACertsChainFilename() string {
	return "tca.cert.chain"
}

func (conf *configuration) getECACertsChainFilename() string {
	return "eca.cert.chain"
}

func (conf *configuration) getTLSCACertsChainFilename() string {
	return "tlsca.cert.chain"
}

func (conf *configuration) getTLSCACertsExternalPath() string {
	return viper.GetString("peer.pki.tls.rootcert.file")
}

func (conf *configuration) isTLSEnabled() bool {
	return viper.GetBool("peer.pki.tls.enabled")
}

func (conf *configuration) isTLSClientAuthEnabled() bool {
	return viper.GetBool("peer.pki.tls.client.auth.enabled")
}

func (conf *configuration) IsMultithreadingEnabled() bool {
	return conf.multiThreading
}

func (conf *configuration) getTCAServerName() string {
	return conf.tlsServerName
}

func (conf *configuration) getECAServerName() string {
	return conf.tlsServerName
}

func (conf *configuration) getTLSCAServerName() string {
	return conf.tlsServerName
}

func (conf *configuration) getTLSKeyFilename() string {
	return "tls.key"
}

func (conf *configuration) getTLSCertFilename() string {
	return "tls.cert"
}

func (conf *configuration) getTLSRootCertFilename() string {
	return "tls.cert.chain"
}

func (conf *configuration) getEnrollmentChainKeyFilename() string {
	return "chain.key"
}

func (conf *configuration) getTCertOwnerKDFKeyFilename() string {
	return "tca.kdf.key"
}

func (conf *configuration) getTCertBatchSize() int {
	return conf.tCertBatchSize
}

func (conf *configuration) GetConfidentialityProtocolVersion() string {
	return conf.confidentialityProtocolVersion
}
