/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fakes

// HistoryleveldbLogger is a wrapper to fakes.Logger so that we can overwrite Warnf.
type HistoryleveldbLogger struct {
	*HistorydbLogger
}

// Warnf copies byte array in arg2 and then delegates to fakes.Logger.
// We have to make the copy because goleveldb iterator
// reuses the byte array for keys and values for the Next() call.
func (hl *HistoryleveldbLogger) Warnf(arg1 string, arg2 ...interface{}) {
	for i := 0; i < len(arg2); i++ {
		if b, ok := arg2[i].([]byte); ok {
			bCopy := make([]byte, len(b))
			copy(bCopy, b)
			arg2[i] = bCopy
		}
	}
	hl.HistorydbLogger.Warnf(arg1, arg2...)
}
