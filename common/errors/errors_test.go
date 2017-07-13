/*
 Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.
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

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	name               string
	errorArgs          []string
	messageArgs        []string
	wrappedErrorArgs   []string
	wrappedMessageArgs []string
	generateStack      []bool
}

func TestError(t *testing.T) {
	var tc []testCase

	tc = append(tc,
		testCase{"UnknownErrorWithCallstack", []string{Core, NotFound, "An unknown error occurred."}, nil, nil, nil, []bool{true}},
		testCase{"UnknownError", []string{MSP, Forbidden, "An unknown error occurred."}, nil, nil, nil, []bool{false}},
		testCase{"UnknownErrorWithCallstackAndArg", []string{Ledger, Conflict, "An error occurred: %s"}, []string{"arg1"}, nil, nil, []bool{true}},
		testCase{"UnknownErrorWithArg", []string{SystemChaincode, Internal, "An error occurred: %s"}, []string{"arg1"}, nil, nil, []bool{false}},
		testCase{"CallStackErrorWrappedCallstackError", []string{BCCSP, NotImplemented, "An unknown error occurred."}, nil, []string{Peer, UpgradeRequired, "An unknown error occurred."}, nil, []bool{true, true}},
		testCase{"ErrorWrappedError", []string{Common, Unavailable, "An unknown error occurred."}, nil, []string{SystemChaincode, Gone, "An unknown error occurred."}, nil, []bool{false, false}},
		testCase{"CallStackErrorWrappedError", []string{Event, Timeout, "An unknown error occurred."}, nil, []string{Orderer, NetworkIO, "An unknown error occurred."}, nil, []bool{true, false}},
		testCase{"ErrorWrappedCallStackError", []string{Orderer, UnprocessableEntity, "An unknown error occurred."}, nil, []string{"UNK", "404", "An unknown error occurred."}, nil, []bool{false, true}},
		testCase{"ErrorWrappedStandardError", []string{DeliveryService, Unavailable, "An unknown error occurred."}, nil, []string{"grpc timed out: %s"}, []string{"failed to connect to server"}, []bool{false, true}},
	)

	assert := assert.New(t)

	for i := 0; i < len(tc); i++ {
		t.Run(tc[i].name, func(t *testing.T) {
			var err, wrappedErr CallStackError
			var wrappedStandardError error

			// generate callstack
			if tc[i].generateStack[0] {
				if tc[i].messageArgs != nil {
					err = ErrorWithCallstack(tc[i].errorArgs[0], tc[i].errorArgs[1], tc[i].errorArgs[2], tc[i].messageArgs)
				} else {
					err = ErrorWithCallstack(tc[i].errorArgs[0], tc[i].errorArgs[1], tc[i].errorArgs[2])
				}
				assert.NotEqual("", err.GetStack(), "Test case '%s' failed", tc[i].name)
				assert.Contains(err.Error(), "github.com/hyperledger/fabric/common/errors", "Test case '%s' failed", tc[i].name)
			} else {
				if tc[i].messageArgs != nil {
					err = Error(tc[i].errorArgs[0], tc[i].errorArgs[1], tc[i].errorArgs[2], tc[i].messageArgs)
				} else {
					err = Error(tc[i].errorArgs[0], tc[i].errorArgs[1], tc[i].errorArgs[2])
				}
				assert.Equal("", err.GetStack(), "Test case '%s' failed", tc[i].name)
				assert.NotContains(err.Error(), "github.com/hyperledger/fabric/common/errors", "Test case '%s' failed", tc[i].name)
			}
			assert.Equal(tc[i].errorArgs[0], err.GetComponentCode(), "Test case '%s' failed", tc[i].name)
			assert.Equal(tc[i].errorArgs[1], err.GetReasonCode(), "Test case '%s' failed", tc[i].name)
			if tc[i].messageArgs != nil {
				assert.Contains(err.Error(), fmt.Sprintf(tc[i].errorArgs[2], tc[i].messageArgs), "Test case '%s' failed", tc[i].name)
			} else {
				assert.Contains(err.Error(), tc[i].errorArgs[2], "Test case '%s' failed", tc[i].name)
			}
			assert.NotContains(err.Message(), "github.com/hyperledger/fabric/common/errors", "Test case '%s' failed", tc[i].name)

			if tc[i].wrappedErrorArgs != nil {
				if len(tc[i].wrappedErrorArgs) == 3 {
					if tc[i].generateStack[1] {
						if tc[i].wrappedMessageArgs != nil {
							wrappedErr = ErrorWithCallstack(tc[i].wrappedErrorArgs[0], tc[i].wrappedErrorArgs[1], tc[i].wrappedErrorArgs[2], tc[i].wrappedMessageArgs)
						} else {
							wrappedErr = ErrorWithCallstack(tc[i].wrappedErrorArgs[0], tc[i].wrappedErrorArgs[1], tc[i].wrappedErrorArgs[2])
						}
						assert.NotEqual("", wrappedErr.GetStack(), "Test case '%s' failed", tc[i].name)
						assert.Contains(wrappedErr.Error(), "github.com/hyperledger/fabric/common/errors", "Test case '%s' failed", tc[i].name)
					} else {
						if tc[i].wrappedMessageArgs != nil {
							wrappedErr = Error(tc[i].wrappedErrorArgs[0], tc[i].wrappedErrorArgs[1], tc[i].wrappedErrorArgs[2], tc[i].wrappedMessageArgs)
						} else {
							wrappedErr = Error(tc[i].wrappedErrorArgs[0], tc[i].wrappedErrorArgs[1], tc[i].wrappedErrorArgs[2])
						}
						assert.Equal("", wrappedErr.GetStack(), "Test case '%s' failed", tc[i].name)
						assert.NotContains(wrappedErr.Error(), "github.com/hyperledger/fabric/common/errors", "Test case '%s' failed", tc[i].name)
					}
					assert.Equal(tc[i].wrappedErrorArgs[0], wrappedErr.GetComponentCode(), "Test case '%s' failed", tc[i].name)
					assert.Equal(tc[i].wrappedErrorArgs[1], wrappedErr.GetReasonCode(), "Test case '%s' failed", tc[i].name)
					err = err.WrapError(wrappedErr)
					if tc[i].wrappedMessageArgs != nil {
						assert.Contains(err.Error(), fmt.Sprintf(tc[i].wrappedErrorArgs[2], tc[i].wrappedMessageArgs), "Test case '%s' failed", tc[i].name)
					} else {
						assert.Contains(err.Error(), tc[i].wrappedErrorArgs[2], "Test case '%s' failed", tc[i].name)
					}
				} else {
					wrappedStandardError = fmt.Errorf(tc[i].wrappedErrorArgs[0], tc[i].wrappedMessageArgs)
					err = err.WrapError(wrappedStandardError)
					assert.Contains(err.Error(), fmt.Sprintf(tc[i].wrappedErrorArgs[0], tc[i].wrappedMessageArgs), "Test case '%s' failed", tc[i].name)

				}
				assert.Contains(err.Error(), "Caused by:", "Test case '%s' failed", tc[i].name)
				assert.NotContains(err.Message(), "github.com/hyperledger/fabric/common/errors", "Test case '%s' failed", tc[i].name)
			}
		})
	}
}

func TestIsValidComponentOrReasonCode(t *testing.T) {
	validComponents := []string{"LGR", "CHA", "PER", "xyz", "aBc"}
	for _, component := range validComponents {
		if ok := isValidComponentOrReasonCode(component, componentPattern); !ok {
			t.FailNow()
		}
	}

	validReasons := []string{"404", "500", "999"}
	for _, reason := range validReasons {
		if ok := isValidComponentOrReasonCode(reason, reasonPattern); !ok {
			t.FailNow()
		}
	}

	invalidComponents := []string{"LEDG", "CH", "P3R", "123", ""}
	for _, component := range invalidComponents {
		if ok := isValidComponentOrReasonCode(component, componentPattern); ok {
			t.FailNow()
		}
	}

	invalidReasons := []string{"4045", "E12", "ZZZ", "1", ""}
	for _, reason := range invalidReasons {
		if ok := isValidComponentOrReasonCode(reason, reasonPattern); ok {
			t.FailNow()
		}
	}

}
