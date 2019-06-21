/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/pkg/errors"
)

// ContainerType is the string which the docker container type
// is registered with the container.VMController
const ContainerType = "DOCKER"

var (
	dockerLogger = flogging.MustGetLogger("dockercontroller")
	vmRegExp     = regexp.MustCompile("[^a-zA-Z0-9-_.]")
	imageRegExp  = regexp.MustCompile("^[a-z0-9]+(([._-][a-z0-9]+)+)?$")
)

//go:generate counterfeiter -o mock/dockerclient.go --fake-name DockerClient dockerClient

// dockerClient represents a docker client
type dockerClient interface {
	// CreateContainer creates a docker container, returns an error in case of failure
	CreateContainer(opts docker.CreateContainerOptions) (*docker.Container, error)
	// UploadToContainer uploads a tar archive to be extracted to a path in the
	// filesystem of the container.
	UploadToContainer(id string, opts docker.UploadToContainerOptions) error
	// StartContainer starts a docker container, returns an error in case of failure
	StartContainer(id string, cfg *docker.HostConfig) error
	// AttachToContainer attaches to a docker container, returns an error in case of
	// failure
	AttachToContainer(opts docker.AttachToContainerOptions) error
	// BuildImage builds an image from a tarball's url or a Dockerfile in the input
	// stream, returns an error in case of failure
	BuildImage(opts docker.BuildImageOptions) error
	// StopContainer stops a docker container, killing it after the given timeout
	// (in seconds). Returns an error in case of failure
	StopContainer(id string, timeout uint) error
	// KillContainer sends a signal to a docker container, returns an error in
	// case of failure
	KillContainer(opts docker.KillContainerOptions) error
	// RemoveContainer removes a docker container, returns an error in case of failure
	RemoveContainer(opts docker.RemoveContainerOptions) error
	// PingWithContext pings the docker daemon. The context object can be used
	// to cancel the ping request.
	PingWithContext(context.Context) error
	// WaitContainer blocks until the given container stops, and returns the exit
	// code of the container status.
	WaitContainer(containerID string) (int, error)
}

// Provider implements container.VMProvider
type Provider struct {
	PeerID        string
	NetworkID     string
	BuildMetrics  *BuildMetrics
	HostConfig    *docker.HostConfig
	Client        dockerClient
	AttachStdOut  bool
	ChaincodePull bool
	NetworkMode   string
}

// DockerVM is a vm. It is identified by an image id
type DockerVM struct {
	PeerID        string
	NetworkID     string
	BuildMetrics  *BuildMetrics
	HostConfig    *docker.HostConfig
	Client        dockerClient
	AttachStdOut  bool
	ChaincodePull bool
	NetworkMode   string
}

// NewVM creates a new DockerVM instance
func (p *Provider) NewVM() container.VM {
	return &DockerVM{
		PeerID:        p.PeerID,
		NetworkID:     p.NetworkID,
		Client:        p.Client,
		BuildMetrics:  p.BuildMetrics,
		AttachStdOut:  p.AttachStdOut,
		HostConfig:    p.HostConfig,
		ChaincodePull: p.ChaincodePull,
		NetworkMode:   p.NetworkMode,
	}
}

func (vm *DockerVM) createContainer(imageID, containerID string, args, env []string) error {
	logger := dockerLogger.With("imageID", imageID, "containerID", containerID)
	logger.Debugw("create container")
	_, err := vm.Client.CreateContainer(docker.CreateContainerOptions{
		Name: containerID,
		Config: &docker.Config{
			Cmd:          args,
			Image:        imageID,
			Env:          env,
			AttachStdout: vm.AttachStdOut,
			AttachStderr: vm.AttachStdOut,
		},
		HostConfig: vm.HostConfig,
	})
	if err != nil {
		return err
	}
	logger.Debugw("created container")
	return nil
}

func (vm *DockerVM) deployImage(ccid ccintf.CCID, reader io.Reader) error {
	id, err := vm.GetVMNameForDocker(ccid)
	if err != nil {
		return err
	}

	outputbuf := bytes.NewBuffer(nil)
	opts := docker.BuildImageOptions{
		Name:         id,
		Pull:         vm.ChaincodePull,
		NetworkMode:  vm.NetworkMode,
		InputStream:  reader,
		OutputStream: outputbuf,
	}

	startTime := time.Now()
	err = vm.Client.BuildImage(opts)

	vm.BuildMetrics.ChaincodeImageBuildDuration.With(
		"chaincode", ccid.String(),
		"success", strconv.FormatBool(err == nil),
	).Observe(time.Since(startTime).Seconds())

	if err != nil {
		dockerLogger.Errorf("Error building image: %s", err)
		dockerLogger.Errorf("Build Output:\n********************\n%s\n********************", outputbuf.String())
		return err
	}

	dockerLogger.Debugf("Created image: %s", id)
	return nil
}

