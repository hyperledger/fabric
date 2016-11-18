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

package errors

// Component codes - should be set to the same key in the errorCodes json below
const (
	Utility ComponentCode = "Utility"
	Logging ComponentCode = "Logging"
	Peer    ComponentCode = "Peer"
)

// Reason codes - should be set to the same key in the errorCodes json below
const (
	// Utility errors - placeholders
	UtilityUnknownError ReasonCode = "UtilityUnknownError"
	UtilityErrorWithArg ReasonCode = "UtilityErrorWithArg"

	// Logging errors
	LoggingUnknownError        ReasonCode = "LoggingUnknownError"
	LoggingErrorWithArg        ReasonCode = "LoggingErrorWithArg"
	LoggingNoParameters        ReasonCode = "LoggingNoParameters"
	LoggingNoLogLevelParameter ReasonCode = "LoggingNoLogLevelParameter"
	LoggingInvalidLogLevel     ReasonCode = "LoggingInvalidLogLevel"

	// Peer errors
	PeerConnectionError ReasonCode = "PeerConnectionError"
)

// To use this file to define a new component code or reason code, please follow
// these steps:
// 1. Add the new component code below the last component code or add the
// 		new reason code below the last reason code for the applicable component
//	  in the json formatted string below
// 2. Define the message in English using "en" as the key
// 3. Add a constant above for each new component/reason code making sure it
// 		matches the key used in the json formatted string below
// 4. Import "github.com/hyperledger/fabric/core/errors" wherever the error
//		will be created.
// 5. Reference the component and reason codes in the call to
// 		errors.ErrorWithCallstack as follows:
//			err = errors.ErrorWithCallstack(errors.Logging, errors.LoggingNoParameters)
// 6. Any code that receives this error will automatically have the callstack
//		appended if CORE_LOGGING_ERROR is set to DEBUG in peer/core.yaml, at the
//		command line when the peer is started, or if the error module is set
//		dynamically using the CLI call "peer logging setlevel error debug"
// 6. For an example in context, see "peer/clilogging/common.go", which creates
//		a number of callstack errors, and "peer/clilogging/setlevel.go", which
//		receives the errors
const errorMapping string = `
{"Utility" :
  {"UtilityUnknownError" :
    {"en": "An unknown error occurred."},
   "UtilityErrorWithArg":
    {"en": "An error occurred: %s"}
  },
 "Logging":
  {"LoggingUnknownError" :
    {"en": "A logging error occurred."},
   "LoggingErrorWithArg":
    {"en": "A logging error occurred: %s"},
   "LoggingNoParameters":
    {"en": "No parameters provided."},
   "LoggingNoLogLevelParameter":
    {"en": "No log level provided."},
   "LoggingInvalidLogLevel":
    {"en": "Invalid log level provided - %s"}
  },
 "Peer" :
  {"PeerConnectionError" :
    {"en": "Error trying to connect to local peer: %s"}
  }
}`
