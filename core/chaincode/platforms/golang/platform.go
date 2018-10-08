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
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/ccmetadata"
	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// Platform for chaincodes written in Go
type Platform struct {
}

// Returns whether the given file or directory exists or not
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func decodeUrl(path string) (string, error) {
	var urlLocation string
	if strings.HasPrefix(path, "http://") {
		urlLocation = path[7:]
	} else if strings.HasPrefix(path, "https://") {
		urlLocation = path[8:]
	} else {
		urlLocation = path
	}

	if len(urlLocation) < 2 {
		return "", errors.New("ChaincodeSpec's path/URL invalid")
	}

	if strings.LastIndex(urlLocation, "/") == len(urlLocation)-1 {
		urlLocation = urlLocation[:len(urlLocation)-1]
	}

	return urlLocation, nil
}

func getGopath() (string, error) {
	env, err := getGoEnv()
	if err != nil {
		return "", err
	}
	// Only take the first element of GOPATH
	splitGoPath := filepath.SplitList(env["GOPATH"])
	if len(splitGoPath) == 0 {
		return "", fmt.Errorf("invalid GOPATH environment variable value: %s", env["GOPATH"])
	}
	return splitGoPath[0], nil
}

func filter(vs []string, f func(string) bool) []string {
	vsf := make([]string, 0)
	for _, v := range vs {
		if f(v) {
			vsf = append(vsf, v)
		}
	}
	return vsf
}

// Name returns the name of this platform
func (goPlatform *Platform) Name() string {
	return pb.ChaincodeSpec_GOLANG.String()
}

// ValidateSpec validates Go chaincodes
func (goPlatform *Platform) ValidatePath(rawPath string) error {
	path, err := url.Parse(rawPath)
	if err != nil || path == nil {
		return fmt.Errorf("invalid path: %s", err)
	}

	//we have no real good way of checking existence of remote urls except by downloading and testing
	//which we do later anyway. But we *can* - and *should* - test for existence of local paths.
	//Treat empty scheme as a local filesystem path
	if path.Scheme == "" {
		gopath, err := getGopath()
		if err != nil {
			return err
		}
		pathToCheck := filepath.Join(gopath, "src", rawPath)
		exists, err := pathExists(pathToCheck)
		if err != nil {
			return fmt.Errorf("error validating chaincode path: %s", err)
		}
		if !exists {
			return fmt.Errorf("path to chaincode does not exist: %s", pathToCheck)
		}
	}
	return nil
}

func (goPlatform *Platform) ValidateCodePackage(code []byte) error {

	if len(code) == 0 {
		// Nothing to validate if no CodePackage was included
		return nil
	}

	// FAB-2122: Scan the provided tarball to ensure it only contains source-code under
	// /src/$packagename.  We do not want to allow something like ./pkg/shady.a to be installed under
	// $GOPATH within the container.  Note, we do not look deeper than the path at this time
	// with the knowledge that only the go/cgo compiler will execute for now.  We will remove the source
	// from the system after the compilation as an extra layer of protection.
	//
	// It should be noted that we cannot catch every threat with these techniques.  Therefore,
	// the container itself needs to be the last line of defense and be configured to be
	// resilient in enforcing constraints. However, we should still do our best to keep as much
	// garbage out of the system as possible.
	re := regexp.MustCompile(`^(/)?(src|META-INF)/.*`)
	is := bytes.NewReader(code)
	gr, err := gzip.NewReader(is)
	if err != nil {
		return fmt.Errorf("failure opening codepackage gzip stream: %s", err)
	}
	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err != nil {
			// We only get here if there are no more entries to scan
			break
		}

		// --------------------------------------------------------------------------------------
		// Check name for conforming path
		// --------------------------------------------------------------------------------------
		if !re.MatchString(header.Name) {
			return fmt.Errorf("illegal file detected in payload: \"%s\"", header.Name)
		}

		// --------------------------------------------------------------------------------------
		// Check that file mode makes sense
		// --------------------------------------------------------------------------------------
		// Acceptable flags:
		//      ISREG      == 0100000
		//      -rw-rw-rw- == 0666
		//
		// Anything else is suspect in this context and will be rejected
		// --------------------------------------------------------------------------------------
		if header.Mode&^0100666 != 0 {
			return fmt.Errorf("illegal file mode detected for file %s: %o", header.Name, header.Mode)
		}
	}

	return nil
}

