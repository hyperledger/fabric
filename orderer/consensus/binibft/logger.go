/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine"
)

// engineLogger adapts Fabric's logger to the engine's key/value Logger.
type engineLogger struct {
	l *flogging.FabricLogger
}

var _ engine.Logger = engineLogger{}

// The engine narrates every protocol step at Info level; at Fabric's default log
// level that would drown the orderer log, so it is demoted to Debug.
func (e engineLogger) Info(msg string, kv ...interface{})  { e.l.Debugw(msg, evenKV(kv)...) }
func (e engineLogger) Debug(msg string, kv ...interface{}) { e.l.Debugw(msg, evenKV(kv)...) }
func (e engineLogger) Error(msg string, kv ...interface{}) { e.l.Errorw(msg, evenKV(kv)...) }

// evenKV makes the variadic list a valid set of key/value pairs.
func evenKV(kv []interface{}) []interface{} {
	if len(kv)%2 != 0 {
		kv = append(kv, "<missing>")
	}
	return kv
}
