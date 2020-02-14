/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	ob "github.com/hyperledger/fabric-protos-go/orderer"
	eb "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	. "github.com/onsi/gomega"
)

func TestNewOrdererGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ordererType           string
		configMetadata        *eb.ConfigMetadata
		numOrdererGroupValues int
	}{
		{ordererType: ConsensusTypeSolo, configMetadata: nil, numOrdererGroupValues: 5},
		{ordererType: ConsensusTypeEtcdRaft, configMetadata: &eb.ConfigMetadata{}, numOrdererGroupValues: 5},
		{ordererType: ConsensusTypeKafka, configMetadata: nil, numOrdererGroupValues: 6},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.ordererType, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			ordererConf := baseOrderer()
			ordererConf.OrdererType = tt.ordererType
			ordererConf.EtcdRaft = tt.configMetadata

			mspConfig := &mb.MSPConfig{}

			ordererGroup, err := NewOrdererGroup(ordererConf, mspConfig)
			gt.Expect(err).NotTo(HaveOccurred())

			// OrdererGroup checks
			gt.Expect(len(ordererGroup.Groups)).To(Equal(2))
			gt.Expect(ordererGroup.Groups["Org1"]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org2"]).NotTo(BeNil())
			gt.Expect(len(ordererGroup.Values)).To(Equal(tt.numOrdererGroupValues))
			gt.Expect(ordererGroup.Values[BatchSizeKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Values[BatchTimeoutKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Values[ChannelRestrictionsKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Values[CapabilitiesKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Values[ConsensusTypeKey]).NotTo(BeNil())
			var consensusType ob.ConsensusType
			err = proto.Unmarshal(ordererGroup.Values[ConsensusTypeKey].Value, &consensusType)
			gt.Expect(err).NotTo(HaveOccurred())
			gt.Expect(consensusType.Type).To(Equal(tt.ordererType))
			gt.Expect(len(ordererGroup.Policies)).To(Equal(4))
			gt.Expect(ordererGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Policies[WritersPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Policies[BlockValidationPolicyKey]).NotTo(BeNil())

			// OrdererOrgGroup checks
			gt.Expect(len(ordererGroup.Groups["Org1"].Groups)).To(Equal(0))
			gt.Expect(len(ordererGroup.Groups["Org1"].Values)).To(Equal(2))
			gt.Expect(ordererGroup.Groups["Org1"].Values[MSPKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org1"].Values[EndpointsKey]).NotTo(BeNil())
			gt.Expect(len(ordererGroup.Groups["Org1"].Policies)).To(Equal(4))
			gt.Expect(ordererGroup.Groups["Org1"].Policies[AdminsPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org1"].Policies[ReadersPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org1"].Policies[WritersPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org1"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
			gt.Expect(len(ordererGroup.Groups["Org2"].Groups)).To(Equal(0))
			gt.Expect(len(ordererGroup.Groups["Org2"].Values)).To(Equal(2))
			gt.Expect(ordererGroup.Groups["Org2"].Values[MSPKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org2"].Values[EndpointsKey]).NotTo(BeNil())
			gt.Expect(len(ordererGroup.Groups["Org2"].Policies)).To(Equal(4))
			gt.Expect(ordererGroup.Groups["Org2"].Policies[AdminsPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org2"].Policies[ReadersPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org2"].Policies[WritersPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["Org2"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
		})
	}
}

func TestNewOrdererGroupFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName   string
		ordererMod func(*Orderer)
		err        error
	}{
		{
			testName: "When orderer group policy is empty",
			ordererMod: func(o *Orderer) {
				o.Policies = nil
			},
			err: errors.New("failed to add policies: no policies defined"),
		},
		{
			testName: "When marshalling etcdraft metadata for orderer group",
			ordererMod: func(o *Orderer) {
				o.OrdererType = ConsensusTypeEtcdRaft
				o.EtcdRaft = &eb.ConfigMetadata{
					Consenters: []*eb.Consenter{
						{
							Host:          "node-1.example.com",
							Port:          7050,
							ClientTlsCert: []byte("testdata/tls-client-1.pem"),
							ServerTlsCert: []byte("testdata/tls-server-1.pem"),
						},
						{
							Host:          "node-2.example.com",
							Port:          7050,
							ClientTlsCert: []byte("testdata/tls-client-2.pem"),
							ServerTlsCert: []byte("testdata/tls-server-2.pem"),
						},
						{
							Host:          "node-3.example.com",
							Port:          7050,
							ClientTlsCert: []byte("testdata/tls-client-3.pem"),
							ServerTlsCert: []byte("testdata/tls-server-3.pem"),
						},
					},
				}
			},
			err: errors.New("failed to marshal etcdraft metadata for orderer type etcdraft: cannot load client cert for consenter " +
				"node-1.example.com:7050: open testdata/tls-client-1.pem: no such file or directory"),
		},
		{
			testName: "When orderer type is unknown",
			ordererMod: func(o *Orderer) {
				o.OrdererType = "ConsensusTypeGreen"
			},
			err: errors.New("unknown orderer type ConsensusTypeGreen"),
		},
		{
			testName: "When EtcdRaft config is not set for consensus type etcdraft",
			ordererMod: func(o *Orderer) {
				o.OrdererType = ConsensusTypeEtcdRaft
				o.EtcdRaft = nil
			},
			err: errors.New("missing etcdraft metadata for orderer type etcdraft"),
		},
		{
			testName: "When adding policies to orderer org group",
			ordererMod: func(o *Orderer) {
				o.Organizations[0].Policies = nil
			},
			err: errors.New("failed to create orderer org group Org1: failed to add policies: no policies defined"),
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			ordererConf := baseOrderer()
			tt.ordererMod(ordererConf)

			mspConfig := &mb.MSPConfig{}

			ordererGroup, err := NewOrdererGroup(ordererConf, mspConfig)
			gt.Expect(err).To(MatchError(tt.err))
			gt.Expect(ordererGroup).To(BeNil())
		})
	}
}

func baseOrderer() *Orderer {
	return &Orderer{
		Policies:    createOrdererStandardPolicies(),
		OrdererType: ConsensusTypeSolo,
		Organizations: []*Organization{
			{
				Name:     "Org1",
				ID:       "Org1MSP",
				Policies: createOrgStandardPolicies(),
				OrdererEndpoints: []string{
					"localhost:123",
				},
			},
			{
				Name:     "Org2",
				ID:       "Org2MSP",
				Policies: createOrgStandardPolicies(),
				OrdererEndpoints: []string{
					"localhost:123",
				},
			},
		},
		Capabilities: map[string]bool{
			"V1_3": true,
		},
	}
}
