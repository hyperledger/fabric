/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package config

const RootGroupKey = "Channel"

// TODO, move this code elsewhere
/*
	if ppr.initializationLogSuppression {
		ppr.initializationLogSuppression = false
	}

	pm := ppr.policyManager
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
*/
