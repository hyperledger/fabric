/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"crypto/x509"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ob "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/ordererext"
	. "github.com/onsi/gomega"
)

func TestNewOrdererGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ordererType           string
		numOrdererGroupValues int
	}{
		{ordererType: ConsensusTypeSolo, numOrdererGroupValues: 5},
		{ordererType: ConsensusTypeEtcdRaft, numOrdererGroupValues: 5},
		{ordererType: ConsensusTypeKafka, numOrdererGroupValues: 6},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.ordererType, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			ordererConf := baseOrdererOfType(tt.ordererType)

			ordererGroup, err := newOrdererGroup(ordererConf)
			gt.Expect(err).NotTo(HaveOccurred())

			// OrdererGroup checks
			gt.Expect(len(ordererGroup.Groups)).To(Equal(1))
			gt.Expect(ordererGroup.Groups["OrdererOrg"]).NotTo(BeNil())
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
			gt.Expect(len(ordererGroup.Groups["OrdererOrg"].Groups)).To(Equal(0))
			gt.Expect(len(ordererGroup.Groups["OrdererOrg"].Values)).To(Equal(2))
			gt.Expect(ordererGroup.Groups["OrdererOrg"].Values[MSPKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["OrdererOrg"].Values[EndpointsKey]).NotTo(BeNil())
			gt.Expect(len(ordererGroup.Groups["OrdererOrg"].Policies)).To(Equal(4))
			gt.Expect(ordererGroup.Groups["OrdererOrg"].Policies[AdminsPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["OrdererOrg"].Policies[ReadersPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["OrdererOrg"].Policies[WritersPolicyKey]).NotTo(BeNil())
			gt.Expect(ordererGroup.Groups["OrdererOrg"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
		})
	}
}

func TestNewOrdererGroupFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName   string
		ordererMod func(*Orderer)
		err        string
	}{
		{
			testName: "When orderer group policy is empty",
			ordererMod: func(o *Orderer) {
				o.Policies = nil
			},
			err: "no policies defined",
		},
		{
			testName: "When orderer type is unknown",
			ordererMod: func(o *Orderer) {
				o.OrdererType = "ConsensusTypeGreen"
			},
			err: "unknown orderer type 'ConsensusTypeGreen'",
		},
		{
			testName: "When adding policies to orderer org group",
			ordererMod: func(o *Orderer) {
				o.Organizations[0].Policies = nil
			},
			err: "org group 'OrdererOrg': no policies defined",
		},
		{
			testName: "When missing consenters in EtcdRaft for consensus type etcdraft",
			ordererMod: func(o *Orderer) {
				o.OrdererType = ConsensusTypeEtcdRaft
				o.EtcdRaft = EtcdRaft{
					Consenters: nil,
				}
			},
			err: "marshaling etcdraft metadata for orderer type 'etcdraft': consenters are required",
		},
		{
			testName: "When missing a client tls cert in EtcdRaft for consensus type etcdraft",
			ordererMod: func(o *Orderer) {
				o.OrdererType = ConsensusTypeEtcdRaft
				o.EtcdRaft = EtcdRaft{
					Consenters: []Consenter{
						{
							Host:          "host1",
							Port:          123,
							ClientTLSCert: nil,
						},
					},
				}
			},
			err: "marshaling etcdraft metadata for orderer type 'etcdraft': client tls cert for consenter host1:123 is required",
		},
		{
			testName: "When missing a server tls cert in EtcdRaft for consensus type etcdraft",
			ordererMod: func(o *Orderer) {
				o.OrdererType = ConsensusTypeEtcdRaft
				o.EtcdRaft = EtcdRaft{
					Consenters: []Consenter{
						{
							Host:          "host1",
							Port:          123,
							ClientTLSCert: &x509.Certificate{},
							ServerTLSCert: nil,
						},
					},
				}
			},
			err: "marshaling etcdraft metadata for orderer type 'etcdraft': server tls cert for consenter host1:123 is required",
		},
		{
			testName: "When consensus state is invalid",
			ordererMod: func(o *Orderer) {
				o.State = "invalid state"
			},
			err: "unknown consensus state 'invalid state'",
		},
		{
			testName: "When consensus state is invalid",
			ordererMod: func(o *Orderer) {
				o.State = "invalid state"
			},
			err: "unknown consensus state 'invalid state'",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			ordererConf := baseSoloOrderer()
			tt.ordererMod(&ordererConf)

			ordererGroup, err := newOrdererGroup(ordererConf)
			gt.Expect(err).To(MatchError(tt.err))
			gt.Expect(ordererGroup).To(BeNil())
		})
	}
}

