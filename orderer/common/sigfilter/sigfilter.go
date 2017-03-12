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

package sigfilter

import (
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/common/sigfilter")

type sigFilter struct {
	policySource  string
	policyManager policies.Manager
}

// New creates a new signature filter, at every evaluation, the policySource is called
// just before evaluation to get the policy name to use when evaluating the filter
// In general, both the policy name and the policy itself are mutable, this is why
// not only the policy is retrieved at each invocation, but also the name of which
// policy to retrieve
func New(policySource string, policyManager policies.Manager) filter.Rule {
	return &sigFilter{
		policySource:  policySource,
		policyManager: policyManager,
	}
}

// Apply applies the policy given, resulting in Reject or Forward, never Accept and always with nil Committer
func (sf *sigFilter) Apply(message *cb.Envelope) (filter.Action, filter.Committer) {
	signedData, err := message.AsSignedData()

	if err != nil {
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Rejecting because of err: %s", err)
		}
		return filter.Reject, nil
	}

	policy, ok := sf.policyManager.GetPolicy(sf.policySource)
	if !ok {
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Could not find policy %s", sf.policySource)
		}
		return filter.Reject, nil
	}

	err = policy.Evaluate(signedData)

	if err == nil {
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Forwarding validly signed message for policy %s", policy)
		}
		return filter.Forward, nil
	}

	return filter.Reject, nil
}
