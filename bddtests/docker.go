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

package bddtests

import (
	"fmt"
	"os/exec"
	"strings"
)

// DockerHelper helper for docker specific functions
type DockerHelper interface {
	GetIPAddress(containerID string) (string, error)
	RemoveContainersWithNamePrefix(namePrefix string) error
}

// NewDockerCmdlineHelper returns a new command line DockerHelper instance
func NewDockerCmdlineHelper() (DockerHelper, error) {
	dockerCmdlineHelper := &dockerCmdlineHelper{}
	return dockerCmdlineHelper, nil
}

type dockerCmdlineHelper struct {
}

func splitDockerCommandResults(cmdOutput string) (linesToReturn []string) {
	lines := strings.Split(string(cmdOutput), "\n")
	for _, line := range lines {
		if len(line) > 0 {
			linesToReturn = append(linesToReturn, line)
		}
	}
	return linesToReturn
}

func (d *dockerCmdlineHelper) issueDockerCommand(cmdArgs []string) (string, error) {
	var cmdOut []byte
	var err error
	cmd := exec.Command("docker", cmdArgs...)
	//cmd.Env = append(cmd.Env, c.getEnv()...)
	cmdOut, err = cmd.CombinedOutput()
	return string(cmdOut), err
}

func (d *dockerCmdlineHelper) getContainerIDsWithNamePrefix(namePrefix string) ([]string, error) {
	cmdOutput, err := d.issueDockerCommand([]string{"ps", "--filter", fmt.Sprintf("name=%s", namePrefix), "-qa"})
	if err != nil {
		return nil, fmt.Errorf("Error getting containers with name prefix '%s':  %s", namePrefix, err)
	}
	containerIDs := splitDockerCommandResults(cmdOutput)
	return containerIDs, err
}

func (d *dockerCmdlineHelper) GetIPAddress(containerID string) (ipAddress string, err error) {
	var (
		cmdOutput string
		lines     []string
	)
	errRetFunc := func() error {
		return fmt.Errorf("Error getting IPAddress for container '%s':  %s", containerID, err)
	}
	if cmdOutput, err = d.issueDockerCommand([]string{"inspect", "--format", "{{ .NetworkSettings.IPAddress }}", containerID}); err != nil {
		return "", errRetFunc()
	}

	if lines = splitDockerCommandResults(cmdOutput); len(lines) != 1 {
		err = fmt.Errorf("unexpected length on inspect output")
		return "", errRetFunc()
	}
	ipAddress = lines[0]
	return ipAddress, nil
}

func (d *dockerCmdlineHelper) RemoveContainersWithNamePrefix(namePrefix string) error {
	containers, err := d.getContainerIDsWithNamePrefix(namePrefix)
	if err != nil {
		return fmt.Errorf("Error removing containers with name prefix (%s):  %s", namePrefix, err)
	}
	for _, id := range containers {
		fmt.Printf("container: %s", id)
		_, _ = d.issueDockerCommand([]string{"rm", "-f", id})
	}
	return nil
}
