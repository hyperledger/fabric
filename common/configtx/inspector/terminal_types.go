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

package inspector

import (
	"fmt"
)

// This file contains helper functions which create Viewables for fields with no more subfields
// For instance, to display an error, string, slice of strings, etc.

func viewableError(name string, source error) Viewable {
	return &field{
		name: name,
		values: []Viewable{&field{
			name: fmt.Sprintf("ERROR: %s", source),
		}},
	}
}

func viewableBytes(name string, source []byte) Viewable {
	return &field{
		name: name,
		values: []Viewable{&field{
			name: fmt.Sprintf("%x", source),
		}},
	}
}

func viewableString(name string, source string) Viewable {
	return &field{
		name: name,
		values: []Viewable{&field{
			name: source,
		}},
	}
}

func viewableUint32(name string, source uint32) Viewable {
	return &field{
		name: name,
		values: []Viewable{&field{
			name: fmt.Sprintf("%d", source),
		}},
	}
}

func viewableInt32(name string, source int32) Viewable {
	return &field{
		name: name,
		values: []Viewable{&field{
			name: fmt.Sprintf("%d", source),
		}},
	}
}

func viewableStringSlice(name string, source []string) Viewable {
	values := make([]Viewable, len(source))
	for i, str := range source {
		values[i] = viewableString(fmt.Sprintf("Index %d", i), str)
	}

	return &field{
		name:   name,
		values: values,
	}
}
