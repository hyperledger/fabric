/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"testing"
)

func TestBroadcastNoPanic(t *testing.T) {
	// Defer recovers from the panic
	_ = (&server{}).Broadcast(nil)
}

func TestDeliverNoPanic(t *testing.T) {
	// Defer recovers from the panic
	_ = (&server{}).Deliver(nil)
}