func TestUpdateOrdererConfiguration(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer()

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	originalOrdererAddresses, err := proto.Marshal(&cb.OrdererAddresses{
		Addresses: baseOrdererConf.Addresses,
	})
	gt.Expect(err).NotTo(HaveOccurred())

	imp, err := implicitMetaFromString(baseOrdererConf.Policies[AdminsPolicyKey].Rule)
	gt.Expect(err).NotTo(HaveOccurred())

	originalAdminsPolicy, err := proto.Marshal(imp)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
			Values: map[string]*cb.ConfigValue{
				OrdererAddressesKey: {
					Value:     originalOrdererAddresses,
					ModPolicy: AdminsPolicyKey,
				},
			},
			Policies: map[string]*cb.ConfigPolicy{
				AdminsPolicyKey: {
					Policy: &cb.Policy{
						Type:  int32(cb.Policy_IMPLICIT_META),
						Value: originalAdminsPolicy,
					},
					ModPolicy: AdminsPolicyKey,
				},
			},
		},
	}

	updatedOrdererConf := baseOrdererConf

	// Modify MaxMessageCount, Addresses, and ConesnsusType to etcdraft
	updatedOrdererConf.BatchSize.MaxMessageCount = 10000
	updatedOrdererConf.Addresses = []string{"newhost:345"}
	updatedOrdererConf.OrdererType = ConsensusTypeEtcdRaft
	updatedOrdererConf.EtcdRaft = EtcdRaft{
		Consenters: []Consenter{
			{
				Host:          "host1",
				Port:          123,
				ClientTLSCert: &x509.Certificate{},
				ServerTLSCert: &x509.Certificate{},
			},
		},
		Options: EtcdRaftOptions{},
	}

	err = UpdateOrdererConfiguration(config, updatedOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	expectedConfigJSON := `
{
	"channel_group": {
		"groups": {
			"Orderer": {
				"groups": {
					"OrdererOrg": {
						"groups": {},
						"mod_policy": "Admins",
						"policies": {
							"Admins": {
							"mod_policy": "Admins",
							"policy": {
							"type": 3,
							"value": {
							"rule": "MAJORITY",
							"sub_policy": "Admins"
							}
							},
							"version": "0"
							},
							"Endorsement": {
							"mod_policy": "Admins",
							"policy": {
							"type": 3,
							"value": {
							"rule": "MAJORITY",
							"sub_policy": "Endorsement"
							}
							},
							"version": "0"
							},
							"Readers": {
							"mod_policy": "Admins",
							"policy": {
							"type": 3,
							"value": {
							"rule": "ANY",
							"sub_policy": "Readers"
							}
							},
							"version": "0"
							},
							"Writers": {
							"mod_policy": "Admins",
							"policy": {
							"type": 3,
							"value": {
							"rule": "ANY",
							"sub_policy": "Writers"
							}
							},
							"version": "0"
							}
						},
						"values": {
							"Endpoints": {
							"mod_policy": "Admins",
							"value": {
							"addresses": [
							"localhost:123"
							]
							},
							"version": "0"
							},
							"MSP": {
							"mod_policy": "Admins",
							"value": {
							"config": {
							"admins": [
							"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
							],
							"crypto_config": {
							"identity_identifier_hash_function": "SHA256",
							"signature_hash_family": "SHA3"
							},
							"fabric_node_ous": {
							"admin_ou_identifier": {
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
							},
							"client_ou_identifier": {
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
							},
							"enable": false,
							"orderer_ou_identifier": {
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
							},
							"peer_ou_identifier": {
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
							}
							},
							"intermediate_certs": [
							"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
							],
							"name": "MSPID",
							"organizational_unit_identifiers": [
							{
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
							}
							],
							"revocation_list": [
							"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
							],
							"root_certs": [
							"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
							],
							"signing_identity": {
							"private_signer": {
							"key_identifier": "SKI-1",
							"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
							},
							"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
							},
							"tls_intermediate_certs": [
							"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
							],
							"tls_root_certs": [
							"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
							]
							},
							"type": 0
							},
							"version": "0"
							}
						},
						"version": "0"
					}
				},
				"mod_policy": "Admins",
				"policies": {
					"Admins": {
						"mod_policy": "Admins",
						"policy": {
							"type": 3,
							"value": {
							"rule": "MAJORITY",
							"sub_policy": "Admins"
							}
						},
						"version": "0"
					},
					"BlockValidation": {
						"mod_policy": "Admins",
						"policy": {
							"type": 3,
							"value": {
							"rule": "ANY",
							"sub_policy": "Writers"
							}
						},
						"version": "0"
					},
					"Readers": {
						"mod_policy": "Admins",
						"policy": {
							"type": 3,
							"value": {
							"rule": "ANY",
							"sub_policy": "Readers"
							}
						},
						"version": "0"
					},
					"Writers": {
						"mod_policy": "Admins",
						"policy": {
							"type": 3,
							"value": {
							"rule": "ANY",
							"sub_policy": "Writers"
							}
						},
						"version": "0"
					}
				},
				"values": {
					"BatchSize": {
						"mod_policy": "Admins",
						"value": {
							"absolute_max_bytes": 100,
							"max_message_count": 10000,
							"preferred_max_bytes": 100
						},
						"version": "0"
					},
					"BatchTimeout": {
						"mod_policy": "Admins",
						"value": {
							"timeout": "0s"
						},
						"version": "0"
					},
					"Capabilities": {
						"mod_policy": "Admins",
						"value": {
							"capabilities": {
							"V1_3": {}
							}
						},
						"version": "0"
					},
					"ChannelRestrictions": {
						"mod_policy": "Admins",
						"value": {
							"max_count": "0"
						},
						"version": "0"
					},
					"ConsensusType": {
						"mod_policy": "Admins",
						"value": {
							"metadata": {
							"consenters": [
							{
							"client_tls_cert": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
							"host": "host1",
							"port": 123,
							"server_tls_cert": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
							}
							],
							"options": {
							"election_tick": 0,
							"heartbeat_tick": 0,
							"max_inflight_blocks": 0,
							"snapshot_interval_size": 0,
							"tick_interval": ""
							}
							},
							"state": "STATE_NORMAL",
							"type": "etcdraft"
						},
						"version": "0"
					}
				},
				"version": "0"
			}
		},
		"mod_policy": "",
		"policies": {
			"Admins": {
				"mod_policy": "Admins",
				"policy": {
					"type": 3,
					"value": {
						"rule": "MAJORITY",
						"sub_policy": "Admins"
					}
				},
				"version": "0"
			}
		},
		"values": {
			"OrdererAddresses": {
				"mod_policy": "/Channel/Orderer/Admins",
				"value": {
					"addresses": [
						"newhost:345"
					]
				},
				"version": "0"
			}
		},
		"version": "0"
	},
	"sequence": "0"
}
`

	buf := &bytes.Buffer{}
	err = protolator.DeepMarshalJSON(buf, config)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(buf.String()).To(MatchJSON(expectedConfigJSON))
}

