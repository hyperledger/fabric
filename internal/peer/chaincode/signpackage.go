/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"io/ioutil"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/core/common/ccpackage"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/cobra"
)

// signpackageCmd returns the cobra command for signing a package
func signpackageCmd(cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) *cobra.Command {
	spCmd := &cobra.Command{
		Use:       "signpackage",
		Short:     "Sign the specified chaincode package",
		Long:      "Sign the specified chaincode package",
		ValidArgs: []string{"2"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("peer chaincode signpackage <inputpackage> <outputpackage>")
			}
			return signpackage(cmd, args[0], args[1], cf, cryptoProvider)
		},
	}

	return spCmd
}

func signpackage(cmd *cobra.Command, ipackageFile string, opackageFile string, cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) error {
	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(cmd.Name(), false, false, cryptoProvider)
		if err != nil {
			return err
		}
	}

	b, err := ioutil.ReadFile(ipackageFile)
	if err != nil {
		return err
	}

	env := protoutil.UnmarshalEnvelopeOrPanic(b)

	env, err = ccpackage.SignExistingPackage(env, cf.Signer)
	if err != nil {
		return err
	}

	b = protoutil.MarshalOrPanic(env)
	err = ioutil.WriteFile(opackageFile, b, 0700)
	if err != nil {
		return err
	}

	fmt.Printf("Wrote signed package to %s successfully\n", opackageFile)

	return nil
}
