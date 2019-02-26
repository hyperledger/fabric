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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/container/ccintf"
	coreutil "github.com/hyperledger/fabric/core/testutil"
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test used to be part of an integration style test in core/container, moved to here
func TestIntegrationPath(t *testing.T) {
	coreutil.SetupTestConfig()
	dc := NewDockerVM("", util.GenerateUUID(), NewBuildMetrics(&disabled.Provider{}))
	ccid := ccintf.CCID{Name: "simple"}

	err := dc.Start(ccid, nil, nil, nil, InMemBuilder{})
	require.NoError(t, err)

	// Stop, killing, and deleting
	err = dc.Stop(ccid, 0, true, true)
	require.NoError(t, err)

	err = dc.Start(ccid, nil, nil, nil, nil)
	require.NoError(t, err)

	// Stop, killing, but not deleting
	_ = dc.Stop(ccid, 0, false, true)
}

func TestHostConfig(t *testing.T) {
	coreutil.SetupTestConfig()
	var hostConfig = new(docker.HostConfig)
	err := viper.UnmarshalKey("vm.docker.hostConfig", hostConfig)
	if err != nil {
		t.Fatalf("Load docker HostConfig wrong, error: %s", err.Error())
	}
	assert.NotNil(t, hostConfig.LogConfig)
	assert.Equal(t, "json-file", hostConfig.LogConfig.Type)
	assert.Equal(t, "50m", hostConfig.LogConfig.Config["max-size"])
	assert.Equal(t, "5", hostConfig.LogConfig.Config["max-file"])
}

func TestGetDockerHostConfig(t *testing.T) {
	coreutil.SetupTestConfig()
	hostConfig = nil // There is a cached global singleton for docker host config, the other tests can collide with
	hostConfig := getDockerHostConfig()
	assert.NotNil(t, hostConfig)
	assert.Equal(t, "host", hostConfig.NetworkMode)
	assert.Equal(t, "json-file", hostConfig.LogConfig.Type)
	assert.Equal(t, "50m", hostConfig.LogConfig.Config["max-size"])
	assert.Equal(t, "5", hostConfig.LogConfig.Config["max-file"])
	assert.Equal(t, int64(1024*1024*1024*2), hostConfig.Memory)
	assert.Equal(t, int64(0), hostConfig.CPUShares)
}

func Test_Start(t *testing.T) {
	gt := NewGomegaWithT(t)
	dvm := DockerVM{
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
	}
	ccid := ccintf.CCID{
		Name:    "simple",
		Version: "1.0",
	}
	args := make([]string, 1)
	env := make([]string, 1)
	files := map[string][]byte{
		"hello": []byte("world"),
	}

	// Failure cases
	// case 1: getMockClient returns error
	dvm.getClientFnc = getMockClient
	getClientErr = true
	err := dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).To(HaveOccurred())
	getClientErr = false

	// case 2: dockerClient.CreateContainer returns error
	createErr = true
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).To(HaveOccurred())
	createErr = false

	// case 3: dockerClient.UploadToContainer returns error
	uploadErr = true
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).To(HaveOccurred())
	uploadErr = false

	// case 4: dockerClient.StartContainer returns docker.noSuchImgErr, BuildImage fails
	noSuchImgErr = true
	buildErr = true
	err = dvm.Start(ccid, args, env, files, &mockBuilder{buildFunc: func() (io.Reader, error) { return &bytes.Buffer{}, nil }})
	gt.Expect(err).To(HaveOccurred())
	buildErr = false

	chaincodePath := "github.com/hyperledger/fabric/examples/chaincode/go/example01/cmd"
	spec := &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_GOLANG,
		ChaincodeId: &pb.ChaincodeID{Name: "ex01", Path: chaincodePath},
		Input:       &pb.ChaincodeInput{Args: util.ToChaincodeArgs("f")},
	}
	codePackage, err := platforms.NewRegistry(&golang.Platform{}).GetDeploymentPayload(spec.CCType(), spec.Path())
	if err != nil {
		t.Fatal()
	}
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackage}
	bldr := &mockBuilder{
		buildFunc: func() (io.Reader, error) {
			return platforms.NewRegistry(&golang.Platform{}).GenerateDockerBuild(
				cds.CCType(),
				cds.Path(),
				cds.Name(),
				cds.Version(),
				cds.Bytes(),
			)
		},
	}

	// case 5: start called and dockerClient.CreateContainer returns
	// docker.noSuchImgErr and dockerClient.Start returns error
	viper.Set("vm.docker.attachStdout", true)
	startErr = true
	err = dvm.Start(ccid, args, env, files, bldr)
	gt.Expect(err).To(HaveOccurred())
	startErr = false

	// Success cases
	err = dvm.Start(ccid, args, env, files, bldr)
	gt.Expect(err).NotTo(HaveOccurred())
	noSuchImgErr = false

	// dockerClient.StopContainer returns error
	stopErr = true
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	stopErr = false

	// dockerClient.KillContainer returns error
	killErr = true
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	killErr = false

	// dockerClient.RemoveContainer returns error
	removeErr = true
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	removeErr = false

	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())
}

