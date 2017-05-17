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

package config

import (
	"testing"

	"github.com/hyperledger/fabric/common/config/msp"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestRoot(t *testing.T) {
	r := NewRoot(&msp.MSPConfigHandler{})
	appGroup := &ApplicationGroup{}
	ordererGroup := &OrdererGroup{}
	consortiumsGroup := &ConsortiumsGroup{}
	r.Channel().appConfig = appGroup
	r.Channel().ordererConfig = ordererGroup
	r.Channel().consortiumsConfig = consortiumsGroup

	assert.Equal(t, appGroup, r.Application(), "Failed to return correct application group")
	assert.Equal(t, ordererGroup, r.Orderer(), "Failed to return correct orderer group")
	assert.Equal(t, consortiumsGroup, r.Consortiums(), "Failed to return correct consortiums group")
}

func TestBeginBadRoot(t *testing.T) {
	r := NewRoot(&msp.MSPConfigHandler{})

	_, _, err := r.BeginValueProposals(t, []string{ChannelGroupKey, ChannelGroupKey})
	assert.Error(t, err, "Only one root element allowed")

	_, _, err = r.BeginValueProposals(t, []string{"foo"})
	assert.Error(t, err, "Non %s group not allowed", ChannelGroupKey)
}

func TestProposeValue(t *testing.T) {
	r := NewRoot(msp.NewMSPConfigHandler())

	vd, _, err := r.BeginValueProposals(t, []string{ChannelGroupKey})
	assert.NoError(t, err)

	_, err = vd.Deserialize("foo", nil)
	assert.Error(t, err, "ProposeValue should return error")
}
