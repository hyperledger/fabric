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
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test used to be part of an integration style test in core/container, moved to here
func TestIntegrationPath(t *testing.T) {
	client, err := docker.NewClientFromEnv()
	assert.NoError(t, err)
	provider := Provider{
		PeerID:       "",
		NetworkID:    util.GenerateUUID(),
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       client,
	}
	dc := provider.NewVM()
	ccid := ccintf.CCID("simple")

	err = dc.Start(ccid, nil, nil, nil, InMemBuilder{})
	require.NoError(t, err)

	// Stop, killing, and deleting
	err = dc.Stop(ccid, 0, true, true)
	require.NoError(t, err)

	err = dc.Start(ccid, nil, nil, nil, nil)
	require.NoError(t, err)

	// Stop, killing, but not deleting
	_ = dc.Stop(ccid, 0, false, true)
}

func Test_Start(t *testing.T) {
	gt := NewGomegaWithT(t)
	dockerClient := &mock.DockerClient{}
	dvm := DockerVM{
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       dockerClient,
	}
	ccid := ccintf.CCID("simple:1.0")
	args := make([]string, 1)
	env := make([]string, 1)
	files := map[string][]byte{
		"hello": []byte("world"),
	}

	// case 1: dockerClient.CreateContainer returns error
	testError1 := errors.New("junk1")
	dockerClient.CreateContainerReturns(nil, testError1)
	err := dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).To(MatchError(testError1))
	dockerClient.CreateContainerReturns(&docker.Container{}, nil)

	// case 2: dockerClient.UploadToContainer returns error
	testError2 := errors.New("junk2")
	dockerClient.UploadToContainerReturns(testError2)
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err.Error()).To(ContainSubstring("junk2"))
	dockerClient.UploadToContainerReturns(nil)

	// case 3: dockerClient.StartContainer returns docker.noSuchImgErr, BuildImage fails
	testError3 := errors.New("junk3")
	dockerClient.CreateContainerReturns(nil, docker.ErrNoSuchImage)
	dockerClient.BuildImageReturns(testError3)
	err = dvm.Start(ccid, args, env, files, &mockBuilder{buildFunc: func() (io.Reader, error) { return &bytes.Buffer{}, nil }})
	gt.Expect(err).To(MatchError(testError3))
	dockerClient.CreateContainerReturns(&docker.Container{}, nil)
	dockerClient.BuildImageReturns(nil)

	chaincodePath := "github.com/hyperledger/fabric/core/container/dockercontroller/testdata/src/chaincodes/noop"
	spec := &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_GOLANG,
		ChaincodeId: &pb.ChaincodeID{Name: "ex01", Path: chaincodePath},
		Input:       &pb.ChaincodeInput{Args: util.ToChaincodeArgs("f")},
	}
	codePackage, err := platforms.NewRegistry(&golang.Platform{}).GetDeploymentPayload(spec.Type.String(), spec.ChaincodeId.Path)
	if err != nil {
		t.Fatal()
	}
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackage}
	client, err := docker.NewClientFromEnv()
	assert.NoError(t, err)
	bldr := &mockBuilder{
		buildFunc: func() (io.Reader, error) {
			return platforms.NewRegistry(&golang.Platform{}).GenerateDockerBuild(
				cds.ChaincodeSpec.Type.String(),
				cds.ChaincodeSpec.ChaincodeId.Path,
				cds.ChaincodeSpec.ChaincodeId.Name,
				cds.ChaincodeSpec.ChaincodeId.Version,
				cds.CodePackage,
				client,
			)
		},
	}

	// case 4: start called and dockerClient.CreateContainer returns
	// docker.noSuchImgErr and dockerClient.Start returns error
	testError4 := errors.New("junk4")
	dvm.AttachStdOut = true
	dockerClient.CreateContainerReturns(nil, testError4)
	err = dvm.Start(ccid, args, env, files, bldr)
	gt.Expect(err).To(MatchError(testError4))
	dockerClient.CreateContainerReturns(&docker.Container{}, nil)

	// Success cases
	err = dvm.Start(ccid, args, env, files, bldr)
	gt.Expect(err).NotTo(HaveOccurred())

	// dockerClient.StopContainer returns error
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())

	// dockerClient.KillContainer returns error
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())

	// dockerClient.RemoveContainer returns error
	err = dvm.Start(ccid, args, env, files, nil)
	gt.Expect(err).NotTo(HaveOccurred())

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
	ccid := ccintf.CCID("simple:1.0")
	client := &mock.DockerClient{}

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
				Client: client,
			}

			if tt.buildErr {
				client.BuildImageReturns(errors.New("Error building image"))
			}
			dvm.deployImage(ccid, &bytes.Buffer{})

			gt.Expect(fakeChaincodeImageBuildDuration.WithCallCount()).To(Equal(1))
			gt.Expect(fakeChaincodeImageBuildDuration.WithArgsForCall(0)).To(Equal(tt.expectedLabels))
			gt.Expect(fakeChaincodeImageBuildDuration.ObserveArgsForCall(0)).NotTo(BeZero())
			gt.Expect(fakeChaincodeImageBuildDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
		})
	}
}

