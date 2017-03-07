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
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Logic inspired by: https://dave.cheney.net/2014/09/14/go-list-your-swiss-army-knife
func list(env Env, template, pkg string) ([]string, error) {

	if env == nil {
		env = getEnv()
	}

	var stdOut bytes.Buffer
	var stdErr bytes.Buffer

	cmd := exec.Command("go", "list", "-f", template, pkg)
	cmd.Env = flattenEnv(env)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err := cmd.Start()

	// Create a go routine that will wait for the command to finish
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(60 * time.Second):
		if err = cmd.Process.Kill(); err != nil {
			return nil, fmt.Errorf("go list: failed to kill: %s", err)
		} else {
			return nil, errors.New("go list: timeout")
		}
	case err = <-done:
		if err != nil {
			return nil, fmt.Errorf("go list: failed with error: \"%s\"\n%s", err, string(stdErr.Bytes()))
		}

		return strings.Split(string(stdOut.Bytes()), "\n"), nil
	}
}

func listDeps(env Env, pkg string) ([]string, error) {
	return list(env, "{{ join .Deps \"\\n\"}}", pkg)
}

func listImports(env Env, pkg string) ([]string, error) {
	return list(env, "{{ join .Imports \"\\n\"}}", pkg)
}
