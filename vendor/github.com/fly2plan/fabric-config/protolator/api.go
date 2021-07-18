/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protolator

import (
	"github.com/golang/protobuf/proto"
)

///////////////////////////////////////////////////////////////////////////////////////////////////
//
// This set of interfaces and methods is designed to allow protos to have Go methods attached
// to them, so that they may be automatically marshaled to human readable JSON (where the
// opaque byte fields are represented as their expanded proto contents) and back once again
// to standard proto messages.
//
// There are currently three different types of interfaces available for protos to implement:
//
// 1. StaticallyOpaque*FieldProto:  These interfaces should be implemented by protos which have
// opaque byte fields whose marshaled type is known at compile time.  This is mostly true
// for the signature oriented fields like the Envelope.Payload, or Header.ChannelHeader
//
// 2. VariablyOpaque*FieldProto: These interfaces are identical to the StaticallyOpaque*FieldProto
// definitions, with the exception that they are guaranteed to be evaluated after the
// StaticallyOpaque*FieldProto definitions.  In particular, this allows for the type selection of
// a VariablyOpaque*FieldProto to depend on data populated by the StaticallyOpaque*FieldProtos.
// For example, the Payload.data field depends upon the Payload.Header.ChannelHeader.type field,
// which is along a statically marshaled path.
//
// 3. Dynamic*FieldProto: These interfaces are for messages which contain other messages whose
// attributes cannot be determined at compile time.  For example, a ConfigValue message may evaluate
// the map field values["MSP"] successfully in an organization context, but not at all in a channel
// context.  Because go is not a dynamic language, this dynamic behavior must be simulated by
// wrapping the underlying proto message in another type which can be configured at runtime with
// different contextual behavior. (See tests for examples)
//
///////////////////////////////////////////////////////////////////////////////////////////////////

// StaticallyOpaqueFieldProto should be implemented by protos which have bytes fields which
// are the marshaled value of a fixed type
type StaticallyOpaqueFieldProto interface {
	// StaticallyOpaqueFields returns the field names which contain opaque data
	StaticallyOpaqueFields() []string

	// StaticallyOpaqueFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	StaticallyOpaqueFieldProto(name string) (proto.Message, error)
}

// StaticallyOpaqueMapFieldProto should be implemented by protos which have maps to bytes fields
// which are the marshaled value of a fixed type
type StaticallyOpaqueMapFieldProto interface {
	// StaticallyOpaqueMapFields returns the field names which contain opaque data
	StaticallyOpaqueMapFields() []string

	// StaticallyOpaqueMapFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	StaticallyOpaqueMapFieldProto(name string, key string) (proto.Message, error)
}

// StaticallyOpaqueSliceFieldProto should be implemented by protos which have maps to bytes fields
// which are the marshaled value of a fixed type
type StaticallyOpaqueSliceFieldProto interface {
	// StaticallyOpaqueSliceFields returns the field names which contain opaque data
	StaticallyOpaqueSliceFields() []string

	// StaticallyOpaqueSliceFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	StaticallyOpaqueSliceFieldProto(name string, index int) (proto.Message, error)
}

// VariablyOpaqueFieldProto should be implemented by protos which have bytes fields which
// are the marshaled value depends upon the other contents of the proto
type VariablyOpaqueFieldProto interface {
	// VariablyOpaqueFields returns the field names which contain opaque data
	VariablyOpaqueFields() []string

	// VariablyOpaqueFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	VariablyOpaqueFieldProto(name string) (proto.Message, error)
}

// VariablyOpaqueMapFieldProto should be implemented by protos which have maps to bytes fields
// which are the marshaled value of a a message type determined by the other contents of the proto
type VariablyOpaqueMapFieldProto interface {
	// VariablyOpaqueMapFields returns the field names which contain opaque data
	VariablyOpaqueMapFields() []string

	// VariablyOpaqueMapFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	VariablyOpaqueMapFieldProto(name string, key string) (proto.Message, error)
}

// VariablyOpaqueSliceFieldProto should be implemented by protos which have maps to bytes fields
// which are the marshaled value of a a message type determined by the other contents of the proto
type VariablyOpaqueSliceFieldProto interface {
	// VariablyOpaqueSliceFields returns the field names which contain opaque data
	VariablyOpaqueSliceFields() []string

	// VariablyOpaqueFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	VariablyOpaqueSliceFieldProto(name string, index int) (proto.Message, error)
}

// DynamicFieldProto should be implemented by protos which have nested fields whose attributes
// (such as their opaque types) cannot be determined until runtime
type DynamicFieldProto interface {
	// DynamicFields returns the field names which are dynamic
	DynamicFields() []string

	// DynamicFieldProto returns a newly allocated dynamic message, decorating an underlying
	// proto message with the runtime determined function
	DynamicFieldProto(name string, underlying proto.Message) (proto.Message, error)
}

// DynamicMapFieldProto should be implemented by protos which have maps to messages whose attributes
// (such as their opaque types) cannot be determined until runtime
type DynamicMapFieldProto interface {
	// DynamicMapFields returns the field names which are dynamic
	DynamicMapFields() []string

	// DynamicMapFieldProto returns a newly allocated dynamic message, decorating an underlying
	// proto message with the runtime determined function
	DynamicMapFieldProto(name string, key string, underlying proto.Message) (proto.Message, error)
}

// DynamicSliceFieldProto should be implemented by protos which have slices of messages whose attributes
// (such as their opaque types) cannot be determined until runtime
type DynamicSliceFieldProto interface {
	// DynamicSliceFields returns the field names which are dynamic
	DynamicSliceFields() []string

	// DynamicSliceFieldProto returns a newly allocated dynamic message, decorating an underlying
	// proto message with the runtime determined function
	DynamicSliceFieldProto(name string, index int, underlying proto.Message) (proto.Message, error)
}

// DecoratedProto should be implemented by the dynamic wrappers applied by the Dynamic*FieldProto interfaces
// This is necessary for the proto system to unmarshal, because it discovers proto message type by reflection
// (Rather than by interface definition as it probably should ( https://github.com/golang/protobuf/issues/291 )
type DecoratedProto interface {
	// Underlying returns the underlying proto message which is being dynamically decorated
	Underlying() proto.Message
}
