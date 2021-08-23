/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestDockerPull(t *testing.T) {
	codepackage, output := io.Pipe()
	go func() {
		tw := tar.NewWriter(output)

		tw.Close()
		output.Close()
	}()

	binpackage := bytes.NewBuffer(nil)

	// Perform a nop operation within a fixed target.  We choose 1.1.0 because we know it's
	// published and available.  Ideally we could choose something that we know is both multi-arch
	// and ok to delete prior to executing DockerBuild.  This would ensure that we exercise the
	// image pull logic.  However, no suitable target exists that meets all the criteria.  Therefore
	// we settle on using a known released image.  We don't know if the image is already
	// downloaded per se, and we don't want to explicitly delete this particular image first since
	// it could be in use legitimately elsewhere.  Instead, we just know that this should always
	// work and call that "close enough".
	//
	// Future considerations: publish a known dummy image that is multi-arch and free to randomly
	// delete, and use that here instead.
	image := fmt.Sprintf("hyperledger/fabric-ccenv:%s-1.1.0", runtime.GOARCH)
	client, err := docker.NewClientFromEnv()
	if err != nil {
		t.Errorf("failed to get docker client: %s", err)
	}

	err = DockerBuild(
		DockerBuildOptions{
			Image:        image,
			Cmd:          "/bin/true",
			InputStream:  codepackage,
			OutputStream: binpackage,
		},
		client,
	)
	if err != nil {
		t.Errorf("Error during build: %s", err)
	}
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

	version = "latest"
	expected = "latest"
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
