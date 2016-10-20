/*
Copyright London Stock Exchange 2016 All Rights Reserved.

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

package util

import (
	"testing"

	"github.com/hyperledger/fabric/metadata"
)

func TestUtil_DockerfileTemplateParser(t *testing.T) {
	expected := "FROM foo:" + getArch() + "-" + metadata.Version
	actual := parseDockerfileTemplate("FROM foo:$(ARCH)-$(PROJECT_VERSION)")
	if actual != expected {
		t.Errorf("Error parsing Dockerfile Template.  Expected \"%s\", got \"%s\"", expected, actual)
	}
}
