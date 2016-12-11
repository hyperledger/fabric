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
	"github.com/hyperledger/fabric/protos/utils"
)

func (cbs *commonBootstrapper) makeGenesisConfigEnvelope() *cb.ConfigurationEnvelope {
	return utils.MakeConfigurationEnvelope(
		cbs.encodeConsensusType(),
		cbs.encodeBatchSize(),
		cbs.encodeChainCreators(),
		cbs.encodeAcceptAllPolicy(),
		cbs.lockDefaultModificationPolicy(),
	)
}

func (kbs *kafkaBootstrapper) makeGenesisConfigEnvelope() *cb.ConfigurationEnvelope {
	return utils.MakeConfigurationEnvelope(
		kbs.encodeConsensusType(),
		kbs.encodeBatchSize(),
		kbs.encodeKafkaBrokers(),
		kbs.encodeChainCreators(),
		kbs.encodeAcceptAllPolicy(),
		kbs.lockDefaultModificationPolicy(),
	)
}
