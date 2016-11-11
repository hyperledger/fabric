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
	pb "github.com/hyperledger/fabric/protos/peer"
)

// KeyedProposalResponse the response for an endorsement for internal usage in maps
type KeyedProposalResponse struct {
	endorser string
	proposal *pb.ProposalResponse
	err      error
}

//KeyedProposalResponseMap map of composeServices to KeyedProposalResponse
type KeyedProposalResponseMap map[string]*KeyedProposalResponse
