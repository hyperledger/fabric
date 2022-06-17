/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func init() {
	// Significantly reduce execution time of deployment payload tests.
	gzipCompressionLevel = gzip.NoCompression
}

func generateFakeCDS(ccname, path, file string, mode int64) (*pb.ChaincodeDeploymentSpec, error) {
	codePackage := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(codePackage)
	tw := tar.NewWriter(gw)

	payload := make([]byte, 25)
	err := tw.WriteHeader(&tar.Header{Name: file, Mode: mode, Size: int64(len(payload))})
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(file, "/") {
		_, err = tw.Write(payload)
		if err != nil {
			return nil, err
		}
	}

	tw.Close()
	gw.Close()

	cds := &pb.ChaincodeDeploymentSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Name: ccname,
				Path: path,
			},
		},
		CodePackage: codePackage.Bytes(),
	}

	return cds, nil
}

func TestName(t *testing.T) {
	platform := &Platform{}
	require.Equal(t, "GOLANG", platform.Name())
}

func TestValidatePath(t *testing.T) {
	reset := setupGopath(t, "testdata")
	defer reset()

	tests := []struct {
		path string
		succ bool
	}{
		{path: "http://example.com/", succ: false},
		{path: "./testdata/missing", succ: false},
		{path: "chaincodes/noop", succ: true},
		{path: "testdata/ccmodule", succ: true},
	}

	for _, tt := range tests {
		platform := &Platform{}
		err := platform.ValidatePath(tt.path)
		if tt.succ {
			require.NoError(t, err, "expected %s to be a valid path", tt.path)
		} else {
			require.Errorf(t, err, "expected %s to be an invalid path", tt.path)
		}
	}
}

func TestNormalizePath(t *testing.T) {
	tempdir := t.TempDir()

	tests := []struct {
		path   string
		result string
	}{
		{path: "github.com/hyperledger/fabric/cmd/peer", result: "github.com/hyperledger/fabric/cmd/peer"},
		{path: "testdata/ccmodule", result: "ccmodule"},
		{path: "missing", result: "missing"},
		{path: tempdir, result: tempdir}, // /dev/null is returned from `go env GOMOD`
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			platform := &Platform{}
			result, err := platform.NormalizePath(tt.path)
			require.NoError(t, err, "normalize path failed")
			require.Equalf(t, tt.result, result, "want result %s got %s", tt.result, result)
		})
	}
}

// copied from the tar package
const (
	c_ISUID = 0o4000   // Set uid
	c_ISGID = 0o2000   // Set gid
	c_ISREG = 0o100000 // Regular file
)

func TestValidateCodePackage(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		file            string
		mode            int64
		successExpected bool
	}{
		{name: "NoCode", path: "path/to/somewhere", file: "/src/path/to/somewhere/main.go", mode: c_ISREG | 0o400, successExpected: false},
		{name: "NoCode", path: "path/to/somewhere", file: "src/path/to/somewhere/main.go", mode: c_ISREG | 0o400, successExpected: true},
		{name: "NoCode", path: "path/to/somewhere", file: "src/path/to/somewhere/main.go", mode: c_ISREG | 0o644, successExpected: true},
		{name: "NoCode", path: "path/to/somewhere", file: "src/path/to/somewhere/main.go", mode: c_ISREG | 0o755, successExpected: true},
		{name: "NoCode", path: "path/to/directory/", file: "src/path/to/directory/", mode: c_ISDIR | 0o755, successExpected: true},
		{name: "NoCode", path: "path/to/directory", file: "src/path/to/directory", mode: c_ISDIR | 0o755, successExpected: true},
		{name: "NoCode", path: "path/to/setuid", file: "src/path/to/setuid", mode: c_ISUID | 0o755, successExpected: false},
		{name: "NoCode", path: "path/to/setgid", file: "src/path/to/setgid", mode: c_ISGID | 0o755, successExpected: false},
		{name: "NoCode", path: "path/to/sticky/", file: "src/path/to/sticky/", mode: c_ISDIR | c_ISGID | 0o755, successExpected: false},
		{name: "NoCode", path: "path/to/somewhere", file: "META-INF/path/to/a/meta3", mode: 0o100400, successExpected: true},
	}

	for _, tt := range tests {
		cds, err := generateFakeCDS(tt.name, tt.path, tt.file, tt.mode)
		require.NoError(t, err, "failed to generate fake cds")

		platform := &Platform{}
		err = platform.ValidateCodePackage(cds.CodePackage)
		if tt.successExpected {
			require.NoError(t, err, "expected success for path: %s, file: %s", tt.path, tt.file)
		} else {
			require.Errorf(t, err, "expected error for path: %s, file: %s", tt.path, tt.file)
		}
	}
}