// Start starts a container using a previously created docker image
func (vm *DockerVM) Start(ccid ccintf.CCID, args, env []string, filesToUpload map[string][]byte, builder container.Builder) error {
	imageName, err := vm.GetVMNameForDocker(ccid)
	if err != nil {
		return err
	}

	containerName := vm.GetVMName(ccid)
	logger := dockerLogger.With("imageName", imageName, "containerName", containerName)

	vm.stopInternal(containerName, 0, false, false)

	err = vm.createContainer(imageName, containerName, args, env)
	if err == docker.ErrNoSuchImage {
		reader, err := builder.Build()
		if err != nil {
			return errors.Wrapf(err, "failed to generate Dockerfile to build %s", containerName)
		}

		err = vm.deployImage(ccid, reader)
		if err != nil {
			return err
		}

		err = vm.createContainer(imageName, containerName, args, env)
		if err != nil {
			logger.Errorf("failed to create container: %s", err)
			return err
		}
	} else if err != nil {
		logger.Errorf("create container failed: %s", err)
		return err
	}

	// stream stdout and stderr to chaincode logger
	if vm.AttachStdOut {
		containerLogger := flogging.MustGetLogger("peer.chaincode." + containerName)
		streamOutput(dockerLogger, vm.Client, containerName, containerLogger)
	}

	// upload specified files to the container before starting it
	// this can be used for configurations such as TLS key and certs
	if len(filesToUpload) != 0 {
		// the docker upload API takes a tar file, so we need to first
		// consolidate the file entries to a tar
		payload := bytes.NewBuffer(nil)
		gw := gzip.NewWriter(payload)
		tw := tar.NewWriter(gw)

		for path, fileToUpload := range filesToUpload {
			cutil.WriteBytesToPackage(path, fileToUpload, tw)
		}

		// Write the tar file out
		if err := tw.Close(); err != nil {
			return fmt.Errorf("Error writing files to upload to Docker instance into a temporary tar blob: %s", err)
		}

		gw.Close()

		err := vm.Client.UploadToContainer(containerName, docker.UploadToContainerOptions{
			InputStream:          bytes.NewReader(payload.Bytes()),
			Path:                 "/",
			NoOverwriteDirNonDir: false,
		})
		if err != nil {
			return fmt.Errorf("Error uploading files to the container instance %s: %s", containerName, err)
		}
	}

	// start container with HostConfig was deprecated since v1.10 and removed in v1.2
	err = vm.Client.StartContainer(containerName, nil)
	if err != nil {
		dockerLogger.Errorf("start-could not start container: %s", err)
		return err
	}

	dockerLogger.Debugf("Started container %s", containerName)
	return nil
}

// streamOutput mirrors output from the named container to a fabric logger.
func streamOutput(logger *flogging.FabricLogger, client dockerClient, containerName string, containerLogger *flogging.FabricLogger) {
	// Launch a few go routines to manage output streams from the container.
	// They will be automatically destroyed when the container exits
	attached := make(chan struct{})
	r, w := io.Pipe()

	go func() {
		// AttachToContainer will fire off a message on the "attached" channel once the
		// attachment completes, and then block until the container is terminated.
		// The returned error is not used outside the scope of this function. Assign the
		// error to a local variable to prevent clobbering the function variable 'err'.
		err := client.AttachToContainer(docker.AttachToContainerOptions{
			Container:    containerName,
			OutputStream: w,
			ErrorStream:  w,
			Logs:         true,
			Stdout:       true,
			Stderr:       true,
			Stream:       true,
			Success:      attached,
		})

		// If we get here, the container has terminated.  Send a signal on the pipe
		// so that downstream may clean up appropriately
		_ = w.CloseWithError(err)
	}()

	go func() {
		defer r.Close() // ensure the pipe reader gets closed

		// Block here until the attachment completes or we timeout
		select {
		case <-attached: // successful attach
			close(attached) // close indicates the streams can now be copied

		case <-time.After(10 * time.Second):
			logger.Errorf("Timeout while attaching to IO channel in container %s", containerName)
			return
		}

		is := bufio.NewReader(r)
		for {
			// Loop forever dumping lines of text into the containerLogger
			// until the pipe is closed
			line, err := is.ReadString('\n')
			switch err {
			case nil:
				containerLogger.Info(line)
			case io.EOF:
				logger.Infof("Container %s has closed its IO channel", containerName)
				return
			default:
				logger.Errorf("Error reading container output: %s", err)
				return
			}
		}
	}()
}