func Test_Stop(t *testing.T) {
	dvm := DockerVM{Client: &mock.DockerClient{}}
	ccid := ccintf.CCID("simple")

	// Success case
	err := dvm.Stop(ccid, 10, true, true)
	assert.NoError(t, err)
}

func Test_Wait(t *testing.T) {
	dvm := DockerVM{}

	// happy path
	client := &mock.DockerClient{}
	dvm.Client = client

	client.WaitContainerReturns(99, nil)
	exitCode, err := dvm.Wait(ccintf.CCID("the-name:the-version"))
	assert.NoError(t, err)
	assert.Equal(t, 99, exitCode)
	assert.Equal(t, "the-name-the-version", client.WaitContainerArgsForCall(0))

	// wait fails
	client.WaitContainerReturns(99, errors.New("no-wait-for-you"))
	_, err = dvm.Wait(ccintf.CCID(""))
	assert.EqualError(t, err, "no-wait-for-you")
}

func Test_HealthCheck(t *testing.T) {
	dvm := DockerVM{}
	client := mock.DockerClient{}
	dvm.Client = &client
	err := dvm.HealthCheck(context.Background())
	assert.NoError(t, err)

	client.PingWithContextReturns(errors.New("Error pinging daemon"))
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
			ccid:           ccintf.CCID("mycc:1.0"),
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-peer0-mycc:1.0")))),
		},
		{
			name:           "mycc-nonetworkid",
			vm:             &DockerVM{PeerID: "peer1"},
			ccid:           ccintf.CCID("mycc:1.0"),
			expectedOutput: fmt.Sprintf("%s-%s", "peer1-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("peer1-mycc:1.0")))),
		},
		{
			name:           "myCC-UCids",
			vm:             &DockerVM{NetworkID: "Dev", PeerID: "Peer0"},
			ccid:           ccintf.CCID("myCC:1.0"),
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("Dev-Peer0-myCC:1.0")))),
		},
		{
			name:           "myCC-idsWithSpecialChars",
			vm:             &DockerVM{NetworkID: "Dev$dev", PeerID: "Peer*0"},
			ccid:           ccintf.CCID("myCC:1.0"),
			expectedOutput: fmt.Sprintf("%s-%s", "dev-dev-peer-0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("Dev$dev-Peer*0-myCC:1.0")))),
		},
		{
			name:           "mycc-nopeerid",
			vm:             &DockerVM{NetworkID: "dev"},
			ccid:           ccintf.CCID("mycc:1.0"),
			expectedOutput: fmt.Sprintf("%s-%s", "dev-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-mycc:1.0")))),
		},
		{
			name:           "myCC-LCids",
			vm:             &DockerVM{NetworkID: "dev", PeerID: "peer0"},
			ccid:           ccintf.CCID("myCC:1.0"),
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-peer0-myCC:1.0")))),
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
			ccid:           ccintf.CCID("myCC:1.0"),
			expectedOutput: fmt.Sprintf("%s", "Dev-Peer0-myCC-1.0"),
		},
	}

	for _, test := range tc {
		name := test.vm.GetVMName(test.ccid)
		assert.Equal(t, test.expectedOutput, name, "Unexpected output for test case name: %s", test.name)
	}

}

func TestCreateNewVM(t *testing.T) {
	networkID := util.GenerateUUID()
	client := &mock.DockerClient{}

	provider := Provider{
		PeerID:       "peerID",
		NetworkID:    networkID,
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       client,
		NetworkMode:  "bridge",
	}
	dvm := provider.NewVM()

	expectedClient := &DockerVM{
		PeerID:       "peerID",
		NetworkID:    networkID,
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       client,
		NetworkMode:  "bridge",
	}
	assert.Equal(t, expectedClient, dvm)
}

func Test_deployImage(t *testing.T) {
	client := &mock.DockerClient{}
	dvm := DockerVM{
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       client,
		NetworkMode:  "network-mode",
	}

	err := dvm.deployImage(ccintf.CCID("simple"), &bytes.Buffer{})
	assert.NoError(t, err)
	assert.Equal(t, 1, client.BuildImageCallCount())

	opts := client.BuildImageArgsForCall(0)
	assert.Equal(t, "simple-a7a39b72f29718e653e73503210fbb597057b7a1c77d1fe321a1afcff041d4e1", opts.Name)
	assert.False(t, opts.Pull)
	assert.Equal(t, "network-mode", opts.NetworkMode)
	assert.Equal(t, &bytes.Buffer{}, opts.InputStream)
	assert.NotNil(t, opts.OutputStream)
}

func Test_deployImageFailure(t *testing.T) {
	client := &mock.DockerClient{}
	client.BuildImageReturns(errors.New("oh-bother-we-failed-badly"))
	dvm := DockerVM{
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       client,
		NetworkMode:  "network-mode",
	}

	err := dvm.deployImage(ccintf.CCID("simple"), &bytes.Buffer{})
	assert.EqualError(t, err, "oh-bother-we-failed-badly")
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
