/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

// Templates can be used to provide custom templates to GenerateConfigTree.
type Templates struct {
	ConfigTx string `yaml:"configtx,omitempty"`
	Core     string `yaml:"core,omitempty"`
	Crypto   string `yaml:"crypto,omitempty"`
	Orderer  string `yaml:"orderer,omitempty"`
}

func (t *Templates) ConfigTxTemplate() string {
	if t.ConfigTx != "" {
		return t.ConfigTx
	}
	return DefaultConfigTxTemplate
}

func (t *Templates) CoreTemplate() string {
	if t.Core != "" {
		return t.Core
	}
	return DefaultCoreTemplate
}

func (t *Templates) CryptoTemplate() string {
	if t.Crypto != "" {
		return t.Crypto
	}
	return DefaultCryptoTemplate
}

func (t *Templates) OrdererTemplate() string {
	if t.Orderer != "" {
		return t.Orderer
	}
	return DefaultOrdererTemplate
}
