/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

import "fmt"

type ChaincodeServerStart struct {
	Address    string
	PackageID  string
	ServerArgs []string
}

func (c ChaincodeServerStart) SessionName() string {
	return fmt.Sprintf("%s-server", c.PackageID)
}

func (c ChaincodeServerStart) Args() []string {
	return c.ServerArgs
}
