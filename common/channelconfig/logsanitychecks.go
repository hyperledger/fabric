/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"github.com/hyperledger/fabric/common/policies"
)

func LogSanityChecks(res Resources) {
	pm := res.PolicyManager()
	for _, policyName := range []string{policies.ChannelReaders, policies.ChannelWriters} {
		_, ok := pm.GetPolicy(policyName)
		if !ok {
			logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
		} else {
			logger.Debugf("As expected, current configuration has policy '%s'", policyName)
		}
	}
	if _, ok := pm.Manager([]string{policies.ApplicationPrefix}); ok {
		// Check for default application policies if the application component is defined
		for _, policyName := range []string{
			policies.ChannelApplicationReaders,
			policies.ChannelApplicationWriters,
			policies.ChannelApplicationAdmins} {
			_, ok := pm.GetPolicy(policyName)
			if !ok {
				logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
			} else {
				logger.Debugf("As expected, current configuration has policy '%s'", policyName)
			}
		}
	}
	if _, ok := pm.Manager([]string{policies.OrdererPrefix}); ok {
		for _, policyName := range []string{policies.BlockValidation} {
			_, ok := pm.GetPolicy(policyName)
			if !ok {
				logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
			} else {
				logger.Debugf("As expected, current configuration has policy '%s'", policyName)
			}
		}
	}
}
