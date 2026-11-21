/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpclogging

import (
	"encoding/json/jsontext"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type protoMarshaler struct {
	message proto.Message
}

func (m *protoMarshaler) MarshalJSONTo(enc *jsontext.Encoder) error {
	out, err := protojson.Marshal(m.message)
	if err != nil {
		return err
	}
	return enc.WriteValue(out)
}

func ProtoMessage(key string, val any) zapcore.Field {
	if pm, ok := val.(proto.Message); ok && pm.ProtoReflect().IsValid() {
		return zap.Reflect(key, &protoMarshaler{message: pm})
	}
	return zap.Any(key, val)
}

func Error(err error) zapcore.Field {
	if err == nil {
		return zap.Skip()
	}

	// Wrap the error so it no longer implements fmt.Formatter. This will prevent
	// zap from adding the "verboseError" field to the log record that includes a
	// full stack trace.
	return zap.Error(struct{ error }{err})
}