func TestAddOrdererOrg(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	ordererGroup, err := newOrdererGroup(baseSoloOrderer())
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	org := Organization{
		Name:     "OrdererOrg2",
		Policies: orgStandardPolicies(),
		OrdererEndpoints: []string{
			"localhost:123",
		},
		MSP: baseMSP(),
	}

	expectedConfigJSON := `
{
	"groups": {},
	"mod_policy": "Admins",
	"policies": {
		"Admins": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "MAJORITY",
					"sub_policy": "Admins"
				}
			},
			"version": "0"
		},
		"Endorsement": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "MAJORITY",
					"sub_policy": "Endorsement"
				}
			},
			"version": "0"
		},
		"Readers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Readers"
				}
			},
			"version": "0"
		},
		"Writers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Writers"
				}
			},
			"version": "0"
		}
	},
	"values": {
		"Endpoints": {
			"mod_policy": "Admins",
			"value": {
				"addresses": [
					"localhost:123"
				]
			},
			"version": "0"
		},
		"MSP": {
			"mod_policy": "Admins",
			"value": {
				"config": {
					"admins": [
						"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
					],
					"crypto_config": {
						"identity_identifier_hash_function": "SHA256",
						"signature_hash_family": "SHA3"
					},
					"fabric_node_ous": {
						"admin_ou_identifier": {
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
						},
						"client_ou_identifier": {
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
						},
						"enable": false,
						"orderer_ou_identifier": {
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
						},
						"peer_ou_identifier": {
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
						}
					},
					"intermediate_certs": [
						"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
					],
					"name": "MSPID",
					"organizational_unit_identifiers": [
						{
							"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
							"organizational_unit_identifier": "OUID"
						}
					],
					"revocation_list": [
						"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
					],
					"root_certs": [
						"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
					],
					"signing_identity": {
						"private_signer": {
							"key_identifier": "SKI-1",
							"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
						},
						"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
					},
					"tls_intermediate_certs": [
						"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
					],
					"tls_root_certs": [
						"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
					]
				},
				"type": 0
			},
			"version": "0"
		}
	},
	"version": "0"
}
`

	err = AddOrdererOrg(config, org)
	gt.Expect(err).NotTo(HaveOccurred())

	actualOrdererConfigGroup := config.ChannelGroup.Groups[OrdererGroupKey].Groups["OrdererOrg2"]
	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &ordererext.DynamicOrdererOrgGroup{ConfigGroup: actualOrdererConfigGroup})
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(buf.String()).To(MatchJSON(expectedConfigJSON))
}

func TestAddOrdererOrgFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	ordererGroup, err := newOrdererGroup(baseSoloOrderer())
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	org := Organization{
		Name: "OrdererOrg2",
	}

	err = AddOrdererOrg(config, org)
	gt.Expect(err).To(MatchError("failed to create orderer org 'OrdererOrg2': no policies defined"))
}

func baseOrdererOfType(ordererType string) Orderer {
	switch ordererType {
	case ConsensusTypeKafka:
		return baseKafkaOrderer()
	case ConsensusTypeEtcdRaft:
		return baseEtcdRaftOrderer()
	default:
		return baseSoloOrderer()
	}
}

func baseSoloOrderer() Orderer {
	return Orderer{
		Policies:    ordererStandardPolicies(),
		OrdererType: ConsensusTypeSolo,
		Organizations: []Organization{
			{
				Name:     "OrdererOrg",
				Policies: orgStandardPolicies(),
				OrdererEndpoints: []string{
					"localhost:123",
				},
				MSP: baseMSP(),
			},
		},
		Capabilities: map[string]bool{
			"V1_3": true,
		},
		BatchSize: BatchSize{
			MaxMessageCount:   100,
			AbsoluteMaxBytes:  100,
			PreferredMaxBytes: 100,
		},
		Addresses: []string{"localhost:123"},
		State:     ConsensusStateNormal,
	}
}

// marshalOrPanic is a helper for proto marshal.
func marshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}

	return data
}

func TestAddOrdererEndpoint(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: {
					Version: 0,
					Groups: map[string]*cb.ConfigGroup{
						"Orderer1Org": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								EndpointsKey: {
									ModPolicy: AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:8050"},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
		Sequence: 0,
	}

	expectedUpdatedConfigJSON := `
{
	"channel_group": {
		"groups": {
			"Orderer": {
				"groups": {
                    "Orderer1Org": {
						"groups": {},
						"mod_policy": "",
						"policies": {},
						"values": {
							"Endpoints": {
								"mod_policy": "Admins",
								"value": {
									"addresses": [
										"127.0.0.1:8050",
										"127.0.0.1:9050"
									]
								},
								"version": "0"
							}
						},
						"version": "0"
					}
				},
                "mod_policy": "",
		        "policies": {},
		        "values": {},
				"version": "0"
			}
		},
		"mod_policy": "",
		"policies": {},
		"values": {},
		"version": "0"
	},
	"sequence": "0"
}
	`
	expectedUpdatedConfig := &cb.Config{}
	err := protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedUpdatedConfigJSON), expectedUpdatedConfig)
	gt.Expect(err).ToNot(HaveOccurred())

	newOrderer1OrgEndpoint := "127.0.0.1:9050"
	err = AddOrdererEndpoint(config, "Orderer1Org", newOrderer1OrgEndpoint)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedUpdatedConfig))
}

