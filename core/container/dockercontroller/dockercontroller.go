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
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

var (
	dockerLogger = flogging.MustGetLogger("dockercontroller")
	vmRegExp     = regexp.MustCompile("[^a-zA-Z0-9-_.]")
	imageRegExp  = regexp.MustCompile("^[a-z0-9]+(([._-][a-z0-9]+)+)?$")
)

//go:generate counterfeiter -o mock/dockerclient.go --fake-name DockerClient . dockerClient

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
	// InspectImage returns an image by its name or ID.
	InspectImage(imageName string) (*docker.Image, error)
}

type PlatformBuilder interface {
	GenerateDockerBuild(ccType, path string, codePackage io.Reader) (io.Reader, error)
}

type ContainerInstance struct {
	CCID     string
	Type     string
	DockerVM *DockerVM
}

func (ci *ContainerInstance) Start(peerConnection *ccintf.PeerConnection) error {
	return ci.DockerVM.Start(ci.CCID, ci.Type, peerConnection)
}

func (ci *ContainerInstance) ChaincodeServerInfo() (*ccintf.ChaincodeServerInfo, error) {
	return nil, nil
}

func (ci *ContainerInstance) Stop() error {
	return ci.DockerVM.Stop(ci.CCID)
}

func (ci *ContainerInstance) Wait() (int, error) {
	return ci.DockerVM.Wait(ci.CCID)
}

// DockerVM is a vm. It is identified by an image id
type DockerVM struct {
	PeerID          string
	NetworkID       string
	BuildMetrics    *BuildMetrics
	HostConfig      *docker.HostConfig
	Client          dockerClient
	AttachStdOut    bool
	ChaincodePull   bool
	NetworkMode     string
	PlatformBuilder PlatformBuilder
	LoggingEnv      []string
	MSPID           string
}