func Test_findSource(t *testing.T) {
	t.Run("Gopath", func(t *testing.T) {
		source, err := findSource(&CodeDescriptor{
			Module:       false,
			Source:       filepath.FromSlash("testdata/src/chaincodes/noop"),
			MetadataRoot: filepath.FromSlash("testdata/src/chaincodes/noop/META-INF"),
			Path:         "chaincodes/noop",
		})
		require.NoError(t, err, "failed to find source")
		require.Contains(t, source, "src/chaincodes/noop/chaincode.go")
		require.Contains(t, source, "META-INF/statedb/couchdb/indexes/indexOwner.json")
		require.NotContains(t, source, "src/chaincodes/noop/go.mod")
		require.NotContains(t, source, "src/chaincodes/noop/go.sum")
		require.Len(t, source, 2)
	})

	t.Run("Module", func(t *testing.T) {
		source, err := findSource(&CodeDescriptor{
			Module:       true,
			Source:       filepath.FromSlash("testdata/ccmodule"),
			MetadataRoot: filepath.FromSlash("testdata/ccmodule/META-INF"),
			Path:         "ccmodule",
		})
		require.NoError(t, err, "failed to find source")
		require.Len(t, source, 7)
		require.Contains(t, source, "META-INF/statedb/couchdb/indexes/indexOwner.json")
		require.Contains(t, source, "src/go.mod")
		require.Contains(t, source, "src/go.sum")
		require.Contains(t, source, "src/chaincode.go")
		require.Contains(t, source, "src/customlogger/customlogger.go")
		require.Contains(t, source, "src/nested/chaincode.go")
		require.Contains(t, source, "src/nested/META-INF/statedb/couchdb/indexes/nestedIndexOwner.json")
	})

	t.Run("NonExistent", func(t *testing.T) {
		_, err := findSource(&CodeDescriptor{Path: "acme.com/this/should/not/exist"})
		require.Error(t, err)
		require.True(t, os.IsNotExist(errors.Cause(err)))
	})
}

func tarContents(t *testing.T, payload []byte) []string {
	var files []string

	is := bytes.NewReader(payload)
	gr, err := gzip.NewReader(is)
	require.NoError(t, err, "failed to create new gzip reader")

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if header.Typeflag == tar.TypeReg {
			files = append(files, header.Name)
		}
	}

	return files
}

func TestGopathDeploymentPayload(t *testing.T) {
	reset := setupGopath(t, "testdata")
	defer reset()

	platform := &Platform{}

	t.Run("IncludesMetadata", func(t *testing.T) {
		payload, err := platform.GetDeploymentPayload("chaincodes/noop")
		require.NoError(t, err)

		contents := tarContents(t, payload)
		require.Contains(t, contents, "META-INF/statedb/couchdb/indexes/indexOwner.json")
	})

	tests := []struct {
		path string
		succ bool
	}{
		{path: "testdata/src/chaincodes/noop", succ: true},
		{path: "chaincodes/noop", succ: true},
		{path: "bad/chaincodes/noop", succ: false},
		{path: "chaincodes/BadImport", succ: false},
		{path: "chaincodes/BadMetadataInvalidIndex", succ: false},
		{path: "chaincodes/BadMetadataUnexpectedFolderContent", succ: false},
		{path: "chaincodes/BadMetadataIgnoreHiddenFile", succ: true},
		{path: "chaincodes/empty", succ: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			_, err := platform.GetDeploymentPayload(tt.path)
			if tt.succ {
				require.NoError(t, err, "expected success for path: %s", tt.path)
			} else {
				require.Errorf(t, err, "expected error for path: %s", tt.path)
			}
		})
	}
}

