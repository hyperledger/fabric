/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
// There are currently two different types of interfaces available for protos to implement:
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
	// StaticallyOpaqueFields returns the field names which contain opaque data
	StaticallyOpaqueMapFields() []string

	// StaticallyOpaqueFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	StaticallyOpaqueMapFieldProto(name string, key string) (proto.Message, error)
}

// StaticallyOpaqueSliceFieldProto should be implemented by protos which have maps to bytes fields
// which are the marshaled value of a fixed type
type StaticallyOpaqueSliceFieldProto interface {
	// StaticallyOpaqueFields returns the field names which contain opaque data
	StaticallyOpaqueSliceFields() []string

	// StaticallyOpaqueFieldProto returns a newly allocated proto message of the correct
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
	// VariablyOpaqueFields returns the field names which contain opaque data
	VariablyOpaqueMapFields() []string

	// VariablyOpaqueFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	VariablyOpaqueMapFieldProto(name string, key string) (proto.Message, error)
}

// VariablyOpaqueSliceFieldProto should be implemented by protos which have maps to bytes fields
// which are the marshaled value of a a message type determined by the other contents of the proto
type VariablyOpaqueSliceFieldProto interface {
	// VariablyOpaqueFields returns the field names which contain opaque data
	VariablyOpaqueSliceFields() []string

	// VariablyOpaqueFieldProto returns a newly allocated proto message of the correct
	// type for the field name.
	VariablyOpaqueSliceFieldProto(name string, index int) (proto.Message, error)
}
