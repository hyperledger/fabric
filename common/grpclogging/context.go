/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpclogging

import (
	"context"

	"go.uber.org/zap/zapcore"
)

type fieldKeyType struct{}

var fieldKey = &fieldKeyType{}

func ZapFields(ctx context.Context) []zapcore.Field {
	fields, ok := ctx.Value(fieldKey).([]zapcore.Field)
	if ok {
		return fields
	}
	return nil
}

func Fields(ctx context.Context) []interface{} {
	fields, ok := ctx.Value(fieldKey).([]zapcore.Field)
	if !ok {
		return nil
	}
	genericFields := make([]interface{}, len(fields))
	for i := range fields {
		genericFields[i] = fields[i]
	}
	return genericFields
}

func WithFields(ctx context.Context, fields []zapcore.Field) context.Context {
	return context.WithValue(ctx, fieldKey, fields)
}
