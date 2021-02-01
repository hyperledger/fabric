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
	"io/ioutil"
	"testing"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/dockercontroller/mock"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/stretchr/testify/require"
)

// This test used to be part of an integration style test in core/container, moved to here
func TestIntegrationPath(t *testing.T) {
	client, err := docker.NewClientFromEnv()
	require.NoError(t, err)

	fakePlatformBuilder := &mock.PlatformBuilder{}
	fakePlatformBuilder.GenerateDockerBuildReturns(InMemBuilder{}.Build())

	dc := DockerVM{
		PeerID:          "",
		NetworkID:       util.GenerateUUID(),
		BuildMetrics:    NewBuildMetrics(&disabled.Provider{}),
		Client:          client,
		PlatformBuilder: fakePlatformBuilder,
	}
	ccid := "simple"

	instance, err := dc.Build("simple", &persistence.ChaincodePackageMetadata{
		Type: "type",
		Path: "path",
	}, bytes.NewBuffer([]byte("code-package")))
	require.NoError(t, err)

	require.Equal(t, &ContainerInstance{
		CCID:     "simple",
		Type:     "TYPE",
		DockerVM: &dc,
	}, instance)

	err = dc.Start(ccid, "GOLANG", &ccintf.PeerConnection{
		Address: "peer-address",
	})
	require.NoError(t, err)

	err = dc.Stop(ccid)
	require.NoError(t, err)
}

var expectedNodeStartScript = `
set -e
if [ -x /chaincode/start.sh ]; then
	/chaincode/start.sh --peer.address peer-address
else
	cd /usr/local/src
	npm start -- --peer.address peer-address
fi
`

func TestGetArgs(t *testing.T) {
	tests := []struct {
		name         string
		ccType       pb.ChaincodeSpec_Type
		expectedArgs []string
		expectedErr  string
	}{
		{"golang-chaincode", pb.ChaincodeSpec_GOLANG, []string{"chaincode", "-peer.address=peer-address"}, ""},
		{"java-chaincode", pb.ChaincodeSpec_JAVA, []string{"/root/chaincode-java/start", "--peerAddress", "peer-address"}, ""},
		{"node-chaincode", pb.ChaincodeSpec_NODE, []string{"/bin/sh", "-c", expectedNodeStartScript}, ""},
		{"unknown-chaincode", pb.ChaincodeSpec_Type(999), []string{}, "unknown chaincodeType: 999"},
	}
	for _, tc := range tests {
		vm := &DockerVM{}

		args, err := vm.GetArgs(tc.ccType.String(), "peer-address")
		if tc.expectedErr != "" {
			require.EqualError(t, err, tc.expectedErr)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, tc.expectedArgs, args)
	}
}

func TestGetEnv(t *testing.T) {
	vm := &DockerVM{
		LoggingEnv: []string{"LOG_ENV=foo"},
		MSPID:      "mspid",
	}

	t.Run("nil TLS config", func(t *testing.T) {
		env := vm.GetEnv("test", nil)
		require.Equal(t, []string{"CORE_CHAINCODE_ID_NAME=test", "LOG_ENV=foo", "CORE_PEER_TLS_ENABLED=false", "CORE_PEER_LOCALMSPID=mspid"}, env)
	})

	t.Run("real TLS config", func(t *testing.T) {
		env := vm.GetEnv("test", &ccintf.TLSConfig{
			ClientKey:  []byte("key"),
			ClientCert: []byte("cert"),
			RootCert:   []byte("root"),
		})
		require.Equal(t, []string{
			"CORE_CHAINCODE_ID_NAME=test",
			"LOG_ENV=foo",
			"CORE_PEER_TLS_ENABLED=true",
			"CORE_TLS_CLIENT_KEY_PATH=/etc/hyperledger/fabric/client.key",
			"CORE_TLS_CLIENT_CERT_PATH=/etc/hyperledger/fabric/client.crt",
			"CORE_TLS_CLIENT_KEY_FILE=/etc/hyperledger/fabric/client_pem.key",
			"CORE_TLS_CLIENT_CERT_FILE=/etc/hyperledger/fabric/client_pem.crt",
			"CORE_PEER_TLS_ROOTCERT_FILE=/etc/hyperledger/fabric/peer.crt",
			"CORE_PEER_LOCALMSPID=mspid",
		}, env)
	})
}