// Vendor any packages that are not already within our chaincode's primary package
// or vendored by it.  We take the name of the primary package and a list of files
// that have been previously determined to comprise the package's dependencies.
// For anything that needs to be vendored, we simply update its path specification.
// Everything else, we pass through untouched.
func vendorDependencies(pkg string, files Sources) {

	exclusions := make([]string, 0)
	elements := strings.Split(pkg, "/")

	// --------------------------------------------------------------------------------------
	// First, add anything already vendored somewhere within our primary package to the
	// "exclusions".  For a package "foo/bar/baz", we want to ensure we don't auto-vendor
	// any of the following:
	//
	//     [ "foo/vendor", "foo/bar/vendor", "foo/bar/baz/vendor"]
	//
	// and we therefore employ a recursive path building process to form this list
	// --------------------------------------------------------------------------------------
	prev := filepath.Join("src")
	for _, element := range elements {
		curr := filepath.Join(prev, element)
		vendor := filepath.Join(curr, "vendor")
		exclusions = append(exclusions, vendor)
		prev = curr
	}

	// --------------------------------------------------------------------------------------
	// Next add our primary package to the list of "exclusions"
	// --------------------------------------------------------------------------------------
	exclusions = append(exclusions, filepath.Join("src", pkg))

	count := len(files)
	sem := make(chan bool, count)

	// --------------------------------------------------------------------------------------
	// Now start a parallel process which checks each file in files to see if it matches
	// any of the excluded patterns.  Any that match are renamed such that they are vendored
	// under src/$pkg/vendor.
	// --------------------------------------------------------------------------------------
	vendorPath := filepath.Join("src", pkg, "vendor")
	for i, file := range files {
		go func(i int, file SourceDescriptor) {
			excluded := false

			for _, exclusion := range exclusions {
				if strings.HasPrefix(file.Name, exclusion) == true {
					excluded = true
					break
				}
			}

			if excluded == false {
				origName := file.Name
				file.Name = strings.Replace(origName, "src", vendorPath, 1)
				logger.Debugf("vendoring %s -> %s", origName, file.Name)
			}

			files[i] = file
			sem <- true
		}(i, file)
	}

	for i := 0; i < count; i++ {
		<-sem
	}
}

