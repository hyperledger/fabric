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
	"github.com/hyperledger/fabric/core/container/dockercontroller/mock"
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

	client := &mock.DockerClient{}
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
	// case 1: getClient returns error
	dvm.getClientFnc = func() (dockerClient, error) {
		return nil, errors.New("failed to get Docker client")
	}
	err := dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).To(HaveOccurred())

	dvm.getClientFnc = func() (dockerClient, error) {
		return client, nil
	}

	// case 2: dockerClient.CreateContainer returns error
	client.CreateContainerReturns(nil, errors.New("create failed"))
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).To(HaveOccurred())
	client.CreateContainerReturns(&docker.Container{}, nil)

	// case 3: dockerClient.UploadToContainer returns error
	client.UploadToContainerReturns(errors.New("upload failed"))
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).To(HaveOccurred())

	client.UploadToContainerReturns(nil)

	// case 4: dockerClient.StartContainer returns docker.noSuchImgErr, BuildImage fails
	client.StartContainerReturns(docker.ErrNoSuchImage)
	client.BuildImageReturns(errors.New("build failed"))
	err = dvm.Start(ccid, args, env, files, &mockBuilder{buildFunc: func() (io.Reader, error) { return &bytes.Buffer{}, nil }})
	gt.Expect(err).To(HaveOccurred())

	client.BuildImageReturns(nil)

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
	err = dvm.Start(ccid, args, env, files, bldr)
	gt.Expect(err).To(HaveOccurred())

	client.StartContainerReturns(nil)

	// Success cases
	err = dvm.Start(ccid, args, env, files, bldr)
	gt.Expect(err).NotTo(HaveOccurred())

	// dockerClient.StopContainer returns error
	client.StopContainerReturns(errors.New("stop failed"))
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	client.StopContainerReturns(nil)

	// dockerClient.KillContainer returns error
	client.KillContainerReturns(errors.New("kill failed"))
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	client.KillContainerReturns(nil)

	// dockerClient.RemoveContainer returns error
	client.RemoveContainerReturns(errors.New("remove failed"))
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	client.RemoveContainerReturns(nil)

	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())
}

func Test_streamOutput(t *testing.T) {
	gt := NewGomegaWithT(t)

	logger, recorder := floggingtest.NewTestLogger(t)
	containerLogger, containerRecorder := floggingtest.NewTestLogger(t)

	client := &mock.DockerClient{}
	errCh := make(chan error, 1)
	optsCh := make(chan docker.AttachToContainerOptions, 1)
	client.AttachToContainerStub = func(opts docker.AttachToContainerOptions) error {
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
	client := &mock.DockerClient{}

	tests := []struct {
		desc           string
		buildErr       error
		expectedLabels []string
	}{
		{
			desc:           "success",
			buildErr:       nil,
			expectedLabels: []string{"chaincode", "simple:1.0", "success", "true"},
		},
		{
			desc:           "failure",
			buildErr:       errors.New("build failed"),
			expectedLabels: []string{"chaincode", "simple:1.0", "success", "false"},
		},
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

			client.BuildImageReturns(tt.buildErr)
			dvm.deployImage(client, ccid, &bytes.Buffer{})

			gt.Expect(fakeChaincodeImageBuildDuration.WithCallCount()).To(Equal(1))
			gt.Expect(fakeChaincodeImageBuildDuration.WithArgsForCall(0)).To(Equal(tt.expectedLabels))
			gt.Expect(fakeChaincodeImageBuildDuration.ObserveArgsForCall(0)).NotTo(BeZero())
			gt.Expect(fakeChaincodeImageBuildDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
		})
	}
}

func Test_Stop(t *testing.T) {
	dvm := DockerVM{}
	ccid := ccintf.CCID{Name: "simple"}

	// Failure case
	dvm.getClientFnc = func() (dockerClient, error) {
		return nil, errors.New("failed to get Docker client")
	}
	err := dvm.Stop(ccid, 10, true, true)
	assert.Error(t, err)

	// Success case
	client := &mock.DockerClient{}
	dvm.getClientFnc = func() (dockerClient, error) {
		return client, nil
	}
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
	client := &mock.DockerClient{}
	dvm.getClientFnc = func() (dockerClient, error) { return client, nil }

	client.WaitContainerReturns(99, nil)
	exitCode, err := dvm.Wait(ccintf.CCID{Name: "the-name", Version: "the-version"})
	assert.NoError(t, err)
	assert.Equal(t, 99, exitCode)

	// wait fails
	client.WaitContainerReturns(0, errors.New("no-wait-for-you"))
	_, err = dvm.Wait(ccintf.CCID{})
	assert.EqualError(t, err, "no-wait-for-you")
}

func Test_HealthCheck(t *testing.T) {
	dvm := DockerVM{}

	client := &mock.DockerClient{}
	dvm.getClientFnc = func() (dockerClient, error) {
		return client, nil
	}
	client.PingWithContextReturns(nil)
	err := dvm.HealthCheck(context.Background())
	assert.NoError(t, err)

	client.PingWithContextReturns(errors.New("error pinging daemon"))
	err = dvm.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error pinging daemon")
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

func Test_deployImage(t *testing.T) {
	gt := NewGomegaWithT(t)

	client := &mock.DockerClient{}

	dockerVM := &DockerVM{
		PeerID:       "peer",
		NetworkID:    "network",
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
	}

	ccid := ccintf.CCID{
		Name:    "mycc",
		Version: "1.0",
	}
	ccname, err := dockerVM.GetVMNameForDocker(ccid)
	gt.Expect(err).NotTo(HaveOccurred())

	expectedOpts := docker.BuildImageOptions{
		Name:         ccname,
		Pull:         viper.GetBool("chaincode.pull"),
		InputStream:  nil,
		OutputStream: bytes.NewBuffer(nil),
		NetworkMode:  "host",
	}

	err = dockerVM.deployImage(client, ccid, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(client.BuildImageCallCount()).To(Equal(1))
	gt.Expect(client.BuildImageArgsForCall(0)).To(Equal(expectedOpts))

	// set network mode
	hostConfig = getDockerHostConfig()
	hostConfig.NetworkMode = "bridge"
	expectedOpts.NetworkMode = "bridge"
	err = dockerVM.deployImage(client, ccid, nil)

	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(client.BuildImageCallCount()).To(Equal(2))
	gt.Expect(client.BuildImageArgsForCall(1)).To(Equal(expectedOpts))
	hostConfig = nil
}

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

type mockBuilder struct {
	buildFunc func() (io.Reader, error)
}

func (m *mockBuilder) Build() (io.Reader, error) {
	return m.buildFunc()
}
