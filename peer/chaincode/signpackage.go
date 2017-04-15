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
	"fmt"
	"io/ioutil"

	"github.com/spf13/cobra"

	"github.com/hyperledger/fabric/core/common/ccpackage"
	"github.com/hyperledger/fabric/protos/utils"
)

// signpackageCmd returns the cobra command for signing a package
func signpackageCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	spCmd := &cobra.Command{
		Use:       "signpackage",
		Short:     "Sign the specified chaincode package",
		Long:      "Sign the specified chaincode package",
		ValidArgs: []string{"2"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("peer chaincode signpackage <inputpackage> <outputpackage>")
			}
			return signpackage(cmd, args[0], args[1], cf)
		},
	}

	return spCmd
}

func signpackage(cmd *cobra.Command, ipackageFile string, opackageFile string, cf *ChaincodeCmdFactory) error {
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(false, false)
		if err != nil {
			return err
		}
	}

	b, err := ioutil.ReadFile(ipackageFile)
	if err != nil {
		return err
	}

	env := utils.UnmarshalEnvelopeOrPanic(b)

	env, err = ccpackage.SignExistingPackage(env, cf.Signer)
	if err != nil {
		return err
	}

	b = utils.MarshalOrPanic(env)
	err = ioutil.WriteFile(opackageFile, b, 0700)
	if err != nil {
		return err
	}

	fmt.Printf("Wrote signed package to %s successfully\n", opackageFile)

	return nil
}
