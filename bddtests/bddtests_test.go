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

