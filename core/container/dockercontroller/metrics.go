/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller

import "github.com/hyperledger/fabric/common/metrics"

var (
	chaincodeContainerBuildDuration = metrics.HistogramOpts{
		Namespace:    "dockercontroller",
		Name:         "chaincode_container_build_duration",
		Help:         "The time to build a chaincode container in seconds.",
		LabelNames:   []string{"chaincode", "success"},
		StatsdFormat: "%{#fqname}.%{chaincode}.%{success}",
	}
)

type BuildMetrics struct {
	ChaincodeContainerBuildDuration metrics.Histogram
}

func NewBuildMetrics(p metrics.Provider) *BuildMetrics {
	return &BuildMetrics{
		ChaincodeContainerBuildDuration: p.NewHistogram(chaincodeContainerBuildDuration),
	}
}