// Stop stops a running chaincode
func (vm *DockerVM) Stop(ccid ccintf.CCID, timeout uint, dontkill bool, dontremove bool) error {
	id := vm.ccidToContainerID(ccid)
	return vm.stopInternal(id, timeout, dontkill, dontremove)
}

// Wait blocks until the container stops and returns the exit code of the container.
func (vm *DockerVM) Wait(ccid ccintf.CCID) (int, error) {
	id := vm.ccidToContainerID(ccid)
	return vm.Client.WaitContainer(id)
}

func (vm *DockerVM) ccidToContainerID(ccid ccintf.CCID) string {
	return strings.Replace(vm.GetVMName(ccid), ":", "_", -1)
}

// HealthCheck checks if the DockerVM is able to communicate with the Docker
// daemon.
func (vm *DockerVM) HealthCheck(ctx context.Context) error {
	if err := vm.Client.PingWithContext(ctx); err != nil {
		return errors.Wrap(err, "failed to ping to Docker daemon")
	}
	return nil
}

func (vm *DockerVM) stopInternal(id string, timeout uint, dontkill, dontremove bool) error {
	logger := dockerLogger.With("id", id)

	logger.Debugw("stopping container")
	err := vm.Client.StopContainer(id, timeout)
	dockerLogger.Debugw("stop container result", "error", err)

	if !dontkill {
		logger.Debugw("killing container")
		err = vm.Client.KillContainer(docker.KillContainerOptions{ID: id})
		logger.Debugw("kill container result", "error", err)
	}

	if !dontremove {
		logger.Debugw("removing container")
		err = vm.Client.RemoveContainer(docker.RemoveContainerOptions{ID: id, Force: true})
		logger.Debugw("remove container result", "error", err)
	}

	return err
}

// GetVMName generates the VM name from peer information. It accepts a format
// function parameter to allow different formatting based on the desired use of
// the name.
func (vm *DockerVM) GetVMName(ccid ccintf.CCID) string {
	// replace any invalid characters with "-" (either in network id, peer id, or in the
	// entire name returned by any format function)
	return vmRegExp.ReplaceAllString(vm.preFormatImageName(ccid), "-")
}

// GetVMNameForDocker formats the docker image from peer information. This is
// needed to keep image (repository) names unique in a single host, multi-peer
// environment (such as a development environment). It computes the hash for the
// supplied image name and then appends it to the lowercase image name to ensure
// uniqueness.
func (vm *DockerVM) GetVMNameForDocker(ccid ccintf.CCID) (string, error) {
	name := vm.preFormatImageName(ccid)
	hash := hex.EncodeToString(util.ComputeSHA256([]byte(name)))
	saniName := vmRegExp.ReplaceAllString(name, "-")
	imageName := strings.ToLower(fmt.Sprintf("%s-%s", saniName, hash))

	// Check that name complies with Docker's repository naming rules
	if !imageRegExp.MatchString(imageName) {
		dockerLogger.Errorf("Error constructing Docker VM Name. '%s' breaks Docker's repository naming rules", name)
		return "", fmt.Errorf("Error constructing Docker VM Name. '%s' breaks Docker's repository naming rules", imageName)
	}

	return imageName, nil
}

func (vm *DockerVM) preFormatImageName(ccid ccintf.CCID) string {
	name := ccid.String()

	if vm.NetworkID != "" && vm.PeerID != "" {
		name = fmt.Sprintf("%s-%s-%s", vm.NetworkID, vm.PeerID, name)
	} else if vm.NetworkID != "" {
		name = fmt.Sprintf("%s-%s", vm.NetworkID, name)
	} else if vm.PeerID != "" {
		name = fmt.Sprintf("%s-%s", vm.PeerID, name)
	}

	return name
}
