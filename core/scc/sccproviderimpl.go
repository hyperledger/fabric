/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

// BuiltinSCCs are special system chaincodes, differentiated from other (plugin)
// system chaincodes.  These chaincodes do not need to be initialized in '_lifecycle'
// and may be invoked without a channel context.  It is expected that '_lifecycle'
// will eventually be the only builtin SCCs.
// Note, this field should only be used on _endorsement_ side, never in validation
// as it might change.
type BuiltinSCCs map[string]struct{}

func (bccs BuiltinSCCs) IsSysCC(name string) bool {
	_, ok := bccs[name]
	return ok
}
