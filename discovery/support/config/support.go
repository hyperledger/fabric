/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"
	"net"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	mspconstants "github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("discovery.config")

// CurrentConfigBlockGetter enables to fetch the last config block
type CurrentConfigBlockGetter interface {
	// GetCurrConfigBlock returns the current config block for the given channel
	GetCurrConfigBlock(channel string) *common.Block
}

// CurrentConfigBlockGetterFunc enables to fetch the last config block
type CurrentConfigBlockGetterFunc func(channel string) *common.Block

// CurrentConfigBlockGetterFunc enables to fetch the last config block
func (f CurrentConfigBlockGetterFunc) GetCurrConfigBlock(channel string) *common.Block {
	return f(channel)
}

// DiscoverySupport implements support that is used for service discovery
// that is related to configuration
type DiscoverySupport struct {
	CurrentConfigBlockGetter
}

// NewDiscoverySupport creates a new DiscoverySupport
func NewDiscoverySupport(getLastConfigBlock CurrentConfigBlockGetter) *DiscoverySupport {
	return &DiscoverySupport{
		CurrentConfigBlockGetter: getLastConfigBlock,
	}
}

// Config returns the channel's configuration
func (s *DiscoverySupport) Config(channel string) (*discovery.ConfigResult, error) {
	block := s.GetCurrConfigBlock(channel)
	if block == nil {
		return nil, errors.Errorf("could not get last config block for channel %s", channel)
	}
	if block.Data == nil || len(block.Data.Data) == 0 {
		return nil, errors.Errorf("no transactions in block")
	}
	env := &common.Envelope{}
	if err := proto.Unmarshal(block.Data.Data[0], env); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling envelope")
	}
	pl := &common.Payload{}
	if err := proto.Unmarshal(env.Payload, pl); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling payload")
	}
	ce := &common.ConfigEnvelope{}
	if err := proto.Unmarshal(pl.Data, ce); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling config envelope")
	}

	if err := ValidateConfigEnvelope(ce); err != nil {
		return nil, errors.Wrap(err, "config envelope is invalid")
	}

	res := &discovery.ConfigResult{
		Msps:     make(map[string]*msp.FabricMSPConfig),
		Orderers: make(map[string]*discovery.Endpoints),
	}
	ordererGrp := ce.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups
	appGrp := ce.Config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups

	var globalEndpoints []string
	globalOrderers := ce.Config.ChannelGroup.Values[channelconfig.OrdererAddressesKey]
	if globalOrderers != nil {
		ordererAddressesConfig := &common.OrdererAddresses{}
		if err := proto.Unmarshal(globalOrderers.Value, ordererAddressesConfig); err != nil {
			return nil, errors.Wrap(err, "failed unmarshaling orderer addresses")
		}
		globalEndpoints = ordererAddressesConfig.Addresses
	}

	ordererEndpoints, err := computeOrdererEndpoints(ordererGrp, globalEndpoints)
	if err != nil {
		return nil, errors.Wrap(err, "failed computing orderer addresses")
	}
	res.Orderers = ordererEndpoints

	if err := appendMSPConfigs(ordererGrp, appGrp, res.Msps); err != nil {
		return nil, errors.WithStack(err)
	}
	return res, nil

}

func computeOrdererEndpoints(ordererGrp map[string]*common.ConfigGroup, globalOrdererAddresses []string) (map[string]*discovery.Endpoints, error) {
	endpointsByMSPID, err := perOrgEndpointsByMSPID(ordererGrp)
	if err != nil {
		return nil, err
	}

	var somePerOrgEndpoint bool
	for _, perOrgEndpoints := range endpointsByMSPID {
		if len(perOrgEndpoints) > 0 {
			somePerOrgEndpoint = true
			break
		}
	}

	// Per org endpoints take precedence over global endpoints if applicable.
	if somePerOrgEndpoint {
		return computePerOrgEndpoints(endpointsByMSPID), nil
	}

	// Fallback to global endpoints if per org endpoints aren't configured.
	return globalEndpoints(endpointsByMSPID, globalOrdererAddresses)
}

func computePerOrgEndpoints(endpointsByMSPID map[string][]string) map[string]*discovery.Endpoints {
	res := make(map[string]*discovery.Endpoints)

	for mspID, endpoints := range endpointsByMSPID {
		res[mspID] = &discovery.Endpoints{}
		for _, endpoint := range endpoints {
			host, portStr, err := net.SplitHostPort(endpoint)
			if err != nil {
				logger.Warningf("Failed parsing endpoint %s for %s: %v", endpoint, mspID, err)
				continue
			}
			port, err := strconv.ParseInt(portStr, 10, 32)
			if err != nil {
				logger.Warningf("%s for endpoint %s which belongs to %s is not a valid port number: %v", portStr, endpoint, mspID, err)
				continue
			}
			res[mspID].Endpoint = append(res[mspID].Endpoint, &discovery.Endpoint{
				Host: host,
				Port: uint32(port),
			})
		}
	}

	return res
}

