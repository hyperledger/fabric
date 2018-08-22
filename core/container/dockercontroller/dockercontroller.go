/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"bufio"

	"regexp"

	"github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/spf13/viper"
)

// ContainerType is the string which the docker container type
// is registered with the container.VMController
const ContainerType = "DOCKER"

var (
	dockerLogger = flogging.MustGetLogger("dockercontroller")
	hostConfig   *docker.HostConfig
	vmRegExp     = regexp.MustCompile("[^a-zA-Z0-9-_.]")
	imageRegExp  = regexp.MustCompile("^[a-z0-9]+(([._-][a-z0-9]+)+)?$")
)

// getClient returns an instance that implements dockerClient interface
type getClient func() (dockerClient, error)

//DockerVM is a vm. It is identified by an image id
type DockerVM struct {
	getClientFnc getClient
	PeerID       string
	NetworkID    string
}

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
	// RemoveImageExtended removes a docker image by its name or ID, returns an
	// error in case of failure
	RemoveImageExtended(id string, opts docker.RemoveImageOptions) error
	// StopContainer stops a docker container, killing it after the given timeout
	// (in seconds). Returns an error in case of failure
	StopContainer(id string, timeout uint) error
	// KillContainer sends a signal to a docker container, returns an error in
	// case of failure
	KillContainer(opts docker.KillContainerOptions) error
	// RemoveContainer removes a docker container, returns an error in case of failure
	RemoveContainer(opts docker.RemoveContainerOptions) error
}

// Controller implements container.VMProvider
type Provider struct {
	PeerID    string
	NetworkID string
}

// NewProvider creates a new instance of Provider
func NewProvider(peerID, networkID string) *Provider {
	return &Provider{
		PeerID:    peerID,
		NetworkID: networkID,
	}
}

// NewVM creates a new DockerVM instance
func (p *Provider) NewVM() container.VM {
	return NewDockerVM(p.PeerID, p.NetworkID)
}

// NewDockerVM returns a new DockerVM instance
func NewDockerVM(peerID, networkID string) *DockerVM {
	vm := DockerVM{
		PeerID:    peerID,
		NetworkID: networkID,
	}
	vm.getClientFnc = getDockerClient
	return &vm
}

func getDockerClient() (dockerClient, error) {
	return cutil.NewDockerClient()
}

func getDockerHostConfig() *docker.HostConfig {
	if hostConfig != nil {
		return hostConfig
	}
	dockerKey := func(key string) string {
		return "vm.docker.hostConfig." + key
	}
	getInt64 := func(key string) int64 {
		defer func() {
			if err := recover(); err != nil {
				dockerLogger.Warningf("load vm.docker.hostConfig.%s failed, error: %v", key, err)
			}
		}()
		n := viper.GetInt(dockerKey(key))
		return int64(n)
	}

	var logConfig docker.LogConfig
	err := viper.UnmarshalKey(dockerKey("LogConfig"), &logConfig)
	if err != nil {
		dockerLogger.Warningf("load docker HostConfig.LogConfig failed, error: %s", err.Error())
	}
	networkMode := viper.GetString(dockerKey("NetworkMode"))
	if networkMode == "" {
		networkMode = "host"
	}
	dockerLogger.Debugf("docker container hostconfig NetworkMode: %s", networkMode)

	hostConfig = &docker.HostConfig{
		CapAdd:  viper.GetStringSlice(dockerKey("CapAdd")),
		CapDrop: viper.GetStringSlice(dockerKey("CapDrop")),

		DNS:         viper.GetStringSlice(dockerKey("Dns")),
		DNSSearch:   viper.GetStringSlice(dockerKey("DnsSearch")),
		ExtraHosts:  viper.GetStringSlice(dockerKey("ExtraHosts")),
		NetworkMode: networkMode,
		IpcMode:     viper.GetString(dockerKey("IpcMode")),
		PidMode:     viper.GetString(dockerKey("PidMode")),
		UTSMode:     viper.GetString(dockerKey("UTSMode")),
		LogConfig:   logConfig,

		ReadonlyRootfs:   viper.GetBool(dockerKey("ReadonlyRootfs")),
		SecurityOpt:      viper.GetStringSlice(dockerKey("SecurityOpt")),
		CgroupParent:     viper.GetString(dockerKey("CgroupParent")),
		Memory:           getInt64("Memory"),
		MemorySwap:       getInt64("MemorySwap"),
		MemorySwappiness: getInt64("MemorySwappiness"),
		OOMKillDisable:   viper.GetBool(dockerKey("OomKillDisable")),
		CPUShares:        getInt64("CpuShares"),
		CPUSet:           viper.GetString(dockerKey("Cpuset")),
		CPUSetCPUs:       viper.GetString(dockerKey("CpusetCPUs")),
		CPUSetMEMs:       viper.GetString(dockerKey("CpusetMEMs")),
		CPUQuota:         getInt64("CpuQuota"),
		CPUPeriod:        getInt64("CpuPeriod"),
		BlkioWeight:      getInt64("BlkioWeight"),
	}

	return hostConfig
}