// HealthCheck checks if the DockerVM is able to communicate with the Docker
// daemon.
func (vm *DockerVM) HealthCheck(ctx context.Context) error {
	if err := vm.Client.PingWithContext(ctx); err != nil {
		return errors.Wrap(err, "failed to ping to Docker daemon")
	}
	return nil
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

func (vm *DockerVM) buildImage(ccid string, reader io.Reader) error {
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
		"chaincode", ccid,
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

// Build is responsible for building an image if it does not already exist.
func (vm *DockerVM) Build(ccid string, metadata *persistence.ChaincodePackageMetadata, codePackage io.Reader) (container.Instance, error) {
	imageName, err := vm.GetVMNameForDocker(ccid)
	if err != nil {
		return nil, err
	}

	// This is an awkward translation, but better here in a future dead path
	// than elsewhere.  The old enum types are capital, but at least as implemented
	// lifecycle tools seem to allow type to be set lower case.
	ccType := strings.ToUpper(metadata.Type)

	_, err = vm.Client.InspectImage(imageName)
	switch err {
	case docker.ErrNoSuchImage:
		dockerfileReader, err := vm.PlatformBuilder.GenerateDockerBuild(ccType, metadata.Path, codePackage)
		if err != nil {
			return nil, errors.Wrap(err, "platform builder failed")
		}
		err = vm.buildImage(ccid, dockerfileReader)
		if err != nil {
			return nil, errors.Wrap(err, "docker image build failed")
		}
	case nil:
	default:
		return nil, errors.Wrap(err, "docker image inspection failed")
	}

	return &ContainerInstance{
		DockerVM: vm,
		CCID:     ccid,
		Type:     ccType,
	}, nil
}

// In order to support starting chaincode containers built with Fabric v1.4 and earlier,
// we must check for the precense of the start.sh script for Node.js chaincode before
// attempting to call it.
var nodeStartScript = `
set -e
if [ -x /chaincode/start.sh ]; then
	/chaincode/start.sh --peer.address %[1]s
else
	cd /usr/local/src
	npm start -- --peer.address %[1]s
fi
`

func (vm *DockerVM) GetArgs(ccType string, peerAddress string) ([]string, error) {
	// language specific arguments, possibly should be pushed back into platforms, but were simply
	// ported from the container_runtime chaincode component
	switch ccType {
	case pb.ChaincodeSpec_GOLANG.String(), pb.ChaincodeSpec_CAR.String():
		return []string{"chaincode", fmt.Sprintf("-peer.address=%s", peerAddress)}, nil
	case pb.ChaincodeSpec_JAVA.String():
		return []string{"/root/chaincode-java/start", "--peerAddress", peerAddress}, nil
	case pb.ChaincodeSpec_NODE.String():
		return []string{"/bin/sh", "-c", fmt.Sprintf(nodeStartScript, peerAddress)}, nil
	default:
		return nil, errors.Errorf("unknown chaincodeType: %s", ccType)
	}
}

const (
	// Mutual TLS auth client key and cert paths in the chaincode container
	TLSClientKeyPath      string = "/etc/hyperledger/fabric/client.key"
	TLSClientCertPath     string = "/etc/hyperledger/fabric/client.crt"
	TLSClientKeyFile      string = "/etc/hyperledger/fabric/client_pem.key"
	TLSClientCertFile     string = "/etc/hyperledger/fabric/client_pem.crt"
	TLSClientRootCertFile string = "/etc/hyperledger/fabric/peer.crt"
)

func (vm *DockerVM) GetEnv(ccid string, tlsConfig *ccintf.TLSConfig) []string {
	// common environment variables
	// FIXME: we are using the env variable CHAINCODE_ID to store
	// the package ID; in the legacy lifecycle they used to be the
	// same but now they are not, so we should use a different env
	// variable. However chaincodes built by older versions of the
	// peer still adopt this broken convention. (FAB-14630)
	envs := []string{fmt.Sprintf("CORE_CHAINCODE_ID_NAME=%s", ccid)}
	envs = append(envs, vm.LoggingEnv...)

	// Pass TLS options to chaincode
	if tlsConfig != nil {
		envs = append(envs, "CORE_PEER_TLS_ENABLED=true")
		envs = append(envs, fmt.Sprintf("CORE_TLS_CLIENT_KEY_PATH=%s", TLSClientKeyPath))
		envs = append(envs, fmt.Sprintf("CORE_TLS_CLIENT_CERT_PATH=%s", TLSClientCertPath))
		envs = append(envs, fmt.Sprintf("CORE_TLS_CLIENT_KEY_FILE=%s", TLSClientKeyFile))
		envs = append(envs, fmt.Sprintf("CORE_TLS_CLIENT_CERT_FILE=%s", TLSClientCertFile))
		envs = append(envs, fmt.Sprintf("CORE_PEER_TLS_ROOTCERT_FILE=%s", TLSClientRootCertFile))
	} else {
		envs = append(envs, "CORE_PEER_TLS_ENABLED=false")
	}

	envs = append(envs, fmt.Sprintf("CORE_PEER_LOCALMSPID=%s", vm.MSPID))

	return envs
}

// Start starts a container using a previously created docker image
func (vm *DockerVM) Start(ccid string, ccType string, peerConnection *ccintf.PeerConnection) error {
	imageName, err := vm.GetVMNameForDocker(ccid)
	if err != nil {
		return err
	}

	containerName := vm.GetVMName(ccid)
	logger := dockerLogger.With("imageName", imageName, "containerName", containerName)

	vm.stopInternal(containerName)

	args, err := vm.GetArgs(ccType, peerConnection.Address)
	if err != nil {
		return errors.WithMessage(err, "could not get args")
	}
	dockerLogger.Debugf("start container with args: %s", strings.Join(args, " "))

	env := vm.GetEnv(ccid, peerConnection.TLSConfig)
	dockerLogger.Debugf("start container with env:\n\t%s", strings.Join(env, "\n\t"))

	err = vm.createContainer(imageName, containerName, args, env)
	if err != nil {
		logger.Errorf("create container failed: %s", err)
		return err
	}

	// stream stdout and stderr to chaincode logger
	if vm.AttachStdOut {
		containerLogger := flogging.MustGetLogger("peer.chaincode." + containerName)
		streamOutput(dockerLogger, vm.Client, containerName, containerLogger)
	}

	// upload TLS files to the container before starting it if needed
	if peerConnection.TLSConfig != nil {
		// the docker upload API takes a tar file, so we need to first
		// consolidate the file entries to a tar
		payload := bytes.NewBuffer(nil)
		gw := gzip.NewWriter(payload)
		tw := tar.NewWriter(gw)

		// Note, we goofily base64 encode 2 of the TLS artifacts but not the other for strange historical reasons
		err = addFiles(tw, map[string][]byte{
			TLSClientKeyPath:      []byte(base64.StdEncoding.EncodeToString(peerConnection.TLSConfig.ClientKey)),
			TLSClientCertPath:     []byte(base64.StdEncoding.EncodeToString(peerConnection.TLSConfig.ClientCert)),
			TLSClientKeyFile:      peerConnection.TLSConfig.ClientKey,
			TLSClientCertFile:     peerConnection.TLSConfig.ClientCert,
			TLSClientRootCertFile: peerConnection.TLSConfig.RootCert,
		})
		if err != nil {
			return fmt.Errorf("error writing files to upload to Docker instance into a temporary tar blob: %s", err)
		}

		// Write the tar file out
		if err := tw.Close(); err != nil {
			return fmt.Errorf("error writing files to upload to Docker instance into a temporary tar blob: %s", err)
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

func addFiles(tw *tar.Writer, contents map[string][]byte) error {
	for name, payload := range contents {
		err := tw.WriteHeader(&tar.Header{
			Name: name,
			Size: int64(len(payload)),
			Mode: 0o100644,
		})
		if err != nil {
			return err
		}

		_, err = tw.Write(payload)
		if err != nil {
			return err
		}
	}

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
			if len(line) > 0 {
				containerLogger.Info(line)
			}
			switch err {
			case nil:
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
func (vm *DockerVM) Stop(ccid string) error {
	id := vm.ccidToContainerID(ccid)
	return vm.stopInternal(id)
}

// Wait blocks until the container stops and returns the exit code of the container.
func (vm *DockerVM) Wait(ccid string) (int, error) {
	id := vm.ccidToContainerID(ccid)
	return vm.Client.WaitContainer(id)
}

func (vm *DockerVM) ccidToContainerID(ccid string) string {
	return strings.Replace(vm.GetVMName(ccid), ":", "_", -1)
}

func (vm *DockerVM) stopInternal(id string) error {
	logger := dockerLogger.With("id", id)

	logger.Debugw("stopping container")
	err := vm.Client.StopContainer(id, 0)
	dockerLogger.Debugw("stop container result", "error", err)

	logger.Debugw("killing container")
	err = vm.Client.KillContainer(docker.KillContainerOptions{ID: id})
	logger.Debugw("kill container result", "error", err)

	logger.Debugw("removing container")
	err = vm.Client.RemoveContainer(docker.RemoveContainerOptions{ID: id, Force: true})
	logger.Debugw("remove container result", "error", err)

	return err
}

// GetVMName generates the VM name from peer information. It accepts a format
// function parameter to allow different formatting based on the desired use of
// the name.
func (vm *DockerVM) GetVMName(ccid string) string {
	// replace any invalid characters with "-" (either in network id, peer id, or in the
	// entire name returned by any format function)
	return vmRegExp.ReplaceAllString(vm.preFormatImageName(ccid), "-")
}

// GetVMNameForDocker formats the docker image from peer information. This is
// needed to keep image (repository) names unique in a single host, multi-peer
// environment (such as a development environment). It computes the hash for the
// supplied image name and then appends it to the lowercase image name to ensure
// uniqueness.
func (vm *DockerVM) GetVMNameForDocker(ccid string) (string, error) {
	name := vm.preFormatImageName(ccid)
	// pre-2.0 used "-" as the separator in the ccid, so replace ":" with
	// "-" here to ensure 2.0 peers can find pre-2.0 cc images
	name = strings.ReplaceAll(name, ":", "-")
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

func (vm *DockerVM) preFormatImageName(ccid string) string {
	name := ccid

	if vm.NetworkID != "" && vm.PeerID != "" {
		name = fmt.Sprintf("%s-%s-%s", vm.NetworkID, vm.PeerID, name)
	} else if vm.NetworkID != "" {
		name = fmt.Sprintf("%s-%s", vm.NetworkID, name)
	} else if vm.PeerID != "" {
		name = fmt.Sprintf("%s-%s", vm.PeerID, name)
	}

	return name
}