// Generates a deployment payload for GOLANG as a series of src/$pkg entries in .tar.gz format
func (goPlatform *Platform) GetDeploymentPayload(path string) ([]byte, error) {

	var err error

	// --------------------------------------------------------------------------------------
	// retrieve a CodeDescriptor from either HTTP or the filesystem
	// --------------------------------------------------------------------------------------
	code, err := getCode(path)
	if err != nil {
		return nil, err
	}
	if code.Cleanup != nil {
		defer code.Cleanup()
	}

	// --------------------------------------------------------------------------------------
	// Update our environment for the purposes of executing go-list directives
	// --------------------------------------------------------------------------------------
	env, err := getGoEnv()
	if err != nil {
		return nil, err
	}
	gopaths := splitEnvPaths(env["GOPATH"])
	goroots := splitEnvPaths(env["GOROOT"])
	gopaths[code.Gopath] = true
	env["GOPATH"] = flattenEnvPaths(gopaths)

	// --------------------------------------------------------------------------------------
	// Retrieve the list of first-order imports referenced by the chaincode
	// --------------------------------------------------------------------------------------
	imports, err := listImports(env, code.Pkg)
	if err != nil {
		return nil, fmt.Errorf("Error obtaining imports: %s", err)
	}

	// --------------------------------------------------------------------------------------
	// Remove any imports that are provided by the ccenv or system
	// --------------------------------------------------------------------------------------
	var provided = map[string]bool{
		"github.com/hyperledger/fabric/core/chaincode/shim": true,
		"github.com/hyperledger/fabric/protos/peer":         true,
	}

	// Golang "pseudo-packages" - packages which don't actually exist
	var pseudo = map[string]bool{
		"C": true,
	}

	imports = filter(imports, func(pkg string) bool {
		// Drop if provided by CCENV
		if _, ok := provided[pkg]; ok == true {
			logger.Debugf("Discarding provided package %s", pkg)
			return false
		}

		// Drop pseudo-packages
		if _, ok := pseudo[pkg]; ok == true {
			logger.Debugf("Discarding pseudo-package %s", pkg)
			return false
		}

		// Drop if provided by GOROOT
		for goroot := range goroots {
			fqp := filepath.Join(goroot, "src", pkg)
			exists, err := pathExists(fqp)
			if err == nil && exists {
				logger.Debugf("Discarding GOROOT package %s", pkg)
				return false
			}
		}

		// Else, we keep it
		logger.Debugf("Accepting import: %s", pkg)
		return true
	})

	// --------------------------------------------------------------------------------------
	// Assemble the fully resolved list of direct and transitive dependencies based on the
	// imports that remain after filtering
	// --------------------------------------------------------------------------------------
	deps := make(map[string]bool)

	for _, pkg := range imports {
		// ------------------------------------------------------------------------------
		// Resolve direct import's transitives
		// ------------------------------------------------------------------------------
		transitives, err := listDeps(env, pkg)
		if err != nil {
			return nil, fmt.Errorf("Error obtaining dependencies for %s: %s", pkg, err)
		}

		// ------------------------------------------------------------------------------
		// Merge all results with our top list
		// ------------------------------------------------------------------------------

		// Merge direct dependency...
		deps[pkg] = true

		// .. and then all transitives
		for _, dep := range transitives {
			deps[dep] = true
		}
	}

	// cull "" if it exists
	delete(deps, "")

	// --------------------------------------------------------------------------------------
	// Find the source from our first-order code package ...
	// --------------------------------------------------------------------------------------
	fileMap, err := findSource(code.Gopath, code.Pkg)
	if err != nil {
		return nil, err
	}

	// --------------------------------------------------------------------------------------
	// ... followed by the source for any non-system dependencies that our code-package has
	// from the filtered list
	// --------------------------------------------------------------------------------------
	for dep := range deps {

		logger.Debugf("processing dep: %s", dep)

		// Each dependency should either be in our GOPATH or GOROOT.  We are not interested in packaging
		// any of the system packages.  However, the official way (go-list) to make this determination
		// is too expensive to run for every dep.  Therefore, we cheat.  We assume that any packages that
		// cannot be found must be system packages and silently skip them
		for gopath := range gopaths {
			fqp := filepath.Join(gopath, "src", dep)
			exists, err := pathExists(fqp)

			logger.Debugf("checking: %s exists: %v", fqp, exists)

			if err == nil && exists {

				// We only get here when we found it, so go ahead and load its code
				files, err := findSource(gopath, dep)
				if err != nil {
					return nil, err
				}

				// Merge the map manually
				for _, file := range files {
					fileMap[file.Name] = file
				}
			}
		}
	}

	logger.Debugf("done")

	// --------------------------------------------------------------------------------------
	// Reprocess into a list for easier handling going forward
	// --------------------------------------------------------------------------------------
	files := make(Sources, 0)
	for _, file := range fileMap {
		files = append(files, file)
	}

	// --------------------------------------------------------------------------------------
	// Remap non-package dependencies to package/vendor
	// --------------------------------------------------------------------------------------
	vendorDependencies(code.Pkg, files)

	// --------------------------------------------------------------------------------------
	// Sort on the filename so the tarball at least looks sane in terms of package grouping
	// --------------------------------------------------------------------------------------
	sort.Sort(files)

	// --------------------------------------------------------------------------------------
	// Write out our tar package
	// --------------------------------------------------------------------------------------
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	for _, file := range files {

		// file.Path represents os localpath
		// file.Name represents tar packagepath

		// If the file is metadata rather than golang code, remove the leading go code path, for example:
		// original file.Name:  src/github.com/hyperledger/fabric/examples/chaincode/go/marbles02/META-INF/statedb/couchdb/indexes/indexOwner.json
		// updated file.Name:   META-INF/statedb/couchdb/indexes/indexOwner.json
		if file.IsMetadata {

			file.Name, err = filepath.Rel(filepath.Join("src", code.Pkg), file.Name)
			if err != nil {
				return nil, fmt.Errorf("This error was caused by bad packaging of the metadata.  The file [%s] is marked as MetaFile, however not located under META-INF   Error:[%s]", file.Name, err)
			}

			// Split the tar location (file.Name) into a tar package directory and filename
			_, filename := filepath.Split(file.Name)

			// Hidden files are not supported as metadata, therefore ignore them.
			// User often doesn't know that hidden files are there, and may not be able to delete them, therefore warn user rather than error out.
			if strings.HasPrefix(filename, ".") {
				logger.Warningf("Ignoring hidden file in metadata directory: %s", file.Name)
				continue
			}

			fileBytes, err := ioutil.ReadFile(file.Path)
			if err != nil {
				return nil, err
			}

			// Validate metadata file for inclusion in tar
			// Validation is based on the passed filename with path
			err = ccmetadata.ValidateMetadataFile(file.Name, fileBytes)
			if err != nil {
				return nil, err
			}
		}

		err = cutil.WriteFileToPackage(file.Path, file.Name, tw)
		if err != nil {
			return nil, fmt.Errorf("Error writing %s to tar: %s", file.Name, err)
		}
	}

	err = tw.Close()
	if err == nil {
		err = gw.Close()
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create tar for chaincode")
	}

	return payload.Bytes(), nil
}

