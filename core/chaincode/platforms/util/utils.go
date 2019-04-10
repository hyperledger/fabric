/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/config"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/spf13/viper"
)

var logger = flogging.MustGetLogger("chaincode.platform.util")

//ComputeHash computes contents hash based on previous hash
func ComputeHash(contents []byte, hash []byte) []byte {
	data := util.ConcatenateBytes(contents, hash)
	return util.ComputeSHA256(data)
}

//HashFilesInDir computes h=hash(h,file bytes) for each file in a directory
//Directory entries are traversed recursively. In the end a single
//hash value is returned for the entire directory structure
func HashFilesInDir(rootDir string, dir string, hash []byte, tw *tar.Writer) ([]byte, error) {
	currentDir := filepath.Join(rootDir, dir)
	logger.Debugf("hashFiles %s", currentDir)
	//ReadDir returns sorted list of files in dir
	fis, err := ioutil.ReadDir(currentDir)
	if err != nil {
		return hash, fmt.Errorf("ReadDir failed %s\n", err)
	}
	for _, fi := range fis {
		name := filepath.Join(dir, fi.Name())
		if fi.IsDir() {
			var err error
			hash, err = HashFilesInDir(rootDir, name, hash, tw)
			if err != nil {
				return hash, err
			}
			continue
		}
		fqp := filepath.Join(rootDir, name)
		buf, err := ioutil.ReadFile(fqp)
		if err != nil {
			logger.Errorf("Error reading %s\n", err)
			return hash, err
		}

		//get the new hash from file contents
		hash = ComputeHash(buf, hash)

		if tw != nil {
			is := bytes.NewReader(buf)
			if err = cutil.WriteStreamToPackage(is, fqp, filepath.Join("src", name), tw); err != nil {
				return hash, fmt.Errorf("Error adding file to tar %s", err)
			}
		}
	}
	return hash, nil
}

type DockerBuildOptions struct {
	Image        string
	Env          []string
	Cmd          string
	InputStream  io.Reader
	OutputStream io.Writer
}

//-------------------------------------------------------------------------------------------
// DockerBuild
//-------------------------------------------------------------------------------------------
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
//      - Image:        (optional) The builder image to use or "chaincode.builder"
//      - Env:          (optional) environment variables for the build environment.
//      - Cmd:          The command to execute inside the container.
//      - InputStream:  A tarball of files that will be expanded into /chaincode/input.
//      - OutputStream: A tarball of files that will be gathered from /chaincode/output
//                      after successful execution of Cmd.
//-------------------------------------------------------------------------------------------
func DockerBuild(opts DockerBuildOptions) error {
	var client *docker.Client
	var err error
	endpoint := viper.GetString("vm.endpoint")
	tlsEnabled := viper.GetBool("vm.docker.tls.enabled")
	if tlsEnabled {
		cert := config.GetPath("vm.docker.tls.cert.file")
		key := config.GetPath("vm.docker.tls.key.file")
		ca := config.GetPath("vm.docker.tls.ca.file")
		client, err = docker.NewTLSClient(endpoint, cert, key, ca)
	} else {
		client, err = docker.NewClient(endpoint)
	}

	if err != nil {
		return fmt.Errorf("Error creating docker client: %s", err)
	}
	if opts.Image == "" {
		opts.Image = GetDockerfileFromConfig("chaincode.builder")
		if opts.Image == "" {
			return fmt.Errorf("No image provided and \"chaincode.builder\" default does not exist")
		}
	}

	logger.Debugf("Attempting build with image %s", opts.Image)

	//-----------------------------------------------------------------------------------
	// Ensure the image exists locally, or pull it from a registry if it doesn't
	//-----------------------------------------------------------------------------------
	_, err = client.InspectImage(opts.Image)
	if err != nil {
		logger.Debugf("Image %s does not exist locally, attempt pull", opts.Image)

		err = client.PullImage(docker.PullImageOptions{Repository: opts.Image}, docker.AuthConfiguration{})
		if err != nil {
			return fmt.Errorf("Failed to pull %s: %s", opts.Image, err)
		}
	}

	//-----------------------------------------------------------------------------------
	// Create an ephemeral container, armed with our Env/Cmd
	//-----------------------------------------------------------------------------------
	container, err := client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image:        opts.Image,
			Env:          opts.Env,
			Cmd:          []string{"/bin/sh", "-c", opts.Cmd},
			AttachStdout: true,
			AttachStderr: true,
		},
	})
	if err != nil {
		return fmt.Errorf("Error creating container: %s", err)
	}
	defer client.RemoveContainer(docker.RemoveContainerOptions{ID: container.ID})

	//-----------------------------------------------------------------------------------
	// Upload our input stream
	//-----------------------------------------------------------------------------------
	err = client.UploadToContainer(container.ID, docker.UploadToContainerOptions{
		Path:        "/chaincode/input",
		InputStream: opts.InputStream,
	})
	if err != nil {
		return fmt.Errorf("Error uploading input to container: %s", err)
	}

	//-----------------------------------------------------------------------------------
	// Attach stdout buffer to capture possible compilation errors
	//-----------------------------------------------------------------------------------
	stdout := bytes.NewBuffer(nil)
	cw, err := client.AttachToContainerNonBlocking(docker.AttachToContainerOptions{
		Container:    container.ID,
		OutputStream: stdout,
		ErrorStream:  stdout,
		Logs:         true,
		Stdout:       true,
		Stderr:       true,
		Stream:       true,
	})
	if err != nil {
		return fmt.Errorf("Error attaching to container: %s", err)
	}

	//-----------------------------------------------------------------------------------
	// Launch the actual build, realizing the Env/Cmd specified at container creation
	//-----------------------------------------------------------------------------------
	err = client.StartContainer(container.ID, nil)
	if err != nil {
		cw.Close()
		return fmt.Errorf("Error executing build: %s \"%s\"", err, stdout.String())
	}

	//-----------------------------------------------------------------------------------
	// Wait for the build to complete and gather the return value
	//-----------------------------------------------------------------------------------
	retval, err := client.WaitContainer(container.ID)
	if err != nil {
		cw.Close()
		return fmt.Errorf("Error waiting for container to complete: %s", err)
	}

	// Wait for stream copying to complete before accessing stdout.
	cw.Close()
	if err := cw.Wait(); err != nil {
		logger.Errorf("attach wait failed: %s", err)
	}

	if retval > 0 {
		return fmt.Errorf("Error returned from build: %d \"%s\"", retval, stdout.String())
	}

	logger.Debugf("Build output is %s", stdout.String())

	//-----------------------------------------------------------------------------------
	// Finally, download the result
	//-----------------------------------------------------------------------------------
	err = client.DownloadFromContainer(container.ID, docker.DownloadFromContainerOptions{
		Path:         "/chaincode/output/.",
		OutputStream: opts.OutputStream,
	})
	if err != nil {
		return fmt.Errorf("Error downloading output: %s", err)
	}

	return nil
}

func GetDockerfileFromConfig(path string) string {
	r := strings.NewReplacer(
		"$(ARCH)", runtime.GOARCH,
		"$(PROJECT_VERSION)", metadata.Version,
		"$(DOCKER_NS)", metadata.DockerNamespace,
		"$(BASE_DOCKER_NS)", metadata.BaseDockerNamespace)

	return r.Replace(viper.GetString(path))
}