func Test_streamOutput(t *testing.T) {
	gt := NewGomegaWithT(t)

	logger, recorder := floggingtest.NewTestLogger(t)
	containerLogger, containerRecorder := floggingtest.NewTestLogger(t)

	client := &mockClient{}
	errCh := make(chan error, 1)
	optsCh := make(chan docker.AttachToContainerOptions, 1)
	client.attachToContainerStub = func(opts docker.AttachToContainerOptions) error {
		optsCh <- opts
		return <-errCh
	}

	streamOutput(logger, client, "container-name", containerLogger)

	var opts docker.AttachToContainerOptions
	gt.Eventually(optsCh).Should(Receive(&opts))
	gt.Eventually(opts.Success).Should(BeSent(struct{}{}))
	gt.Eventually(opts.Success).Should(BeClosed())

	fmt.Fprintf(opts.OutputStream, "message-one\n")
	fmt.Fprintf(opts.OutputStream, "message-two") // does not get written
	gt.Eventually(containerRecorder).Should(gbytes.Say("message-one"))
	gt.Consistently(containerRecorder.Entries).Should(HaveLen(1))

	close(errCh)
	gt.Eventually(recorder).Should(gbytes.Say("Container container-name has closed its IO channel"))
	gt.Consistently(recorder.Entries).Should(HaveLen(1))
	gt.Consistently(containerRecorder.Entries).Should(HaveLen(1))
}

func Test_BuildMetric(t *testing.T) {
	ccid := ccintf.CCID{Name: "simple", Version: "1.0"}
	client := &mockClient{}

	tests := []struct {
		desc           string
		buildErr       bool
		expectedLabels []string
	}{
		{desc: "success", buildErr: false, expectedLabels: []string{"chaincode", "simple:1.0", "success", "true"}},
		{desc: "failure", buildErr: true, expectedLabels: []string{"chaincode", "simple:1.0", "success", "false"}},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			gt := NewGomegaWithT(t)
			fakeChaincodeImageBuildDuration := &metricsfakes.Histogram{}
			fakeChaincodeImageBuildDuration.WithReturns(fakeChaincodeImageBuildDuration)
			dvm := DockerVM{
				BuildMetrics: &BuildMetrics{
					ChaincodeImageBuildDuration: fakeChaincodeImageBuildDuration,
				},
			}

			buildErr = tt.buildErr
			dvm.deployImage(client, ccid, &bytes.Buffer{})

			gt.Expect(fakeChaincodeImageBuildDuration.WithCallCount()).To(Equal(1))
			gt.Expect(fakeChaincodeImageBuildDuration.WithArgsForCall(0)).To(Equal(tt.expectedLabels))
			gt.Expect(fakeChaincodeImageBuildDuration.ObserveArgsForCall(0)).NotTo(BeZero())
			gt.Expect(fakeChaincodeImageBuildDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
		})
	}

	buildErr = false
}

func Test_Stop(t *testing.T) {
	dvm := DockerVM{}
	ccid := ccintf.CCID{Name: "simple"}

	// Failure case: getMockClient returns error
	getClientErr = true
	dvm.getClientFnc = getMockClient
	err := dvm.Stop(ccid, 10, true, true)
	assert.Error(t, err)
	getClientErr = false

	// Success case
	err = dvm.Stop(ccid, 10, true, true)
	assert.NoError(t, err)
}

