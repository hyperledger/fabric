/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric/common/metadata"
	dcontainer "github.com/moby/moby/api/types/container"
	dcli "github.com/moby/moby/client"
	"github.com/spf13/viper"
)

var logger = flogging.MustGetLogger("chaincode.platform.util")

type DockerBuildOptions struct {
	Image        string
	Cmd          string
	Env          []string
	InputStream  io.Reader
	OutputStream io.Writer
}

func (dbo DockerBuildOptions) String() string {
	return fmt.Sprintf("Image=%s Env=%s Cmd=%s)", dbo.Image, dbo.Env, dbo.Cmd)
}

// -------------------------------------------------------------------------------------------
// DockerBuild
// -------------------------------------------------------------------------------------------
// This function allows a "pass-through" build of chaincode within a docker container as
// an alternative to using standard "docker build" + Dockerfile mechanisms.  The plain docker
// build is somewhat limiting due to the resulting image that is a superset composition of
// the build-time and run-time environments.  This superset can be problematic on several
// fronts, such as a bloated image size, and additional security exposure associated with
// applications that are not needed, etc.
//
// Therefore, this mechanism creates a pipeline consisting of an ephemeral docker
// container that accepts source code as input, runs some function (e.g. "go build"), and
// outputs the result.  The intention is that this output will be consumed as the basis of
// a streamlined container by installing the output into a downstream docker-build based on
// an appropriate minimal image.
//
// The input parameters are fairly simple:
//   - Image:        (optional) The builder image to use or "chaincode.builder"
//   - Cmd:          The command to execute inside the container.
//   - InputStream:  A tarball of files that will be expanded into /chaincode/input.
//   - OutputStream: A tarball of files that will be gathered from /chaincode/output
//     after successful execution of Cmd.
//
// -------------------------------------------------------------------------------------------
func DockerBuild(opts DockerBuildOptions, client dcli.APIClient) error {
	if opts.Image == "" {
		opts.Image = GetDockerImageFromConfig("chaincode.builder")
		if opts.Image == "" {
			return fmt.Errorf("No image provided and \"chaincode.builder\" default does not exist")
		}
	}

	logger.Debugf("Attempting build with options: %s", opts)

	// -----------------------------------------------------------------------------------
	// Ensure the image exists locally, or pull it from a registry if it doesn't
	// -----------------------------------------------------------------------------------
	_, err := client.ImageInspect(context.Background(), opts.Image)
	if err != nil {
		logger.Debugf("Image %s does not exist locally, attempt pull", opts.Image)

		_, err = client.ImagePull(context.Background(), opts.Image, dcli.ImagePullOptions{})
		if err != nil {
			return fmt.Errorf("Failed to pull %s: %s", opts.Image, err)
		}
	}

	// -----------------------------------------------------------------------------------
	// Create an ephemeral container, armed with our Image/Cmd
	// -----------------------------------------------------------------------------------
	container, err := client.ContainerCreate(context.Background(), dcli.ContainerCreateOptions{
		Config: &dcontainer.Config{
			AttachStdout: true,
			AttachStderr: true,
			Env:          opts.Env,
			Cmd:          []string{"/bin/sh", "-c", opts.Cmd},
			Image:        opts.Image,
		},
	})
	if err != nil {
		return fmt.Errorf("Error creating container: %s", err)
	}
	defer client.ContainerRemove(context.Background(), container.ID, dcli.ContainerRemoveOptions{})

	// -----------------------------------------------------------------------------------
	// Upload our input stream
	// -----------------------------------------------------------------------------------
	_, err = client.CopyToContainer(context.Background(), container.ID, dcli.CopyToContainerOptions{
		DestinationPath:           "/chaincode/input",
		Content:                   opts.InputStream,
		AllowOverwriteDirWithFile: true,
	})
	if err != nil {
		return fmt.Errorf("Error uploading input to container: %s", err)
	}

	// -----------------------------------------------------------------------------------
	// Attach stdout buffer to capture possible compilation errors
	// -----------------------------------------------------------------------------------
	cw, err := client.ContainerAttach(context.Background(), container.ID, dcli.ContainerAttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
		Logs:   true,
	})
	if err != nil {
		return fmt.Errorf("Error attaching to container: %s", err)
	}

	// -----------------------------------------------------------------------------------
	// Launch the actual build, realizing the Env/Cmd specified at container creation
	// -----------------------------------------------------------------------------------
	_, err = client.ContainerStart(context.Background(), container.ID, dcli.ContainerStartOptions{})
	if err != nil {
		buff, _ := io.ReadAll(cw.Reader)
		cw.Close()
		return fmt.Errorf("Error executing build: %s \"%s\"", err, string(buff))
	}

	// -----------------------------------------------------------------------------------
	// Wait for the build to complete and gather the return value
	// -----------------------------------------------------------------------------------
	resWait := client.ContainerWait(context.Background(), container.ID, dcli.ContainerWaitOptions{})
	var res dcontainer.WaitResponse
	select {
	case res = <-resWait.Result:
	case err = <-resWait.Error:
		cw.Close()
		return fmt.Errorf("Error waiting for container to complete: %s", err)
	}

	// Wait for stream copying to complete before accessing stdout.
	defer cw.Close()
	buff, _ := io.ReadAll(cw.Reader)
	if res.StatusCode > 0 {
		logger.Errorf("Docker build failed using options: %s", opts)
		return fmt.Errorf("Error returned from build: %d \"%s\"", res.StatusCode, string(buff))
	}

	logger.Debugf("Build output is %s", string(buff))

	// -----------------------------------------------------------------------------------
	// Finally, download the result
	// -----------------------------------------------------------------------------------
	resCont, err := client.CopyFromContainer(context.Background(), container.ID, dcli.CopyFromContainerOptions{SourcePath: "/chaincode/output/."})
	if err != nil {
		return fmt.Errorf("Error downloading output: %s", err)
	}
	defer resCont.Content.Close()
	io.Copy(opts.OutputStream, resCont.Content)

	return nil
}

// GetDockerImageFromConfig replaces variables in the config
func GetDockerImageFromConfig(path string) string {
	r := strings.NewReplacer(
		"$(ARCH)", runtime.GOARCH,
		"$(PROJECT_VERSION)", metadata.Version,
		"$(TWO_DIGIT_VERSION)", twoDigitVersion(metadata.Version),
		"$(DOCKER_NS)", metadata.DockerNamespace)

	return r.Replace(viper.GetString(path))
}

// twoDigitVersion truncates a 3 digit version (e.g. 2.0.0) to a 2 digit version (e.g. 2.0),
// If version does not include dots (e.g. latest), just return the passed version.
// If version starts with a semantic rev 'v' character, strip it when computing the docker label.
func twoDigitVersion(version string) string {
	version = strings.TrimPrefix(version, "v")
	if strings.LastIndex(version, ".") < 0 {
		return version
	}
	return version[0:strings.LastIndex(version, ".")]
}
