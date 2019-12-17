/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package bccsp

import "fmt"

// SHA256Opts contains options relating to SHA-256.
type SHA256Opts struct {
}

// Algorithm returns the hash algorithm identifier (to be used).
func (opts *SHA256Opts) Algorithm() string {
	return SHA256
}

// SHA384Opts contains options relating to SHA-384.
type SHA384Opts struct {
}

// Algorithm returns the hash algorithm identifier (to be used).
func (opts *SHA384Opts) Algorithm() string {
	return SHA384
}

// SHA3_256Opts contains options relating to SHA3-256.
type SHA3_256Opts struct {
}

// Algorithm returns the hash algorithm identifier (to be used).
func (opts *SHA3_256Opts) Algorithm() string {
	return SHA3_256
}

// SHA3_384Opts contains options relating to SHA3-384.
type SHA3_384Opts struct {
}

// Algorithm returns the hash algorithm identifier (to be used).
func (opts *SHA3_384Opts) Algorithm() string {
	return SHA3_384
}

// GetHashOpt returns the HashOpts corresponding to the passed hash function
func GetHashOpt(hashFunction string) (HashOpts, error) {
	switch hashFunction {
	case SHA256:
		return &SHA256Opts{}, nil
	case SHA384:
		return &SHA384Opts{}, nil
	case SHA3_256:
		return &SHA3_256Opts{}, nil
	case SHA3_384:
		return &SHA3_384Opts{}, nil
	}
	return nil, fmt.Errorf("hash function not recognized [%s]", hashFunction)
}