func (vm *DockerVM) createContainer(client dockerClient,
	imageID string, containerID string, args []string,
	env []string, attachStdout bool) error {
	config := docker.Config{Cmd: args, Image: imageID, Env: env, AttachStdout: attachStdout, AttachStderr: attachStdout}
	copts := docker.CreateContainerOptions{Name: containerID, Config: &config, HostConfig: getDockerHostConfig()}
	dockerLogger.Debugf("Create container: %s", containerID)
	_, err := client.CreateContainer(copts)
	if err != nil {
		return err
	}
	dockerLogger.Debugf("Created container: %s", imageID)
	return nil
}

func (vm *DockerVM) deployImage(client dockerClient, ccid ccintf.CCID,
	args []string, env []string, reader io.Reader) error {
	id, err := vm.GetVMNameForDocker(ccid)
	if err != nil {
		return err
	}
	outputbuf := bytes.NewBuffer(nil)
	opts := docker.BuildImageOptions{
		Name:         id,
		Pull:         viper.GetBool("chaincode.pull"),
		InputStream:  reader,
		OutputStream: outputbuf,
	}

	if err := client.BuildImage(opts); err != nil {
		dockerLogger.Errorf("Error building images: %s", err)
		dockerLogger.Errorf("Image Output:\n********************\n%s\n********************", outputbuf.String())
		return err
	}

	dockerLogger.Debugf("Created image: %s", id)

	return nil
}