func Test_Start(t *testing.T) {
	gt := NewGomegaWithT(t)
	dockerClient := &mock.DockerClient{}
	dvm := DockerVM{
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       dockerClient,
	}

	ccid := "simple:1.0"
	peerConnection := &ccintf.PeerConnection{
		Address: "peer-address",
		TLSConfig: &ccintf.TLSConfig{
			ClientKey:  []byte("key"),
			ClientCert: []byte("cert"),
			RootCert:   []byte("root"),
		},
	}

	// case 1: dockerClient.CreateContainer returns error
	testError1 := errors.New("junk1")
	dockerClient.CreateContainerReturns(nil, testError1)
	err := dvm.Start(ccid, "GOLANG", peerConnection)
	gt.Expect(err).To(MatchError(testError1))
	dockerClient.CreateContainerReturns(&docker.Container{}, nil)

	// case 2: dockerClient.UploadToContainer returns error
	testError2 := errors.New("junk2")
	dockerClient.UploadToContainerReturns(testError2)
	err = dvm.Start(ccid, "GOLANG", peerConnection)
	gt.Expect(err.Error()).To(ContainSubstring("junk2"))
	dockerClient.UploadToContainerReturns(nil)

	// case 3: start called and dockerClient.CreateContainer returns
	// docker.noSuchImgErr and dockerClient.Start returns error
	testError3 := errors.New("junk3")
	dvm.AttachStdOut = true
	dockerClient.CreateContainerReturns(nil, testError3)
	err = dvm.Start(ccid, "GOLANG", peerConnection)
	gt.Expect(err).To(MatchError(testError3))
	dockerClient.CreateContainerReturns(&docker.Container{}, nil)

	// case 4: GetArgs returns error
	err = dvm.Start(ccid, "FAKE_TYPE", peerConnection)
	gt.Expect(err).To(MatchError("could not get args: unknown chaincodeType: FAKE_TYPE"))

	// Success cases
	err = dvm.Start(ccid, "GOLANG", peerConnection)
	gt.Expect(err).NotTo(HaveOccurred())

	// dockerClient.StopContainer returns error
	err = dvm.Start(ccid, "GOLANG", peerConnection)
	gt.Expect(err).NotTo(HaveOccurred())

	// dockerClient.KillContainer returns error
	err = dvm.Start(ccid, "GOLANG", peerConnection)
	gt.Expect(err).NotTo(HaveOccurred())

	// dockerClient.RemoveContainer returns error
	err = dvm.Start(ccid, "GOLANG", peerConnection)
	gt.Expect(err).NotTo(HaveOccurred())

	err = dvm.Start(ccid, "GOLANG", peerConnection)
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
	fmt.Fprintf(opts.OutputStream, "message-two") // does not get written until after stream closed
	gt.Eventually(containerRecorder).Should(gbytes.Say("message-one"))
	gt.Consistently(containerRecorder.Entries).Should(HaveLen(1))

	close(errCh)

	gt.Eventually(recorder).Should(gbytes.Say("Container container-name has closed its IO channel"))
	gt.Consistently(recorder.Entries).Should(HaveLen(1))
	gt.Eventually(containerRecorder).Should(gbytes.Say("message-two"))
	gt.Consistently(containerRecorder.Entries).Should(HaveLen(2))
}

func Test_BuildMetric(t *testing.T) {
	ccid := "simple:1.0"
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
			dvm.buildImage(ccid, &bytes.Buffer{})

			gt.Expect(fakeChaincodeImageBuildDuration.WithCallCount()).To(Equal(1))
			gt.Expect(fakeChaincodeImageBuildDuration.WithArgsForCall(0)).To(Equal(tt.expectedLabels))
			gt.Expect(fakeChaincodeImageBuildDuration.ObserveArgsForCall(0)).NotTo(BeZero())
			gt.Expect(fakeChaincodeImageBuildDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
		})
	}
}

func Test_Stop(t *testing.T) {
	dvm := DockerVM{Client: &mock.DockerClient{}}
	ccid := "simple"

	// Success case
	err := dvm.Stop(ccid)
	require.NoError(t, err)
}

func Test_Wait(t *testing.T) {
	dvm := DockerVM{}

	// happy path
	client := &mock.DockerClient{}
	dvm.Client = client

	client.WaitContainerReturns(99, nil)
	exitCode, err := dvm.Wait("the-name:the-version")
	require.NoError(t, err)
	require.Equal(t, 99, exitCode)
	require.Equal(t, "the-name-the-version", client.WaitContainerArgsForCall(0))

	// wait fails
	client.WaitContainerReturns(99, errors.New("no-wait-for-you"))
	_, err = dvm.Wait("")
	require.EqualError(t, err, "no-wait-for-you")
}

