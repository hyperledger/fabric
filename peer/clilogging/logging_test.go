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

import "testing"

// TestGetLevelEmptyParams tests the parameter checking for getlevel, which
// should return an error when no parameters are provided
func TestGetLevelEmptyParams(t *testing.T) {
	var args []string

	err := checkLoggingCmdParams(getLevelCmd(), args)

	if err == nil {
		t.FailNow()
	}
}

// TestGetLevel tests the parameter checking for getlevel, which should
// should return a nil error when one (or more) parameters are provided
func TestGetLevel(t *testing.T) {
	args := make([]string, 1)
	args[0] = "peer"

	err := checkLoggingCmdParams(getLevelCmd(), args)

	if err != nil {
		t.FailNow()
	}
}

// TestSetLevelEmptyParams tests the parameter checking for setlevel, which
// should return an error when no parameters are provided
func TestSetLevelEmptyParams(t *testing.T) {
	var args []string

	err := checkLoggingCmdParams(setLevelCmd(), args)

	if err == nil {
		t.FailNow()
	}
}

// TestSetLevelEmptyParams tests the parameter checking for setlevel, which
// should return an error when only one parameter is provided
func TestSetLevelOneParam(t *testing.T) {
	args := make([]string, 1)
	args[0] = "peer"

	err := checkLoggingCmdParams(setLevelCmd(), args)

	if err == nil {
		t.FailNow()
	}
}

// TestSetLevelEmptyParams tests the parameter checking for setlevel, which
// should return an error when an invalid log level is provided
func TestSetLevelInvalid(t *testing.T) {
	args := make([]string, 2)
	args[0] = "peer"
	args[1] = "invalidlevel"

	err := checkLoggingCmdParams(setLevelCmd(), args)

	if err == nil {
		t.FailNow()
	}
}

// TestSetLevelEmptyParams tests the parameter checking for setlevel, which
// should return a nil error when two parameters, the second of which is a
// valid log level, are provided
func TestSetLevel(t *testing.T) {
	args := make([]string, 2)
	args[0] = "peer"
	args[1] = "debug"

	err := checkLoggingCmdParams(setLevelCmd(), args)

	if err != nil {
		t.FailNow()
	}
}
