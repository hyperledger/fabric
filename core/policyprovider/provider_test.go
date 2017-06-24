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

package policyprovider

import (
	"testing"

	"github.com/hyperledger/fabric/core/policy"
	"github.com/stretchr/testify/assert"
)

func TestGetPolicyChecker(t *testing.T) {
	// Ensure GetPolicyChecker implements policy.PolicyChecker
	var pc policy.PolicyChecker
	pc = GetPolicyChecker()
	assert.NotNil(t, pc)
}

func TestDefaultFactory_NewPolicyChecker(t *testing.T) {
	// Ensure defaultFactory implements policy.PolicyCheckerFactory
	var pcf policy.PolicyCheckerFactory
	pcf = &defaultFactory{}
	var pc policy.PolicyChecker
	// Check we can obtain a new policyChecker
	pc = pcf.NewPolicyChecker()
	assert.NotNil(t, pc)
}
