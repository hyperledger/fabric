/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"fmt"
	"os/exec"

	"github.com/tedsuo/ifrit/ginkgomon"
)

type Orderer struct {
	Path                        string
	ConfigDir                   string
	LedgerLocation              string
	ConfigtxOrdererKafkaBrokers string
	LogLevel                    string
}

func (o *Orderer) setupEnvironment(cmd *exec.Cmd) {
	if o.ConfigDir != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", o.ConfigDir))
	}
	if o.LedgerLocation != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ORDERER_FILELEDGER_LOCATION=%s", o.LedgerLocation))
	}
	if o.ConfigtxOrdererKafkaBrokers != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CONFIGTX_ORDERER_KAFKA_BROKERS=%s", o.ConfigtxOrdererKafkaBrokers))
	}
	if o.LogLevel != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ORDERER_GENERAL_LOGLEVEL=%s", o.LogLevel))
	}
}

func (o *Orderer) New() *ginkgomon.Runner {
	cmd := exec.Command(o.Path)
	o.setupEnvironment(cmd)

	return ginkgomon.New(ginkgomon.Config{
		Name:          "orderer",
		AnsiColorCode: "35m",
		Command:       cmd,
	})
}
