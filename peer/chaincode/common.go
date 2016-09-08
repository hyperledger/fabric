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

package chaincode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/hyperledger/fabric/core"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/peer/util"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

func getChaincodeSpecification(cmd *cobra.Command) (*pb.ChaincodeSpec, error) {
	spec := &pb.ChaincodeSpec{}
	if err := checkChaincodeCmdParams(cmd); err != nil {
		return spec, err
	}

	// Build the spec
	input := &pb.ChaincodeInput{}
	if err := json.Unmarshal([]byte(chaincodeCtorJSON), &input); err != nil {
		return spec, fmt.Errorf("Chaincode argument error: %s", err)
	}

	var attributes []string
	if err := json.Unmarshal([]byte(chaincodeAttributesJSON), &attributes); err != nil {
		return spec, fmt.Errorf("Chaincode argument error: %s", err)
	}

	chaincodeLang = strings.ToUpper(chaincodeLang)
	spec = &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[chaincodeLang]),
		ChaincodeID: &pb.ChaincodeID{Path: chaincodePath, Name: chaincodeName},
		CtorMsg:     input,
		Attributes:  attributes,
	}
	// If security is enabled, add client login token
	if core.SecurityEnabled() {
		if chaincodeUsr == common.UndefinedParamValue {
			return spec, errors.New("Must supply username for chaincode when security is enabled")
		}

		// Retrieve the CLI data storage path
		// Returns /var/openchain/production/client/
		localStore := util.GetCliFilePath()

		// Check if the user is logged in before sending transaction
		if _, err := os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			logger.Infof("Local user '%s' is already logged in. Retrieving login token.\n", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				panic(fmt.Errorf("Fatal error when reading client login token: %s\n", err))
			}

			// Add the login token to the chaincodeSpec
			spec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				logger.Info("Set confidentiality level to CONFIDENTIAL.\n")
				spec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				return spec, fmt.Errorf("User '%s' not logged in. Use the 'peer network login' command to obtain a security token.", chaincodeUsr)
			}
			// Unexpected error
			panic(fmt.Errorf("Fatal error when checking for client login token: %s\n", err))
		}
	} else {
		if chaincodeUsr != common.UndefinedParamValue {
			logger.Warning("Username supplied but security is disabled.")
		}
		if viper.GetBool("security.privacy") {
			panic(errors.New("Privacy cannot be enabled as requested because security is disabled"))
		}
	}
	return spec, nil
}

// chaincodeInvokeOrQuery invokes or queries the chaincode. If successful, the
// INVOKE form prints the transaction ID on STDOUT, and the QUERY form prints
// the query result on STDOUT. A command-line flag (-r, --raw) determines
// whether the query result is output as raw bytes, or as a printable string.
// The printable form is optionally (-x, --hex) a hexadecimal representation
// of the query response. If the query response is NIL, nothing is output.
func chaincodeInvokeOrQuery(cmd *cobra.Command, args []string, invoke bool) (err error) {
	spec, err := getChaincodeSpecification(cmd)
	if err != nil {
		return err
	}

	devopsClient, err := common.GetDevopsClient(cmd)
	if err != nil {
		return fmt.Errorf("Error building %s: %s", chainFuncName, err)
	}

	// Build the ChaincodeInvocationSpec message
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}
	if customIDGenAlg != common.UndefinedParamValue {
		invocation.IdGenerationAlg = customIDGenAlg
	}

	var resp *pb.Response
	if invoke {
		resp, err = devopsClient.Invoke(context.Background(), invocation)
	} else {
		resp, err = devopsClient.Query(context.Background(), invocation)
	}

	if err != nil {
		if invoke {
			err = fmt.Errorf("Error invoking %s: %s\n", chainFuncName, err)
		} else {
			err = fmt.Errorf("Error querying %s: %s\n", chainFuncName, err)
		}
		return
	}
	if invoke {
		transactionID := string(resp.Msg)
		logger.Infof("Successfully invoked transaction: %s(%s)", invocation, transactionID)
	} else {
		logger.Infof("Successfully queried transaction: %s", invocation)
		if resp != nil {
			if chaincodeQueryRaw {
				if chaincodeQueryHex {
					err = errors.New("Options --raw (-r) and --hex (-x) are not compatible\n")
					return
				}
				fmt.Print("Query Result (Raw): ")
				os.Stdout.Write(resp.Msg)
			} else {
				if chaincodeQueryHex {
					fmt.Printf("Query Result: %x\n", resp.Msg)
				} else {
					fmt.Printf("Query Result: %s\n", string(resp.Msg))
				}
			}
		}
	}
	return nil
}

func checkChaincodeCmdParams(cmd *cobra.Command) error {

	if chaincodeName == common.UndefinedParamValue {
		if chaincodePath == common.UndefinedParamValue {
			return fmt.Errorf("Must supply value for %s path parameter.\n", chainFuncName)
		}
	}

	// Check that non-empty chaincode parameters contain only Args as a key.
	// Type checking is done later when the JSON is actually unmarshaled
	// into a pb.ChaincodeInput. To better understand what's going
	// on here with JSON parsing see http://blog.golang.org/json-and-go -
	// Generic JSON with interface{}
	if chaincodeCtorJSON != "{}" {
		var f interface{}
		err := json.Unmarshal([]byte(chaincodeCtorJSON), &f)
		if err != nil {
			return fmt.Errorf("Chaincode argument error: %s", err)
		}
		m := f.(map[string]interface{})
		sm := make(map[string]interface{})
		for k := range m {
			sm[strings.ToLower(k)] = m[k]
		}
		_, argsPresent := sm["args"]
		_, funcPresent := sm["function"]
		if !argsPresent || (len(m) == 2 && !funcPresent) || len(m) > 2 {
			return fmt.Errorf("Non-empty JSON chaincode parameters must contain the following keys: 'Args' or 'Function' and 'Args'")
		}
	} else {
		return errors.New("Empty JSON chaincode parameters must contain the following keys: 'Args' or 'Function' and 'Args'")
	}

	if chaincodeAttributesJSON != "[]" {
		var f interface{}
		err := json.Unmarshal([]byte(chaincodeAttributesJSON), &f)
		if err != nil {
			return fmt.Errorf("Chaincode argument error: %s", err)
		}
	}

	return nil
}