func TestAddOrdererEndpointFailure(t *testing.T) {
	t.Parallel()

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: {
					Version: 0,
					Groups: map[string]*cb.ConfigGroup{
						"OrdererOrg": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								EndpointsKey: {
									ModPolicy: AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:7050"},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
		Sequence: 0,
	}

	tests := []struct {
		testName    string
		orgName     string
		endpoint    string
		expectedErr string
	}{
		{
			testName:    "When the org for the orderer does not exist",
			orgName:     "BadOrg",
			endpoint:    "127.0.0.1:7050",
			expectedErr: "orderer org BadOrg does not exist in channel config",
		},
		{
			testName:    "When the orderer endpoint being added already exists in the org",
			orgName:     "OrdererOrg",
			endpoint:    "127.0.0.1:7050",
			expectedErr: "orderer org OrdererOrg already contains endpoint 127.0.0.1:7050",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			err := AddOrdererEndpoint(config, tt.orgName, tt.endpoint)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestRemoveOrdererEndpoint(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: {
					Version: 0,
					Groups: map[string]*cb.ConfigGroup{
						"OrdererOrg": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								EndpointsKey: {
									ModPolicy: AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:7050",
											"127.0.0.1:8050"},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
		Sequence: 0,
	}

	expectedUpdatedConfigJSON := `
{
	"channel_group": {
		"groups": {
			"Orderer": {
				"groups": {
                    "OrdererOrg": {
						"groups": {},
						"mod_policy": "",
						"policies": {},
						"values": {
							"Endpoints": {
								"mod_policy": "Admins",
								"value": {
									"addresses": [
										"127.0.0.1:7050"
									]
								},
								"version": "0"
							}
						},
						"version": "0"
					}
				},
                "mod_policy": "",
		        "policies": {},
		        "values": {},
				"version": "0"
			}
		},
		"mod_policy": "",
		"policies": {},
		"values": {},
		"version": "0"
	},
	"sequence": "0"
}
	`
	expectedUpdatedConfig := &cb.Config{}
	err := protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedUpdatedConfigJSON), expectedUpdatedConfig)
	gt.Expect(err).ToNot(HaveOccurred())

	removedEndpoint := "127.0.0.1:8050"
	err = RemoveOrdererEndpoint(config, "OrdererOrg", removedEndpoint)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedUpdatedConfig))
}

func TestRemoveOrdererEndpointFailure(t *testing.T) {
	t.Parallel()

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: {
					Version: 0,
					Groups: map[string]*cb.ConfigGroup{
						"OrdererOrg": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								EndpointsKey: {
									ModPolicy: AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:7050"},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
		Sequence: 0,
	}

	tests := []struct {
		testName    string
		orgName     string
		endpoint    string
		expectedErr string
	}{
		{
			testName:    "When the org for the orderer does not exist",
			orgName:     "BadOrg",
			endpoint:    "127.0.0.1:8050",
			expectedErr: "orderer org BadOrg does not exist in channel config",
		},
		{
			testName:    "When the endpoint being removed does not exist in the org",
			orgName:     "OrdererOrg",
			endpoint:    "127.0.0.1:8050",
			expectedErr: "could not find endpoint 127.0.0.1:8050 in orderer org OrdererOrg",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			err := RemoveOrdererEndpoint(config, tt.orgName, tt.endpoint)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func baseKafkaOrderer() Orderer {
	orderer := baseSoloOrderer()
	orderer.OrdererType = ConsensusTypeKafka
	orderer.Kafka = Kafka{
		Brokers: []string{"broker1", "broker2"},
	}

	return orderer
}

func baseEtcdRaftOrderer() Orderer {
	orderer := baseSoloOrderer()
	orderer.OrdererType = ConsensusTypeEtcdRaft
	orderer.EtcdRaft = EtcdRaft{
		Consenters: []Consenter{
			{
				Host:          "node-1.example.com",
				Port:          7050,
				ClientTLSCert: &x509.Certificate{},
				ServerTLSCert: &x509.Certificate{},
			},
			{
				Host:          "node-2.example.com",
				Port:          7050,
				ClientTLSCert: &x509.Certificate{},
				ServerTLSCert: &x509.Certificate{},
			},
			{
				Host:          "node-3.example.com",
				Port:          7050,
				ClientTLSCert: &x509.Certificate{},
				ServerTLSCert: &x509.Certificate{},
			},
		},
		Options: EtcdRaftOptions{},
	}

	return orderer
}
