// +build race
// +build go1.9,linux,cgo go1.10,darwin,cgo
// +build !ppc64le

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package factory

func init() {
	raceEnabled = true
}