func Test_Wait(t *testing.T) {
	dvm := DockerVM{}

	// failure to get a client
	dvm.getClientFnc = func() (dockerClient, error) {
		return nil, errors.New("gorilla-goo")
	}
	_, err := dvm.Wait(ccintf.CCID{})
	assert.EqualError(t, err, "gorilla-goo")

	// happy path
	client := &mockClient{}
	dvm.getClientFnc = func() (dockerClient, error) { return client, nil }

	client.exitCode = 99
	exitCode, err := dvm.Wait(ccintf.CCID{Name: "the-name", Version: "the-version"})
	assert.NoError(t, err)
	assert.Equal(t, 99, exitCode)
	assert.Equal(t, "the-name-the-version", client.containerID)

	// wait fails
	client.waitErr = errors.New("no-wait-for-you")
	_, err = dvm.Wait(ccintf.CCID{})
	assert.EqualError(t, err, "no-wait-for-you")
}

func Test_HealthCheck(t *testing.T) {
	dvm := DockerVM{}

	dvm.getClientFnc = func() (dockerClient, error) {
		client := &mockClient{
			pingErr: false,
		}
		return client, nil
	}
	err := dvm.HealthCheck(context.Background())
	assert.NoError(t, err)

	dvm.getClientFnc = func() (dockerClient, error) {
		client := &mockClient{
			pingErr: true,
		}
		return client, nil
	}
	err = dvm.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Error pinging daemon")
}

type testCase struct {
	name           string
	vm             *DockerVM
	ccid           ccintf.CCID
	expectedOutput string
}

func TestGetVMNameForDocker(t *testing.T) {
	tc := []testCase{
		{
			name:           "mycc",
			vm:             &DockerVM{NetworkID: "dev", PeerID: "peer0"},
			ccid:           ccintf.CCID{Name: "mycc", Version: "1.0"},
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-peer0-mycc-1.0")))),
		},
		{
			name:           "mycc-nonetworkid",
			vm:             &DockerVM{PeerID: "peer1"},
			ccid:           ccintf.CCID{Name: "mycc", Version: "1.0"},
			expectedOutput: fmt.Sprintf("%s-%s", "peer1-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("peer1-mycc-1.0")))),
		},
		{
			name:           "myCC-UCids",
			vm:             &DockerVM{NetworkID: "Dev", PeerID: "Peer0"},
			ccid:           ccintf.CCID{Name: "myCC", Version: "1.0"},
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("Dev-Peer0-myCC-1.0")))),
		},
		{
			name:           "myCC-idsWithSpecialChars",
			vm:             &DockerVM{NetworkID: "Dev$dev", PeerID: "Peer*0"},
			ccid:           ccintf.CCID{Name: "myCC", Version: "1.0"},
			expectedOutput: fmt.Sprintf("%s-%s", "dev-dev-peer-0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("Dev$dev-Peer*0-myCC-1.0")))),
		},
		{
			name:           "mycc-nopeerid",
			vm:             &DockerVM{NetworkID: "dev"},
			ccid:           ccintf.CCID{Name: "mycc", Version: "1.0"},
			expectedOutput: fmt.Sprintf("%s-%s", "dev-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-mycc-1.0")))),
		},
		{
			name:           "myCC-LCids",
			vm:             &DockerVM{NetworkID: "dev", PeerID: "peer0"},
			ccid:           ccintf.CCID{Name: "myCC", Version: "1.0"},
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-peer0-myCC-1.0")))),
		},
	}

	for _, test := range tc {
		name, err := test.vm.GetVMNameForDocker(test.ccid)
		assert.Nil(t, err, "Expected nil error")
		assert.Equal(t, test.expectedOutput, name, "Unexpected output for test case name: %s", test.name)
	}

}