func TestHealthCheck(t *testing.T) {
	client := &mock.DockerClient{}
	vm := &DockerVM{Client: client}

	err := vm.HealthCheck(context.Background())
	require.NoError(t, err)

	client.PingWithContextReturns(errors.New("Error pinging daemon"))
	err = vm.HealthCheck(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "Error pinging daemon")
}

type testCase struct {
	name           string
	vm             *DockerVM
	ccid           string
	expectedOutput string
}

func TestGetVMNameForDocker(t *testing.T) {
	tc := []testCase{
		{
			name:           "mycc",
			vm:             &DockerVM{NetworkID: "dev", PeerID: "peer0"},
			ccid:           "mycc:1.0",
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-peer0-mycc-1.0")))),
		},
		{
			name:           "mycc-nonetworkid",
			vm:             &DockerVM{PeerID: "peer1"},
			ccid:           "mycc:1.0",
			expectedOutput: fmt.Sprintf("%s-%s", "peer1-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("peer1-mycc-1.0")))),
		},
		{
			name:           "myCC-UCids",
			vm:             &DockerVM{NetworkID: "Dev", PeerID: "Peer0"},
			ccid:           "myCC:1.0",
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("Dev-Peer0-myCC-1.0")))),
		},
		{
			name:           "myCC-idsWithSpecialChars",
			vm:             &DockerVM{NetworkID: "Dev$dev", PeerID: "Peer*0"},
			ccid:           "myCC:1.0",
			expectedOutput: fmt.Sprintf("%s-%s", "dev-dev-peer-0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("Dev$dev-Peer*0-myCC-1.0")))),
		},
		{
			name:           "mycc-nopeerid",
			vm:             &DockerVM{NetworkID: "dev"},
			ccid:           "mycc:1.0",
			expectedOutput: fmt.Sprintf("%s-%s", "dev-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-mycc-1.0")))),
		},
		{
			name:           "myCC-LCids",
			vm:             &DockerVM{NetworkID: "dev", PeerID: "peer0"},
			ccid:           "myCC:1.0",
			expectedOutput: fmt.Sprintf("%s-%s", "dev-peer0-mycc-1.0", hex.EncodeToString(util.ComputeSHA256([]byte("dev-peer0-myCC-1.0")))),
		},
	}

	for _, test := range tc {
		name, err := test.vm.GetVMNameForDocker(test.ccid)
		require.Nil(t, err, "Expected nil error")
		require.Equal(t, test.expectedOutput, name, "Unexpected output for test case name: %s", test.name)
	}
}

func TestGetVMName(t *testing.T) {
	tc := []testCase{
		{
			name:           "myCC-preserveCase",
			vm:             &DockerVM{NetworkID: "Dev", PeerID: "Peer0"},
			ccid:           "myCC:1.0",
			expectedOutput: "Dev-Peer0-myCC-1.0",
		},
	}

	for _, test := range tc {
		name := test.vm.GetVMName(test.ccid)
		require.Equal(t, test.expectedOutput, name, "Unexpected output for test case name: %s", test.name)
	}
}

func Test_buildImage(t *testing.T) {
	client := &mock.DockerClient{}
	dvm := DockerVM{
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       client,
		NetworkMode:  "network-mode",
	}

	err := dvm.buildImage("simple", &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, 1, client.BuildImageCallCount())

	opts := client.BuildImageArgsForCall(0)
	require.Equal(t, "simple-a7a39b72f29718e653e73503210fbb597057b7a1c77d1fe321a1afcff041d4e1", opts.Name)
	require.False(t, opts.Pull)
	require.Equal(t, "network-mode", opts.NetworkMode)
	require.Equal(t, &bytes.Buffer{}, opts.InputStream)
	require.NotNil(t, opts.OutputStream)
}

func Test_buildImageFailure(t *testing.T) {
	client := &mock.DockerClient{}
	client.BuildImageReturns(errors.New("oh-bother-we-failed-badly"))
	dvm := DockerVM{
		BuildMetrics: NewBuildMetrics(&disabled.Provider{}),
		Client:       client,
		NetworkMode:  "network-mode",
	}

	err := dvm.buildImage("simple", &bytes.Buffer{})
	require.EqualError(t, err, "oh-bother-we-failed-badly")
}