func TestModuleDeploymentPayload(t *testing.T) {
	platform := &Platform{}

	t.Run("TopLevel", func(t *testing.T) {
		dp, err := platform.GetDeploymentPayload("testdata/ccmodule")
		require.NoError(t, err)
		contents := tarContents(t, dp)
		require.ElementsMatch(t, contents, []string{
			"META-INF/statedb/couchdb/indexes/indexOwner.json", // top level metadata
			"src/chaincode.go",
			"src/customlogger/customlogger.go",
			"src/go.mod",
			"src/go.sum",
			"src/nested/META-INF/statedb/couchdb/indexes/nestedIndexOwner.json",
			"src/nested/chaincode.go",
		})
	})

	t.Run("NestedPackage", func(t *testing.T) {
		dp, err := platform.GetDeploymentPayload("testdata/ccmodule/nested")
		require.NoError(t, err)
		contents := tarContents(t, dp)
		require.ElementsMatch(t, contents, []string{
			"META-INF/statedb/couchdb/indexes/nestedIndexOwner.json", // nested metadata
			"src/META-INF/statedb/couchdb/indexes/indexOwner.json",
			"src/chaincode.go",
			"src/customlogger/customlogger.go",
			"src/go.mod",
			"src/go.sum",
			"src/nested/chaincode.go",
		})
	})
}

func setupGopath(t *testing.T, path string) func() {
	initialGopath, gopathSet := os.LookupEnv("GOPATH")
	initialGo111Module, go111ModuleSet := os.LookupEnv("GO111MODULE")

	if path == "" {
		err := os.Unsetenv("GOPATH")
		require.NoError(t, err)
	} else {
		absPath, err := filepath.Abs(path)
		require.NoErrorf(t, err, "expected to calculate absolute path from %s", path)
		err = os.Setenv("GOPATH", absPath)
		require.NoError(t, err, "failed to set GOPATH")
		err = os.Setenv("GO111MODULE", "off")
		require.NoError(t, err, "failed set GO111MODULE")
	}

	return func() {
		if !gopathSet {
			os.Unsetenv("GOPATH")
		} else {
			os.Setenv("GOPATH", initialGopath)
		}
		if !go111ModuleSet {
			os.Unsetenv("GO111MODULE")
		} else {
			os.Setenv("GO111MODULE", initialGo111Module)
		}
	}
}

func TestGenerateDockerFile(t *testing.T) {
	platform := &Platform{}

	viper.Set("chaincode.golang.runtime", "buildimage")
	expected := "FROM buildimage\nADD binpackage.tar /usr/local/bin"
	dockerfile, err := platform.GenerateDockerfile()
	require.NoError(t, err)
	require.Equal(t, expected, dockerfile)

	viper.Set("chaincode.golang.runtime", "another-buildimage")
	expected = "FROM another-buildimage\nADD binpackage.tar /usr/local/bin"
	dockerfile, err = platform.GenerateDockerfile()
	require.NoError(t, err)
	require.Equal(t, expected, dockerfile)
}

func TestGetLDFlagsOpts(t *testing.T) {
	viper.Set("chaincode.golang.dynamicLink", true)
	if getLDFlagsOpts() != dynamicLDFlagsOpts {
		t.Error("Error handling chaincode.golang.dynamicLink configuration. ldflags should be for dynamic linkink")
	}
	viper.Set("chaincode.golang.dynamicLink", false)
	if getLDFlagsOpts() != staticLDFlagsOpts {
		t.Error("Error handling chaincode.golang.dynamicLink configuration. ldflags should be for static linkink")
	}
}