//Start starts a container using a previously created docker image
func (vm *DockerVM) Start(ccid ccintf.CCID,
	args []string, env []string, filesToUpload map[string][]byte, builder container.Builder) error {
	imageName, err := vm.GetVMNameForDocker(ccid)
	if err != nil {
		return err
	}

	client, err := vm.getClientFnc()
	if err != nil {
		dockerLogger.Debugf("start - cannot create client %s", err)
		return err
	}

	containerName := vm.GetVMName(ccid)

	attachStdout := viper.GetBool("vm.docker.attachStdout")

	//stop,force remove if necessary
	dockerLogger.Debugf("Cleanup container %s", containerName)
	vm.stopInternal(client, containerName, 0, false, false)

	dockerLogger.Debugf("Start container %s", containerName)
	err = vm.createContainer(client, imageName, containerName, args, env, attachStdout)
	if err != nil {
		//if image not found try to create image and retry
		if err == docker.ErrNoSuchImage {
			if builder != nil {
				dockerLogger.Debugf("start-could not find image <%s> (container id <%s>), because of <%s>..."+
					"attempt to recreate image", imageName, containerName, err)

				reader, err1 := builder.Build()
				if err1 != nil {
					dockerLogger.Errorf("Error creating image builder for image <%s> (container id <%s>), "+
						"because of <%s>", imageName, containerName, err1)
				}

				if err1 = vm.deployImage(client, ccid, args, env, reader); err1 != nil {
					return err1
				}

				dockerLogger.Debug("start-recreated image successfully")
				if err1 = vm.createContainer(client, imageName, containerName, args, env, attachStdout); err1 != nil {
					dockerLogger.Errorf("start-could not recreate container post recreate image: %s", err1)
					return err1
				}
			} else {
				dockerLogger.Errorf("start-could not find image <%s>, because of %s", imageName, err)
				return err
			}
		} else {
			dockerLogger.Errorf("start-could not recreate container <%s>, because of %s", containerName, err)
			return err
		}
	}

	if attachStdout {
		// Launch a few go-threads to manage output streams from the container.
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
			// Block here until the attachment completes or we timeout
			select {
			case <-attached:
				// successful attach
			case <-time.After(10 * time.Second):
				dockerLogger.Errorf("Timeout while attaching to IO channel in container %s", containerName)
				return
			}

			// Acknowledge the attachment?  This was included in the gist I followed
			// (http://bit.ly/2jBrCtM).  Not sure it's actually needed but it doesn't
			// appear to hurt anything.
			attached <- struct{}{}

			// Establish a buffer for our IO channel so that we may do readline-style
			// ingestion of the IO, one log entry per line
			is := bufio.NewReader(r)

			// Acquire a custom logger for our chaincode, inheriting the level from the peer
			containerLogger := flogging.MustGetLogger(containerName)
			flogging.SetModuleLevel(flogging.GetModuleLevel("peer"), containerName)

			for {
				// Loop forever dumping lines of text into the containerLogger
				// until the pipe is closed
				line, err2 := is.ReadString('\n')
				if err2 != nil {
					switch err2 {
					case io.EOF:
						dockerLogger.Infof("Container %s has closed its IO channel", containerName)
					default:
						dockerLogger.Errorf("Error reading container output: %s", err2)
					}

					return
				}

				containerLogger.Info(line)
			}
		}()
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
		if err = tw.Close(); err != nil {
			return fmt.Errorf("Error writing files to upload to Docker instance into a temporary tar blob: %s", err)
		}

		gw.Close()

		err = client.UploadToContainer(containerName, docker.UploadToContainerOptions{
			InputStream:          bytes.NewReader(payload.Bytes()),
			Path:                 "/",
			NoOverwriteDirNonDir: false,
		})

		if err != nil {
			return fmt.Errorf("Error uploading files to the container instance %s: %s", containerName, err)
		}
	}

	// start container with HostConfig was deprecated since v1.10 and removed in v1.2
	err = client.StartContainer(containerName, nil)
	if err != nil {
		dockerLogger.Errorf("start-could not start container: %s", err)
		return err
	}

	dockerLogger.Debugf("Started container %s", containerName)
	return nil
}

//Stop stops a running chaincode
func (vm *DockerVM) Stop(ccid ccintf.CCID, timeout uint, dontkill bool, dontremove bool) error {
	id := vm.GetVMName(ccid)

	client, err := vm.getClientFnc()
	if err != nil {
		dockerLogger.Debugf("stop - cannot create client %s", err)
		return err
	}
	id = strings.Replace(id, ":", "_", -1)

	err = vm.stopInternal(client, id, timeout, dontkill, dontremove)

	return err
}

func (vm *DockerVM) stopInternal(client dockerClient,
	id string, timeout uint, dontkill bool, dontremove bool) error {
	err := client.StopContainer(id, timeout)
	if err != nil {
		dockerLogger.Debugf("Stop container %s(%s)", id, err)
	} else {
		dockerLogger.Debugf("Stopped container %s", id)
	}
	if !dontkill {
		err = client.KillContainer(docker.KillContainerOptions{ID: id})
		if err != nil {
			dockerLogger.Debugf("Kill container %s (%s)", id, err)
		} else {
			dockerLogger.Debugf("Killed container %s", id)
		}
	}
	if !dontremove {
		err = client.RemoveContainer(docker.RemoveContainerOptions{ID: id, Force: true})
		if err != nil {
			dockerLogger.Debugf("Remove container %s (%s)", id, err)
		} else {
			dockerLogger.Debugf("Removed container %s", id)
		}
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
		return imageName, fmt.Errorf("Error constructing Docker VM Name. '%s' breaks Docker's repository naming rules", imageName)
	}

	return imageName, nil
}

func (vm *DockerVM) preFormatImageName(ccid ccintf.CCID) string {
	name := ccid.GetName()

	if vm.NetworkID != "" && vm.PeerID != "" {
		name = fmt.Sprintf("%s-%s-%s", vm.NetworkID, vm.PeerID, name)
	} else if vm.NetworkID != "" {
		name = fmt.Sprintf("%s-%s", vm.NetworkID, name)
	} else if vm.PeerID != "" {
		name = fmt.Sprintf("%s-%s", vm.PeerID, name)
	}

	return name
}
