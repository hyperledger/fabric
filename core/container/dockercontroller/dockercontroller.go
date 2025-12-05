/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller

import (
	"archive/tar"
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

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	dcontainer "github.com/moby/moby/api/types/container"
	dcli "github.com/moby/moby/client"
	"github.com/pkg/errors"
)

var (
	dockerLogger = flogging.MustGetLogger("dockercontroller")
	vmRegExp     = regexp.MustCompile("[^a-zA-Z0-9-_.]")
	imageRegExp  = regexp.MustCompile("^[a-z0-9]+(([._-][a-z0-9]+)+)?$")
)

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
	HostConfig      *dcontainer.HostConfig
	Client          dcli.APIClient
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
	if _, err := vm.Client.Ping(ctx, dcli.PingOptions{}); err != nil {
		return errors.Wrap(err, "failed to ping to Docker daemon")
	}
	return nil
}

func (vm *DockerVM) createContainer(imageID, containerID string, args, env []string) error {
	logger := dockerLogger.With("imageID", imageID, "containerID", containerID)
	logger.Debugw("create container")
	_, err := vm.Client.ContainerCreate(context.Background(), dcli.ContainerCreateOptions{
		Config: &dcontainer.Config{
			AttachStdout: vm.AttachStdOut,
			AttachStderr: vm.AttachStdOut,
			Env:          env,
			Cmd:          args,
			Image:        imageID,
		},
		HostConfig: vm.HostConfig,
		Name:       containerID,
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

	startTime := time.Now()
	res, err := vm.Client.ImageBuild(context.Background(), reader, dcli.ImageBuildOptions{
		Tags:        []string{id},
		PullParent:  true,
		NetworkMode: vm.NetworkMode,
	})

	vm.BuildMetrics.ChaincodeImageBuildDuration.With(
		"chaincode", ccid,
		"success", strconv.FormatBool(err == nil),
	).Observe(time.Since(startTime).Seconds())

	if err != nil {
		dockerLogger.Errorf("Error building image: %s", err)
		return err
	}
	defer res.Body.Close()

	var buf bytes.Buffer
	io.Copy(&buf, res.Body)

	dockerLogger.Debugf("Created image: %s, output: %s", id, buf.String())
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

	_, err = vm.Client.ImageInspect(context.Background(), imageName)
	if err != nil && strings.Contains(err.Error(), "No such image") {
		dockerfileReader, err := vm.PlatformBuilder.GenerateDockerBuild(ccType, metadata.Path, codePackage)
		if err != nil {
			return nil, errors.Wrap(err, "platform builder failed")
		}
		err = vm.buildImage(ccid, dockerfileReader)
		if err != nil {
			return nil, errors.Wrap(err, "docker image build failed")
		}
	} else if err != nil {
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
		if err = tw.Close(); err != nil {
			return fmt.Errorf("error writing files to upload to Docker instance into a temporary tar blob: %s", err)
		}

		gw.Close()

		_, err = vm.Client.CopyToContainer(context.Background(), containerName, dcli.CopyToContainerOptions{
			DestinationPath:           "/",
			Content:                   bytes.NewReader(payload.Bytes()),
			AllowOverwriteDirWithFile: true,
		})
		if err != nil {
			return fmt.Errorf("Error uploading files to the container instance %s: %s", containerName, err)
		}
	}

	// start container with HostConfig was deprecated since v1.10 and removed in v1.2
	_, err = vm.Client.ContainerStart(context.Background(), containerName, dcli.ContainerStartOptions{})
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
func streamOutput(logger *flogging.FabricLogger, client dcli.APIClient, containerName string, containerLogger *flogging.FabricLogger) {
	// Launch a few go routines to manage output streams from the container.
	// They will be automatically destroyed when the container exits
	go func() {
		res, err := client.ContainerAttach(context.Background(), containerName, dcli.ContainerAttachOptions{
			Stream: true,
			Stdout: true,
			Stderr: true,
			Logs:   true,
		})
		if err != nil {
			logger.Errorf("error attaching to IO channel in container %s", containerName)
			return
		}
		defer res.Close()

		for {
			// Loop forever dumping lines of text into the containerLogger
			// until the pipe is closed
			line, err := res.Reader.ReadString('\n')
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
	resWait := vm.Client.ContainerWait(context.Background(), id, dcli.ContainerWaitOptions{})
	select {
	case res := <-resWait.Result:
		return int(res.StatusCode), nil
	case err := <-resWait.Error:
		return 0, err
	}
}

func (vm *DockerVM) ccidToContainerID(ccid string) string {
	return strings.Replace(vm.GetVMName(ccid), ":", "_", -1)
}

func (vm *DockerVM) stopInternal(id string) error {
	logger := dockerLogger.With("id", id)

	logger.Debugw("stopping container")
	t := 0
	_, err := vm.Client.ContainerStop(context.Background(), id, dcli.ContainerStopOptions{
		Timeout: &t,
	})
	dockerLogger.Debugw("stop container result", "error", err)

	logger.Debugw("killing container")
	_, err = vm.Client.ContainerKill(context.Background(), id, dcli.ContainerKillOptions{})
	logger.Debugw("kill container result", "error", err)

	logger.Debugw("removing container")
	_, err = vm.Client.ContainerRemove(context.Background(), id, dcli.ContainerRemoveOptions{
		Force: true,
	})
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
