/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"
	"net"
	"strconv"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/common/channelconfig"
	mspconstants "github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

var logger = flogging.MustGetLogger("discovery.config")

// CurrentConfigGetter enables to fetch the last channel config
type CurrentConfigGetter interface {
	// GetCurrConfig returns the current channel config for the given channel
	GetCurrConfig(channel string) *common.Config
}

// CurrentConfigGetterFunc enables to fetch the last channel config
type CurrentConfigGetterFunc func(channel string) *common.Config

// CurrentConfigGetterFunc enables to fetch the last channel config
func (f CurrentConfigGetterFunc) GetCurrConfig(channel string) *common.Config {
	return f(channel)
}

// DiscoverySupport implements support that is used for service discovery
// that is related to configuration
type DiscoverySupport struct {
	CurrentConfigGetter
}

// NewDiscoverySupport creates a new DiscoverySupport
func NewDiscoverySupport(getLastConfig CurrentConfigGetter) *DiscoverySupport {
	return &DiscoverySupport{
		CurrentConfigGetter: getLastConfig,
	}
}

// Config returns the channel's configuration
func (s *DiscoverySupport) Config(channel string) (*discovery.ConfigResult, error) {
	config := s.GetCurrConfig(channel)
	if config == nil {
		return nil, errors.Errorf("could not get last config for channel %s", channel)
	}

	if err := ValidateConfig(config); err != nil {
		return nil, errors.WithMessage(err, "config is invalid")
	}

	res := &discovery.ConfigResult{
		Msps:     make(map[string]*msp.FabricMSPConfig),
		Orderers: make(map[string]*discovery.Endpoints),
	}
	ordererGrp := config.GetChannelGroup().GetGroups()[channelconfig.OrdererGroupKey].GetGroups()
	appGrp := config.GetChannelGroup().GetGroups()[channelconfig.ApplicationGroupKey].GetGroups()

	var globalEndpoints []string
	globalOrderers := config.GetChannelGroup().GetValues()[channelconfig.OrdererAddressesKey]
	if globalOrderers != nil {
		ordererAddressesConfig := &common.OrdererAddresses{}
		if err := proto.Unmarshal(globalOrderers.GetValue(), ordererAddressesConfig); err != nil {
			return nil, errors.Wrap(err, "failed unmarshalling orderer addresses")
		}
		globalEndpoints = ordererAddressesConfig.GetAddresses()
	}

	ordererEndpoints, err := computeOrdererEndpoints(ordererGrp, globalEndpoints)
	if err != nil {
		return nil, errors.Wrap(err, "failed computing orderer addresses")
	}
	res.Orderers = ordererEndpoints

	if err := appendMSPConfigs(ordererGrp, appGrp, res.GetMsps()); err != nil {
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
		if err := proto.Unmarshal(group.GetValues()[channelconfig.MSPKey].GetValue(), mspConfig); err != nil {
			return nil, errors.Wrap(err, "failed parsing MSPConfig")
		}
		// Skip non fabric MSPs, as they don't carry useful information for service discovery.
		// An idemix MSP shouldn't appear inside an orderer group, but this isn't a fatal error
		// for the discovery service and we can just ignore it.
		if mspConfig.GetType() != int32(mspconstants.FABRIC) {
			logger.Error("Orderer group", name, "is not a FABRIC MSP, but is of type", mspConfig.GetType())
			continue
		}

		fabricConfig := &msp.FabricMSPConfig{}
		if err := proto.Unmarshal(mspConfig.GetConfig(), fabricConfig); err != nil {
			return nil, errors.Wrap(err, "failed marshaling FabricMSPConfig")
		}

		// Initialize an empty MSP to address mapping.
		res[fabricConfig.GetName()] = nil

		// If the key has a corresponding value, it should unmarshal successfully.
		if perOrgAddresses := group.GetValues()[channelconfig.EndpointsKey]; perOrgAddresses != nil {
			ordererEndpoints := &common.OrdererAddresses{}
			if err := proto.Unmarshal(perOrgAddresses.GetValue(), ordererEndpoints); err != nil {
				return nil, errors.Wrap(err, "failed unmarshalling orderer addresses")
			}
			// Override the mapping because this orderer org config contains org-specific endpoints.
			res[fabricConfig.GetName()] = ordererEndpoints.GetAddresses()
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
			if err := proto.Unmarshal(grp.GetValues()[channelconfig.MSPKey].GetValue(), mspConfig); err != nil {
				return errors.Wrap(err, "failed parsing MSPConfig")
			}
			// Skip non fabric MSPs, as they don't carry useful information for service discovery
			if mspConfig.GetType() != int32(mspconstants.FABRIC) {
				continue
			}
			fabricConfig := &msp.FabricMSPConfig{}
			if err := proto.Unmarshal(mspConfig.GetConfig(), fabricConfig); err != nil {
				return errors.Wrap(err, "failed marshaling FabricMSPConfig")
			}
			if _, exists := output[fabricConfig.GetName()]; exists {
				continue
			}
			output[fabricConfig.GetName()] = fabricConfig
		}
	}

	return nil
}

func ValidateConfig(c *common.Config) error {
	if c.GetChannelGroup() == nil {
		return errors.New("field Config.ChannelGroup is nil")
	}
	grps := c.GetChannelGroup().GetGroups()
	if grps == nil {
		return errors.New("field Config.ChannelGroup.Groups is nil")
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
	if c.ChannelGroup.Values == nil {
		return errors.New("field Config.ChannelGroup.Values is nil")
	}
	return nil
}
