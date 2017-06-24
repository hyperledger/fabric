/*
 Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package clilogging

import (
	"testing"

	"github.com/hyperledger/fabric/peer/common"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

type testCase struct {
	name      string
	args      []string
	shouldErr bool
}

func initLoggingTest(command string) (*cobra.Command, *LoggingCmdFactory) {
	adminClient := common.GetMockAdminClient(nil)
	mockCF := &LoggingCmdFactory{
		AdminClient: adminClient,
	}
	var cmd *cobra.Command
	if command == "getlevel" {
		cmd = getLevelCmd(mockCF)
	} else if command == "setlevel" {
		cmd = setLevelCmd(mockCF)
	} else if command == "revertlevels" {
		cmd = revertLevelsCmd(mockCF)
	} else {
		// should only happen when there's a typo in a test case below
	}
	return cmd, mockCF
}

func runTests(t *testing.T, command string, tc []testCase) {
	cmd, _ := initLoggingTest(command)
	assert := assert.New(t)
	for i := 0; i < len(tc); i++ {
		t.Run(tc[i].name, func(t *testing.T) {
			cmd.SetArgs(tc[i].args)
			err := cmd.Execute()
			if tc[i].shouldErr {
				assert.NotNil(err)
			}
			if !tc[i].shouldErr {
				assert.Nil(err)
			}
		})
	}
}

// TestGetLevel tests getlevel with various parameters
func TestGetLevel(t *testing.T) {
	var tc []testCase
	tc = append(tc,
		testCase{"NoParameters", []string{}, true},
		testCase{"Valid", []string{"peer"}, false},
	)
	runTests(t, "getlevel", tc)
}

// TestStLevel tests setlevel with various parameters
func TestSetLevel(t *testing.T) {
	var tc []testCase
	tc = append(tc,
		testCase{"NoParameters", []string{}, true},
		testCase{"OneParameter", []string{"peer"}, true},
		testCase{"Valid", []string{"peer", "warning"}, false},
		testCase{"InvalidLevel", []string{"peer", "invalidlevel"}, true},
	)
	runTests(t, "setlevel", tc)
}

// TestRevertLevels tests revertlevels with various parameters
func TestRevertLevels(t *testing.T) {
	var tc []testCase
	tc = append(tc,
		testCase{"Valid", []string{}, false},
		testCase{"ExtraParameter", []string{"peer"}, true},
	)
	runTests(t, "revertlevels", tc)
}
