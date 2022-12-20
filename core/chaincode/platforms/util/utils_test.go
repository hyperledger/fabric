/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"archive/tar"
	"bytes"
	"encoding/base32"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestDockerBuild(t *testing.T) {
	client, err := docker.NewClientFromEnv()
	require.NoError(t, err, "failed to get docker client")

	is := bytes.NewBuffer(nil)
	tw := tar.NewWriter(is)

	const dockerfile = "FROM busybox:1.33\nADD . /\n"
	err = tw.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Size: int64(len(dockerfile)),
		Mode: 0o644,
	})
	require.NoError(t, err)
	_, err = tw.Write([]byte(dockerfile))
	require.NoError(t, err)

	err = tw.WriteHeader(&tar.Header{
		Name:     "chaincode/input/",
		Typeflag: tar.TypeDir,
		Mode:     0o40755,
	})
	require.NoError(t, err)
	err = tw.WriteHeader(&tar.Header{
		Name:     "chaincode/output/",
		Typeflag: tar.TypeDir,
		Mode:     0o40755,
	})
	require.NoError(t, err)
	tw.Close()

	imageName := uniqueName()
	err = client.BuildImage(docker.BuildImageOptions{
		Name:         imageName,
		InputStream:  is,
		OutputStream: ioutil.Discard,
	})
	require.NoError(t, err, "failed to build base image")
	defer client.RemoveImageExtended(imageName, docker.RemoveImageOptions{Force: true})

	codepackage := bytes.NewBuffer(nil)
	tw = tar.NewWriter(codepackage)
	tw.Close()

	err = DockerBuild(
		DockerBuildOptions{
			Image:        imageName,
			Cmd:          "/bin/true",
			InputStream:  codepackage,
			OutputStream: ioutil.Discard,
		},
		client,
	)
	require.NoError(t, err, "build failed")
}

func uniqueName() string {
	name := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(util.GenerateBytesUUID())
	return strings.ToLower(name)
}

func TestUtil_GetDockerImageFromConfig(t *testing.T) {
	path := "dt"

	expected := "FROM " + metadata.DockerNamespace + ":" + runtime.GOARCH + "-" + metadata.Version
	viper.Set(path, "FROM $(DOCKER_NS):$(ARCH)-$(PROJECT_VERSION)")
	actual := GetDockerImageFromConfig(path)
	require.Equal(t, expected, actual, `Error parsing Dockerfile Template. Expected "%s", got "%s"`, expected, actual)

	expected = "FROM " + metadata.DockerNamespace + ":" + runtime.GOARCH + "-" + twoDigitVersion(metadata.Version)
	viper.Set(path, "FROM $(DOCKER_NS):$(ARCH)-$(TWO_DIGIT_VERSION)")
	actual = GetDockerImageFromConfig(path)
	require.Equal(t, expected, actual, `Error parsing Dockerfile Template. Expected "%s", got "%s"`, expected, actual)
}

func TestMain(m *testing.M) {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	configtest.AddDevConfigPath(nil)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("could not read config %s\n", err)
		os.Exit(-1)
	}
	os.Exit(m.Run())
}

func TestTwoDigitVersion(t *testing.T) {
	version := "2.0.0"
	expected := "2.0"
	actual := twoDigitVersion(version)
	require.Equal(t, expected, actual, `Error parsing two digit version. Expected "%s", got "%s"`, expected, actual)

	version = "2.0.0-beta123"
	expected = "2.0"
	actual = twoDigitVersion(version)
	require.Equal(t, expected, actual, `Error parsing two digit version. Expected "%s", got "%s"`, expected, actual)

	version = "latest"
	expected = "latest"
	actual = twoDigitVersion(version)
	require.Equal(t, expected, actual, `Error parsing two digit version. Expected "%s", got "%s"`, expected, actual)

	version = "v1.2.3"
	expected = "1.2"
	actual = twoDigitVersion(version)
	require.Equal(t, expected, actual, `Error parsing two digit version. Expected "%s", got "%s"`, expected, actual)

	version = "v1.2.3-beta1"
	expected = "1.2"
	actual = twoDigitVersion(version)
	require.Equal(t, expected, actual, `Error parsing two digit version. Expected "%s", got "%s"`, expected, actual)
}

func TestDockerBuildOptions(t *testing.T) {
	buildOptions := DockerBuildOptions{
		Image: "imageName",
		Cmd:   "theCommand",
		Env:   []string{"ENV_VARIABLE"},
	}

	actualBuildOptionsString := buildOptions.String()
	expectedBuildOptionsString := "Image=imageName Env=[ENV_VARIABLE] Cmd=theCommand)"
	require.Equal(t, expectedBuildOptionsString, actualBuildOptionsString, `Expected "%s", got "%s"`, expectedBuildOptionsString, actualBuildOptionsString)
}
