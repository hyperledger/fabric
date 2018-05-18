/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

func (cs *ChaincodeSpec) Path() string {
	if cs.ChaincodeId == nil {
		return ""
	}

	return cs.ChaincodeId.Path
}