func TestGetVMName(t *testing.T) {
	tc := []testCase{
		{
			name:           "myCC-preserveCase",
			vm:             &DockerVM{NetworkID: "Dev", PeerID: "Peer0"},
			ccid:           ccintf.CCID{Name: "myCC", Version: "1.0"},
			expectedOutput: fmt.Sprintf("%s", "Dev-Peer0-myCC-1.0"),
		},
	}

	for _, test := range tc {
		name := test.vm.GetVMName(test.ccid)
		assert.Equal(t, test.expectedOutput, name, "Unexpected output for test case name: %s", test.name)
	}

}

/*func TestFormatImageName_invalidChars(t *testing.T) {
	_, err := formatImageName("invalid*chars")
	assert.NotNil(t, err, "Expected error")
}*/

type InMemBuilder struct{}

func (imb InMemBuilder) Build() (io.Reader, error) {
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "FROM busybox:latest")
	fmt.Fprintln(buf, `CMD ["tail", "-f", "/dev/null"]`)

	startTime := time.Now()
	inputbuf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(inputbuf)
	tr := tar.NewWriter(gw)
	tr.WriteHeader(&tar.Header{
		Name:       "Dockerfile",
		Size:       int64(buf.Len()),
		ModTime:    startTime,
		AccessTime: startTime,
		ChangeTime: startTime,
	})
	tr.Write(buf.Bytes())
	tr.Close()
	gw.Close()
	return inputbuf, nil
}

func getMockClient() (dockerClient, error) {
	if getClientErr {
		return nil, errors.New("Failed to get client")
	}
	return &mockClient{noSuchImgErrReturned: false}, nil
}

type mockBuilder struct {
	buildFunc func() (io.Reader, error)
}

func (m *mockBuilder) Build() (io.Reader, error) {
	return m.buildFunc()
}

type mockClient struct {
	noSuchImgErrReturned bool
	pingErr              bool

	containerID string
	exitCode    int
	waitErr     error

	attachToContainerStub func(docker.AttachToContainerOptions) error
}

var getClientErr, createErr, uploadErr, noSuchImgErr, buildErr, removeImgErr,
	startErr, stopErr, killErr, removeErr bool

func (c *mockClient) CreateContainer(options docker.CreateContainerOptions) (*docker.Container, error) {
	if createErr {
		return nil, errors.New("Error creating the container")
	}
	if noSuchImgErr && !c.noSuchImgErrReturned {
		c.noSuchImgErrReturned = true
		return nil, docker.ErrNoSuchImage
	}
	return &docker.Container{}, nil
}

func (c *mockClient) StartContainer(id string, cfg *docker.HostConfig) error {
	if startErr {
		return errors.New("Error starting the container")
	}
	return nil
}

func (c *mockClient) UploadToContainer(id string, opts docker.UploadToContainerOptions) error {
	if uploadErr {
		return errors.New("Error uploading archive to the container")
	}
	return nil
}

func (c *mockClient) AttachToContainer(opts docker.AttachToContainerOptions) error {
	if c.attachToContainerStub != nil {
		return c.attachToContainerStub(opts)
	}
	if opts.Success != nil {
		opts.Success <- struct{}{}
	}
	return nil
}

func (c *mockClient) BuildImage(opts docker.BuildImageOptions) error {
	if buildErr {
		return errors.New("Error building image")
	}
	return nil
}

func (c *mockClient) RemoveImageExtended(id string, opts docker.RemoveImageOptions) error {
	if removeImgErr {
		return errors.New("Error removing extended image")
	}
	return nil
}

func (c *mockClient) StopContainer(id string, timeout uint) error {
	if stopErr {
		return errors.New("Error stopping container")
	}
	return nil
}

func (c *mockClient) KillContainer(opts docker.KillContainerOptions) error {
	if killErr {
		return errors.New("Error killing container")
	}
	return nil
}

func (c *mockClient) RemoveContainer(opts docker.RemoveContainerOptions) error {
	if removeErr {
		return errors.New("Error removing container")
	}
	return nil
}

func (c *mockClient) PingWithContext(context.Context) error {
	if c.pingErr {
		return errors.New("Error pinging daemon")
	}
	return nil
}

func (c *mockClient) WaitContainer(id string) (int, error) {
	c.containerID = id
	return c.exitCode, c.waitErr
}
