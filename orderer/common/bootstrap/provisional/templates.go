/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package provisional

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

func (cbs *commonBootstrapper) makeOrdererSystemChainConfig() []*cb.ConfigurationItem {
	return []*cb.ConfigurationItem{cbs.encodeChainCreators()}
}

func (cbs *commonBootstrapper) TemplateItems() []*cb.ConfigurationItem {
	return []*cb.ConfigurationItem{
		cbs.encodeConsensusType(),
		cbs.encodeBatchSize(),
		cbs.encodeBatchTimeout(),
		cbs.encodeAcceptAllPolicy(),
		cbs.encodeIngressPolicy(),
		cbs.encodeEgressPolicy(),
		cbs.lockDefaultModificationPolicy(),
	}
}

func (kbs *kafkaBootstrapper) TemplateItems() []*cb.ConfigurationItem {
	return append(kbs.commonBootstrapper.TemplateItems(), kbs.encodeKafkaBrokers())
}
