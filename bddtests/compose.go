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

	"github.com/fsouza/go-dockerclient"
)

const dockerComposeCommand = "docker-compose"

// Composition represents a docker-compose execution and management
type Composition struct {
	endpoint      string
	dockerClient  *docker.Client
	apiContainers []docker.APIContainers

	composeFilesYaml string
	projectName      string
	dockerHelper     DockerHelper
}

// NewComposition create a new Composition specifying the project name (for isolation) and the compose files.
func NewComposition(projectName string, composeFilesYaml string) (composition *Composition, err error) {
	errRetFunc := func() error {
		return fmt.Errorf("Error creating new composition '%s' using compose yaml '%s':  %s", projectName, composeFilesYaml, err)
	}

	endpoint := "unix:///var/run/docker.sock"
	composition = &Composition{composeFilesYaml: composeFilesYaml, projectName: projectName}
	if composition.dockerClient, err = docker.NewClient(endpoint); err != nil {
		return nil, errRetFunc()
	}
	if _, err = composition.issueCommand([]string{"up", "--force-recreate", "-d"}); err != nil {
		return nil, errRetFunc()
	}
	if composition.dockerHelper, err = NewDockerCmdlineHelper(); err != nil {
		return nil, errRetFunc()
	}
	// Now parse the current system
	return composition, nil
}

func parseComposeFilesArg(composeFileArgs string) []string {
	var args []string
	for _, f := range strings.Fields(composeFileArgs) {
		args = append(args, []string{"-f", f}...)
	}
	return args
}

func (c *Composition) getFileArgs() []string {
	return parseComposeFilesArg(c.composeFilesYaml)
}

// GetContainerIDs returns the container IDs for the composition (NOTE: does NOT include those defined outside composition, eg. chaincode containers)
func (c *Composition) GetContainerIDs() (containerIDs []string, err error) {
	var cmdOutput string
	if cmdOutput, err = c.issueCommand([]string{"ps", "-q"}); err != nil {
		return nil, fmt.Errorf("Error getting container IDs for project '%s':  %s", c.projectName, err)
	}
	containerIDs = splitDockerCommandResults(cmdOutput)
	return containerIDs, err
}

func (c *Composition) getEnv() []string {
	return []string{"COMPOSE_PROJECT_NAME=" + c.projectName, "CORE_PEER_NETWORKID=" + c.projectName}
}

func (c *Composition) refreshContainerList() (err error) {
	var allAPIContainers []docker.APIContainers
	var thisProjectsContainers []docker.APIContainers
	if thisProjectsContainers, err = c.dockerClient.ListContainers(docker.ListContainersOptions{All: true, Filters: map[string][]string{"name": {c.projectName}}}); err != nil {
		return fmt.Errorf("Error refreshing container list for project '%s':  %s", c.projectName, err)
	}
	//if allApiContainers, err = c.dockerClient.ListContainers(docker.ListContainersOptions{All: true}); err != nil {
	//	return fmt.Errorf("Error refreshing container list for project '%s':  %s", c.projectName, err)
	//}
	for _, apiContainer := range allAPIContainers {
		if composeService, ok := apiContainer.Labels["com.docker.compose.service"]; ok == true {
			fmt.Println(fmt.Sprintf("Container name:  %s, composeService: %s, IPAddress: %s", apiContainer.Names[0], composeService, apiContainer.Networks.Networks["bridge"].IPAddress))
		}
	}
	c.apiContainers = thisProjectsContainers
	return err
}

func (c *Composition) issueCommand(args []string) (_ string, err error) {
	errRetFunc := func() error {
		return fmt.Errorf("Error issuing command to docker-compose with args '%s':  %s", args, err)
	}
	var cmdArgs []string
	cmdArgs = append(cmdArgs, c.getFileArgs()...)
	cmdArgs = append(cmdArgs, args...)
	var cmdOut []byte
	cmd := exec.Command(dockerComposeCommand, cmdArgs...)
	cmd.Env = append(cmd.Env, c.getEnv()...)
	if cmdOut, err = cmd.CombinedOutput(); err != nil {
		return string(cmdOut), errRetFunc()
	}

	// Reparse Container list
	if err = c.refreshContainerList(); err != nil {
		return "", errRetFunc()
	}
	return string(cmdOut), err
}

// Decompose decompose the composition.  Will also remove any containers with the same projectName prefix (eg. chaincode containers)
func (c *Composition) Decompose() (output string, err error) {
	//var containers []string
	output, err = c.issueCommand([]string{"stop"})
	output, err = c.issueCommand([]string{"rm", "-f"})
	// Now remove associated chaincode containers if any
	c.dockerHelper.RemoveContainersWithNamePrefix(c.projectName)
	return output, err
}

// parseComposition parses the current docker-compose project from ps command
func (c *Composition) parseComposition() (err error) {
	//c.issueCommand()
	return nil
}

// GetAPIContainerForComposeService return the docker.APIContainers with the supplied composeService name.
func (c *Composition) GetAPIContainerForComposeService(composeService string) (apiContainer *docker.APIContainers, err error) {
	for _, apiContainer := range c.apiContainers {
		if currComposeService, ok := apiContainer.Labels["com.docker.compose.service"]; ok == true {
			if currComposeService == composeService {
				return &apiContainer, nil
			}
		}
	}
	return nil, fmt.Errorf("Could not find container with compose service '%s'", composeService)
}

// GetIPAddressForComposeService returns the IPAddress of the container with the supplied composeService name.
func (c *Composition) GetIPAddressForComposeService(composeService string) (ipAddress string, err error) {
	errRetFunc := func() error {
		return fmt.Errorf("Error getting IPAddress for compose service '%s':  %s", composeService, err)
	}
	var apiContainer *docker.APIContainers
	if apiContainer, err = c.GetAPIContainerForComposeService(composeService); err != nil {
		return "", errRetFunc()
	}
	// Now get the IPAddress
	return apiContainer.Networks.Networks["bridge"].IPAddress, nil
}
