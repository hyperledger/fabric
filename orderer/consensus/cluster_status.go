/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consensus

// StatusReporter is implemented by cluster-type Chain implementations.
// It allows the node to report its cluster relation and its status within that relation.
// This information is used to generate the channelparticipation.ChannelInfo in response
// to a "List" request on a particular channel.
//
// Not all chains must implement this, in particular non-cluster-type (solo, kafka) are
// assigned a StaticStatusReporter at construction time.
type StatusReporter interface {
	// StatusReport provides the cluster relation and status.
	// See:  channelparticipation.ChannelInfo for more details.
	StatusReport() (clusterRelation, status string)
}

// StaticStatusReporter is intended for chains that do not implement the StatusReporter interface.
type StaticStatusReporter struct {
	ClusterRelation string
	Status          string
}

func (s StaticStatusReporter) StatusReport() (string, string) {
	return s.ClusterRelation, s.Status
}
