/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	"github.com/hyperledger/fabric/core/container/dockercontroller/mock"
	"github.com/moby/moby/api/types/registry"
	dcli "github.com/moby/moby/client"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestPlatformBuilderPullCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dockerConfigEnv, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"auths":{"private.example":{"username":"alice","password":"secret"}}}`), 0o600))
	defer viper.Reset()
	viper.Set("chaincode.builder", "private.example/builder:latest")
	for _, path := range []string{"ParamsImage", "DockerBuild"} {
		t.Run(path, func(t *testing.T) {
			client := &mock.DockerClient{}
			client.ImageInspectReturns(dcli.ImageInspectResult{}, errors.New("missing image"))
			pullErr := errors.New("stop after authenticated pull")
			client.ImagePullReturns(nil, pullErr)
			wrapper := &RegistryClient{APIClient: client}
			var err error
			if path == "ParamsImage" {
				_, _, _, err = util.ParamsImage(wrapper)
			} else {
				err = util.DockerBuild(util.DockerBuildOptions{Image: "private.example/runtime:latest"}, wrapper)
			}
			require.ErrorIs(t, err, pullErr)
			require.Equal(t, 1, client.ImagePullCallCount())
			_, _, options := client.ImagePullArgsForCall(0)
			decoded, err := base64.URLEncoding.DecodeString(options.RegistryAuth)
			require.NoError(t, err)
			var auth registry.AuthConfig
			require.NoError(t, json.Unmarshal(decoded, &auth))
			require.Equal(t, "alice", auth.Username)
			require.Equal(t, "secret", auth.Password)
		})
	}
}

func TestRegistryClientSelectsCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dockerConfigEnv, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"auths":{"https://index.docker.io/v1/":{"username":"hub","password":"secret"},"private.example:5000":{"username":"private","password":"secret"}}}`), 0o600))
	for _, tc := range []struct{ image, username string }{
		{"ubuntu:latest", "hub"},
		{"private.example:5000/builder:latest", "private"},
		{"other.example/builder:latest", ""},
	} {
		t.Run(tc.image, func(t *testing.T) {
			client := &mock.DockerClient{}
			wrapper := &RegistryClient{APIClient: client}
			_, err := wrapper.ImagePull(context.Background(), tc.image, dcli.ImagePullOptions{})
			require.NoError(t, err)
			_, _, options := client.ImagePullArgsForCall(0)
			if tc.username == "" {
				require.Empty(t, options.RegistryAuth)
				return
			}
			decoded, err := base64.URLEncoding.DecodeString(options.RegistryAuth)
			require.NoError(t, err)
			var auth registry.AuthConfig
			require.NoError(t, json.Unmarshal(decoded, &auth))
			require.Equal(t, tc.username, auth.Username)
		})
	}
}

func TestBuildImageResponseErrors(t *testing.T) {
	for _, tc := range []struct{ body, message string }{
		{"{\"error\":\"unauthorized\"}", "unauthorized"},
		{"{", "read Docker build response"},
		{"{\"stream\":\"Built successfully\"}", ""},
	} {
		t.Run(tc.body, func(t *testing.T) {
			client := &mock.DockerClient{}
			client.ImageBuildReturns(dcli.ImageBuildResult{Body: io.NopCloser(strings.NewReader(tc.body))}, nil)
			duration := &metricsfakes.Histogram{}
			duration.WithReturns(duration)
			vm := &DockerVM{Client: client, BuildMetrics: &BuildMetrics{ChaincodeImageBuildDuration: duration}}
			err := vm.buildImage("test", &bytes.Buffer{})
			if tc.message == "" {
				require.NoError(t, err)
				require.Equal(t, []string{"chaincode", "test", "success", "true"}, duration.WithArgsForCall(0))
			} else {
				require.ErrorContains(t, err, tc.message)
				require.Equal(t, []string{"chaincode", "test", "success", "false"}, duration.WithArgsForCall(0))
			}
			require.Equal(t, 1, duration.ObserveCallCount())
		})
	}
}