func perOrgEndpointsByMSPID(ordererGrp map[string]*common.ConfigGroup) (map[string][]string, error) {
	res := make(map[string][]string)

	for name, group := range ordererGrp {
		mspConfig := &msp.MSPConfig{}
		if err := proto.Unmarshal(group.Values[channelconfig.MSPKey].Value, mspConfig); err != nil {
			return nil, errors.Wrap(err, "failed parsing MSPConfig")
		}
		// Skip non fabric MSPs, as they don't carry useful information for service discovery.
		// An idemix MSP shouldn't appear inside an orderer group, but this isn't a fatal error
		// for the discovery service and we can just ignore it.
		if mspConfig.Type != int32(mspconstants.FABRIC) {
			logger.Error("Orderer group", name, "is not a FABRIC MSP, but is of type", mspConfig.Type)
			continue
		}

		fabricConfig := &msp.FabricMSPConfig{}
		if err := proto.Unmarshal(mspConfig.Config, fabricConfig); err != nil {
			return nil, errors.Wrap(err, "failed marshaling FabricMSPConfig")
		}

		// Initialize an empty MSP to address mapping.
		res[fabricConfig.Name] = nil

		// If the key has a corresponding value, it should unmarshal successfully.
		if perOrgAddresses := group.Values["Endpoints"]; perOrgAddresses != nil {
			ordererEndpoints := &common.OrdererAddresses{}
			if err := proto.Unmarshal(perOrgAddresses.Value, ordererEndpoints); err != nil {
				return nil, errors.Wrap(err, "failed unmarshaling orderer addresses")
			}
			// Override the mapping because this orderer org config contains org-specific endpoints.
			res[fabricConfig.Name] = ordererEndpoints.Addresses
		}
	}

	return res, nil
}

func globalEndpoints(endpointsByMSPID map[string][]string, ordererAddresses []string) (map[string]*discovery.Endpoints, error) {
	res := make(map[string]*discovery.Endpoints)

	for mspID := range endpointsByMSPID {
		res[mspID] = &discovery.Endpoints{}
		for _, endpoint := range ordererAddresses {
			host, portStr, err := net.SplitHostPort(endpoint)
			if err != nil {
				return nil, errors.Errorf("failed parsing orderer endpoint %s", endpoint)
			}
			port, err := strconv.ParseInt(portStr, 10, 32)
			if err != nil {
				return nil, errors.Errorf("%s is not a valid port number", portStr)
			}
			res[mspID].Endpoint = append(res[mspID].Endpoint, &discovery.Endpoint{
				Host: host,
				Port: uint32(port),
			})
		}
	}
	return res, nil
}

func appendMSPConfigs(ordererGrp, appGrp map[string]*common.ConfigGroup, output map[string]*msp.FabricMSPConfig) error {
	for _, group := range []map[string]*common.ConfigGroup{ordererGrp, appGrp} {
		for _, grp := range group {
			mspConfig := &msp.MSPConfig{}
			if err := proto.Unmarshal(grp.Values[channelconfig.MSPKey].Value, mspConfig); err != nil {
				return errors.Wrap(err, "failed parsing MSPConfig")
			}
			// Skip non fabric MSPs, as they don't carry useful information for service discovery
			if mspConfig.Type != int32(mspconstants.FABRIC) {
				continue
			}
			fabricConfig := &msp.FabricMSPConfig{}
			if err := proto.Unmarshal(mspConfig.Config, fabricConfig); err != nil {
				return errors.Wrap(err, "failed marshaling FabricMSPConfig")
			}
			if _, exists := output[fabricConfig.Name]; exists {
				continue
			}
			output[fabricConfig.Name] = fabricConfig
		}
	}

	return nil
}

func ValidateConfigEnvelope(ce *common.ConfigEnvelope) error {
	if ce.Config == nil {
		return fmt.Errorf("field Config is nil")
	}
	if ce.Config.ChannelGroup == nil {
		return fmt.Errorf("field Config.ChannelGroup is nil")
	}
	grps := ce.Config.ChannelGroup.Groups
	if grps == nil {
		return fmt.Errorf("field Config.ChannelGroup.Groups is nil")
	}
	for _, field := range []string{channelconfig.OrdererGroupKey, channelconfig.ApplicationGroupKey} {
		grp, exists := grps[field]
		if !exists {
			return fmt.Errorf("key Config.ChannelGroup.Groups[%s] is missing", field)
		}
		if grp.Groups == nil {
			return fmt.Errorf("key Config.ChannelGroup.Groups[%s].Groups is nil", field)
		}
	}
	if ce.Config.ChannelGroup.Values == nil {
		return fmt.Errorf("field Config.ChannelGroup.Values is nil")
	}
	return nil
}