func TestDockerBuildOptions(t *testing.T) {
	platform := &Platform{}

	t.Run("GOPROXY and GOSUMDB not set", func(t *testing.T) {
		opts, err := platform.DockerBuildOptions("the-path")
		require.NoError(t, err, "unexpected error from DockerBuildOptions")

		expectedOpts := util.DockerBuildOptions{
			Cmd: `
set -e
if [ -f "/chaincode/input/src/go.mod" ] && [ -d "/chaincode/input/src/vendor" ]; then
    cd /chaincode/input/src
    GO111MODULE=on go build -v -mod=vendor -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode the-path
elif [ -f "/chaincode/input/src/go.mod" ]; then
    cd /chaincode/input/src
    GO111MODULE=on go build -v -mod=readonly -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode the-path
elif [ -f "/chaincode/input/src/the-path/go.mod" ] && [ -d "/chaincode/input/src/the-path/vendor" ]; then
    cd /chaincode/input/src/the-path
    GO111MODULE=on go build -v -mod=vendor -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode .
elif [ -f "/chaincode/input/src/the-path/go.mod" ]; then
    cd /chaincode/input/src/the-path
    GO111MODULE=on go build -v -mod=readonly -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode .
else
    GO111MODULE=off GOPATH=/chaincode/input:$GOPATH go build -v -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode the-path
fi
echo Done!
`,
			Env: []string{"GOPROXY=https://proxy.golang.org"},
		}
		require.Equal(t, expectedOpts, opts)
	})

	t.Run("GOPROXY and GOSUMDB set", func(t *testing.T) {
		oldGoproxy, set := os.LookupEnv("GOPROXY")
		if set {
			defer os.Setenv("GOPROXY", oldGoproxy)
		}
		os.Setenv("GOPROXY", "the-goproxy")

		oldGosumdb, set := os.LookupEnv("GOSUMDB")
		if set {
			defer os.Setenv("GOSUMDB", oldGosumdb)
		}
		os.Setenv("GOSUMDB", "the-gosumdb")

		opts, err := platform.DockerBuildOptions("the-path")
		require.NoError(t, err, "unexpected error from DockerBuildOptions")

		expectedOpts := util.DockerBuildOptions{
			Cmd: `
set -e
if [ -f "/chaincode/input/src/go.mod" ] && [ -d "/chaincode/input/src/vendor" ]; then
    cd /chaincode/input/src
    GO111MODULE=on go build -v -mod=vendor -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode the-path
elif [ -f "/chaincode/input/src/go.mod" ]; then
    cd /chaincode/input/src
    GO111MODULE=on go build -v -mod=readonly -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode the-path
elif [ -f "/chaincode/input/src/the-path/go.mod" ] && [ -d "/chaincode/input/src/the-path/vendor" ]; then
    cd /chaincode/input/src/the-path
    GO111MODULE=on go build -v -mod=vendor -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode .
elif [ -f "/chaincode/input/src/the-path/go.mod" ]; then
    cd /chaincode/input/src/the-path
    GO111MODULE=on go build -v -mod=readonly -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode .
else
    GO111MODULE=off GOPATH=/chaincode/input:$GOPATH go build -v -ldflags "-linkmode external -extldflags '-static'" -o /chaincode/output/chaincode the-path
fi
echo Done!
`,
			Env: []string{"GOPROXY=the-goproxy", "GOSUMDB=the-gosumdb"},
		}
		require.Equal(t, expectedOpts, opts)
	})
}

func TestDescribeCode(t *testing.T) {
	abs, err := filepath.Abs(filepath.FromSlash("testdata/ccmodule"))
	require.NoError(t, err)

	t.Run("TopLevelModulePackage", func(t *testing.T) {
		cd, err := DescribeCode("testdata/ccmodule")
		require.NoError(t, err)
		expected := &CodeDescriptor{
			Source:       abs,
			MetadataRoot: filepath.Join(abs, "META-INF"),
			Path:         "ccmodule",
			Module:       true,
		}
		require.Equal(t, expected, cd)
	})

	t.Run("NestedModulePackage", func(t *testing.T) {
		cd, err := DescribeCode("testdata/ccmodule/nested")
		require.NoError(t, err)
		expected := &CodeDescriptor{
			Source:       abs,
			MetadataRoot: filepath.Join(abs, "nested", "META-INF"),
			Path:         "ccmodule/nested",
			Module:       true,
		}
		require.Equal(t, expected, cd)
	})
}

func TestRegularFileExists(t *testing.T) {
	t.Run("RegularFile", func(t *testing.T) {
		ok, err := regularFileExists("testdata/ccmodule/go.mod")
		require.NoError(t, err)
		require.True(t, ok)
	})
	t.Run("MissingFile", func(t *testing.T) {
		ok, err := regularFileExists("testdata/missing.file")
		require.NoError(t, err)
		require.False(t, ok)
	})
	t.Run("Directory", func(t *testing.T) {
		ok, err := regularFileExists("testdata")
		require.NoError(t, err)
		require.False(t, ok)
	})
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
