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
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

//runProgram non-nil Env, timeout (typically secs or millisecs), program name and args
func runProgram(env Env, timeout time.Duration, pgm string, args ...string) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("<%s, %v>: nil env provided", pgm, args)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, pgm, args...)
	cmd.Env = flattenEnv(env)
	stdErr := &bytes.Buffer{}
	cmd.Stderr = stdErr

	out, err := cmd.Output()

	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("timed out after %s", timeout)
	}

	if err != nil {
		return nil,
			fmt.Errorf(
				"command <%s %s>: failed with error: \"%s\"\n%s",
				pgm,
				strings.Join(args, " "),
				err,
				string(stdErr.Bytes()))
	}
	return out, nil
}

// Logic inspired by: https://dave.cheney.net/2014/09/14/go-list-your-swiss-army-knife
func list(env Env, template, pkg string) ([]string, error) {
	if env == nil {
		env = getEnv()
	}

	lst, err := runProgram(env, 60*time.Second, "go", "list", "-f", template, pkg)
	if err != nil {
		return nil, err
	}

	return strings.Split(strings.Trim(string(lst), "\n"), "\n"), nil
}

func listDeps(env Env, pkg string) ([]string, error) {
	return list(env, "{{ join .Deps \"\\n\"}}", pkg)
}

func listImports(env Env, pkg string) ([]string, error) {
	return list(env, "{{ join .Imports \"\\n\"}}", pkg)
}
