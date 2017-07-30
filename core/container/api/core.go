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

package api

import (
	"io"

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/container/ccintf"
)

type BuildSpecFactory func() (io.Reader, error)
type PrelaunchFunc func() error

//VM is an abstract virtual image for supporting arbitrary virual machines
type VM interface {
	Deploy(ctxt context.Context, ccid ccintf.CCID, args []string, env []string, reader io.Reader) error
	Start(ctxt context.Context, ccid ccintf.CCID, args []string, env []string, builder BuildSpecFactory, preLaunchFunc PrelaunchFunc) error
	Stop(ctxt context.Context, ccid ccintf.CCID, timeout uint, dontkill bool, dontremove bool) error
	Destroy(ctxt context.Context, ccid ccintf.CCID, force bool, noprune bool) error
	GetVMName(ccID ccintf.CCID, format func(string) (string, error)) (string, error)
}