func (goPlatform *Platform) GenerateDockerfile() (string, error) {

	var buf []string

	buf = append(buf, "FROM "+cutil.GetDockerfileFromConfig("chaincode.golang.runtime"))
	buf = append(buf, "ADD binpackage.tar /usr/local/bin")

	dockerFileContents := strings.Join(buf, "\n")

	return dockerFileContents, nil
}

const staticLDFlagsOpts = "-ldflags \"-linkmode external -extldflags '-static'\""
const dynamicLDFlagsOpts = ""

func getLDFlagsOpts() string {
	if viper.GetBool("chaincode.golang.dynamicLink") {
		return dynamicLDFlagsOpts
	}
	return staticLDFlagsOpts
}

func (goPlatform *Platform) GenerateDockerBuild(path string, code []byte, tw *tar.Writer) error {
	pkgname, err := decodeUrl(path)
	if err != nil {
		return fmt.Errorf("could not decode url: %s", err)
	}

	ldflagsOpt := getLDFlagsOpts()
	logger.Infof("building chaincode with ldflagsOpt: '%s'", ldflagsOpt)

	codepackage := bytes.NewReader(code)
	binpackage := bytes.NewBuffer(nil)
	err = util.DockerBuild(util.DockerBuildOptions{
		Cmd:          fmt.Sprintf("GOPATH=/chaincode/input:$GOPATH go build  %s -o /chaincode/output/chaincode %s", ldflagsOpt, pkgname),
		InputStream:  codepackage,
		OutputStream: binpackage,
	})
	if err != nil {
		return err
	}

	return cutil.WriteBytesToPackage("binpackage.tar", binpackage.Bytes(), tw)
}

//GetMetadataProvider fetches metadata provider given deployment spec
func (goPlatform *Platform) GetMetadataProvider(code []byte) platforms.MetadataProvider {
	return &ccmetadata.TargzMetadataProvider{Code: code}
}
