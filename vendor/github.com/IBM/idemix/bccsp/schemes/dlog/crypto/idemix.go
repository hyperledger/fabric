/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	math "github.com/IBM/mathlib"
)

type Idemix struct {
	Curve      *math.Curve
	Translator Translator
}