func TestBuild(t *testing.T) {
	buildMetrics := NewBuildMetrics(&disabled.Provider{})
	md := &persistence.ChaincodePackageMetadata{
		Type: "type",
		Path: "path",
	}

	t.Run("when the image does not exist", func(t *testing.T) {
		client := &mock.DockerClient{}
		client.InspectImageReturns(nil, docker.ErrNoSuchImage)

		fakePlatformBuilder := &mock.PlatformBuilder{}
		fakePlatformBuilder.GenerateDockerBuildReturns(&bytes.Buffer{}, nil)

		dvm := &DockerVM{Client: client, BuildMetrics: buildMetrics, PlatformBuilder: fakePlatformBuilder}
		_, err := dvm.Build("chaincode-name:chaincode-version", md, bytes.NewBuffer([]byte("code-package")))
		require.NoError(t, err, "should have built successfully")

		require.Equal(t, 1, client.BuildImageCallCount())

		require.Equal(t, 1, fakePlatformBuilder.GenerateDockerBuildCallCount())
		ccType, path, codePackageStream := fakePlatformBuilder.GenerateDockerBuildArgsForCall(0)
		require.Equal(t, "TYPE", ccType)
		require.Equal(t, "path", path)
		codePackage, err := ioutil.ReadAll(codePackageStream)
		require.NoError(t, err)
		require.Equal(t, []byte("code-package"), codePackage)
	})

	t.Run("when inspecting the image fails", func(t *testing.T) {
		client := &mock.DockerClient{}
		client.InspectImageReturns(nil, errors.New("inspecting-image-fails"))

		dvm := &DockerVM{Client: client, BuildMetrics: buildMetrics}
		_, err := dvm.Build("chaincode-name:chaincode-version", md, bytes.NewBuffer([]byte("code-package")))
		require.EqualError(t, err, "docker image inspection failed: inspecting-image-fails")

		require.Equal(t, 0, client.BuildImageCallCount())
	})

	t.Run("when the image exists", func(t *testing.T) {
		client := &mock.DockerClient{}

		dvm := &DockerVM{Client: client, BuildMetrics: buildMetrics}
		_, err := dvm.Build("chaincode-name:chaincode-version", md, bytes.NewBuffer([]byte("code-package")))
		require.NoError(t, err)

		require.Equal(t, 0, client.BuildImageCallCount())
	})

	t.Run("when the platform builder fails", func(t *testing.T) {
		client := &mock.DockerClient{}
		client.InspectImageReturns(nil, docker.ErrNoSuchImage)
		client.BuildImageReturns(errors.New("no-build-for-you"))

		fakePlatformBuilder := &mock.PlatformBuilder{}
		fakePlatformBuilder.GenerateDockerBuildReturns(nil, errors.New("fake-builder-error"))

		dvm := &DockerVM{Client: client, BuildMetrics: buildMetrics, PlatformBuilder: fakePlatformBuilder}
		_, err := dvm.Build("chaincode-name:chaincode-version", md, bytes.NewBuffer([]byte("code-package")))
		require.Equal(t, 1, client.InspectImageCallCount())
		require.Equal(t, 1, fakePlatformBuilder.GenerateDockerBuildCallCount())
		require.Equal(t, 0, client.BuildImageCallCount())
		require.EqualError(t, err, "platform builder failed: fake-builder-error")
	})

	t.Run("when building the image fails", func(t *testing.T) {
		client := &mock.DockerClient{}
		client.InspectImageReturns(nil, docker.ErrNoSuchImage)
		client.BuildImageReturns(errors.New("no-build-for-you"))

		fakePlatformBuilder := &mock.PlatformBuilder{}

		dvm := &DockerVM{Client: client, BuildMetrics: buildMetrics, PlatformBuilder: fakePlatformBuilder}
		_, err := dvm.Build("chaincode-name:chaincode-version", md, bytes.NewBuffer([]byte("code-package")))
		require.Equal(t, 1, client.InspectImageCallCount())
		require.Equal(t, 1, client.BuildImageCallCount())
		require.EqualError(t, err, "docker image build failed: no-build-for-you")
	})
}

type InMemBuilder struct{}

func (imb InMemBuilder) Build() (io.Reader, error) {
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "FROM busybox:latest")
	fmt.Fprintln(buf, `RUN ln -s /bin/true /bin/chaincode`)
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
