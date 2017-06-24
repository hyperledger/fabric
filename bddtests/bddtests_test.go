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

import "github.com/DATA-DOG/godog"

func FeatureContext(s *godog.Suite) {
	bddCtx := &BDDContext{godogSuite: s, users: make(map[string]*UserRegistration), grpcClientPort: 7051}

	s.BeforeScenario(bddCtx.beforeScenario)
	s.AfterScenario(bddCtx.afterScenarioDecompose)

	FeatureContextBootstrap(bddCtx, s)

	s.Step(`^we compose "([^"]*)"$`, bddCtx.weCompose)
	s.Step(`^requesting "([^"]*)" from "([^"]*)"$`, bddCtx.requestingFrom)
	s.Step(`^I should get a JSON response with array "([^"]*)" contains "([^"]*)" elements$`, bddCtx.iShouldGetAJSONResponseWithArrayContainsElements)
	s.Step(`^I wait "([^"]*)" seconds$`, bddCtx.iWaitSeconds)
	s.Step(`^I register with CA supplying username "([^"]*)" and secret "([^"]*)" on peers:$`, bddCtx.iRegisterWithCASupplyingUsernameAndSecretOnPeers)
	s.Step(`^user "([^"]*)" creates a chaincode spec "([^"]*)" of type "([^"]*)" for chaincode "([^"]*)" with args$`, bddCtx.userCreatesAChaincodeSpecOfTypeForChaincodeWithArgs)

	s.Step(`^user "([^"]*)" creates a deployment spec "([^"]*)" using chaincode spec "([^"]*)" and devops on peer "([^"]*)"$`, bddCtx.userCreatesADeploymentSpecUsingChaincodeSpecAndDevopsOnPeer)
	//s.Step(`^user "([^"]*)" creates a deployment spec "([^"]*)" using chaincode spec "([^"]*)"$`, bddCtx.userCreatesADeploymentSpecUsingChaincodeSpec)

	s.Step(`^user "([^"]*)" creates a deployment proposal "([^"]*)" using chaincode deployment spec "([^"]*)"$`, bddCtx.userCreatesADeploymentProposalUsingChaincodeDeploymentSpec)
	s.Step(`^user "([^"]*)" sends proposal "([^"]*)" to endorsers with timeout of "([^"]*)" seconds:$`, bddCtx.userSendsProposalToEndorsersWithTimeoutOfSeconds)
	s.Step(`^user "([^"]*)" stores their last result as "([^"]*)"$`, bddCtx.userStoresTheirLastResultAs)
	s.Step(`^user "([^"]*)" expects proposal responses "([^"]*)" with status "([^"]*)" from endorsers:$`, bddCtx.userExpectsProposalResponsesWithStatusFromEndorsers)
	s.Step(`^user "([^"]*)" sets ESCC to "([^"]*)" for chaincode spec "([^"]*)"$`, bddCtx.userSetsESCCToForChaincodeSpec)
	s.Step(`^user "([^"]*)" sets VSCC to "([^"]*)" for chaincode spec "([^"]*)"$`, bddCtx.userSetsVSCCToForChaincodeSpec)
}
