// +build experimental

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package experimental

type MyInterface interface {
	BaseInterface
	DoSomethingExperimental() string
}

func (m *MyType) DoSomethingExperimental() string {
	return "did something experimental"
}
