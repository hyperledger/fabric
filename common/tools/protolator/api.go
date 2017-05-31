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
