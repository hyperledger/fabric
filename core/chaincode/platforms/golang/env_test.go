/*
Copyright 2017 - Greg Haskins <gregory.haskins@gmail.com>

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

package golang

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_splitEnvPath(t *testing.T) {
	paths := splitEnvPaths("foo" + string(os.PathListSeparator) + "bar" + string(os.PathListSeparator) + "baz")
	assert.Equal(t, len(paths), 3)
}

func Test_getGoEnv(t *testing.T) {
	goenv, err := getGoEnv()
	assert.NoError(t, err)

	_, ok := goenv["GOPATH"]
	assert.Equal(t, ok, true)

	_, ok = goenv["GOROOT"]
	assert.Equal(t, ok, true)
}
