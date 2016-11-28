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

package bddtests

import (
	"github.com/DATA-DOG/godog"
	"github.com/DATA-DOG/godog/gherkin"
)

func (b *BDDContext) theOrdererNetworkHasOrganizations(arg1 *gherkin.DataTable) error {
	return godog.ErrPending
}

// FeatureContextBootstrap setup the FeatureContext  for bootstrap steps
func FeatureContextBootstrap(bddCtx *BDDContext, s *godog.Suite) {

	s.Step(`^the orderer network has organizations:$`, bddCtx.theOrdererNetworkHasOrganizations)
	//s.Step(`^user requests role of orderer admin by creating a key and csr for orderer and acquires signed certificate from organization:$`, userRequestsRoleOfOrdererAdminByCreatingAKeyAndCsrForOrdererAndAcquiresSignedCertificateFromOrganization)
	//s.Step(`^the peer network has organizations:$`, thePeerNetworkHasOrganizations)
	//s.Step(`^a chainBootstrapAdmin is identified and given access to all public certificates and orderer node info$`, aChainBootstrapAdminIsIdentifiedAndGivenAccessToAllPublicCertificatesAndOrdererNodeInfo)
	//s.Step(`^the chainBootstrapAdmin creates the genesis block for chain \'chain(\d+)\' for network config policy \'unanimous\' and consensus \'solo\' and peer organizations:$`, theChainBootstrapAdminCreatesTheGenesisBlockForChainChainForNetworkConfigPolicyUnanimousAndConsensusSoloAndPeerOrganizations)
	//s.Step(`^the orderer admins inspect and approve the genesis block for chain \'chain(\d+)\'$`, theOrdererAdminsInspectAndApproveTheGenesisBlockForChainChain)
	//s.Step(`^the orderer admins use the genesis block for chain \'chain(\d+)\' to configure orderers$`, theOrdererAdminsUseTheGenesisBlockForChainChainToConfigureOrderers)
	//s.Step(`^user requests role of peer admin by creating a key and csr for peer and acquires signed certificate from organization:$`, userRequestsRoleOfPeerAdminByCreatingAKeyAndCsrForPeerAndAcquiresSignedCertificateFromOrganization)
	//s.Step(`^peer admins get the genesis block for chain \'chain(\d+)\' from chainBoostrapAdmin$`, peerAdminsGetTheGenesisBlockForChainChainFromChainBoostrapAdmin)
	//s.Step(`^the peer admins inspect and approve the genesis block for chain \'chain(\d+)\'$`, thePeerAdminsInspectAndApproveTheGenesisBlockForChainChain)
	//s.Step(`^the peer admins use the genesis block for chain \'chain(\d+)\' to configure peers$`, thePeerAdminsUseTheGenesisBlockForChainChainToConfigurePeers)
	//s.Step(`^user "([^"]*)" registers with peer organization "([^"]*)"$`, userRegistersWithPeerOrganization)
}
