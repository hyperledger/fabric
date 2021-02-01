/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// Untar takes a gzip-ed tar archive, and extracts it to dst.
// It returns an error if the tar contains any files which would escape to a
// parent of dst, or if the archive contains any files whose type is not
// a regular file or directory.
func Untar(buffer io.Reader, dst string) error {
	gzr, err := gzip.NewReader(buffer)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		if err == io.EOF {
			return nil
		}

		if err != nil {
			return errors.WithMessage(err, "could not get next tar element")
		}

		if !ValidPath(header.Name) {
			return errors.Errorf("tar contains the absolute or escaping path '%s'", header.Name)
		}

		target := filepath.Join(dst, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return errors.WithMessagef(err, "could not create directory '%s'", header.Name)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return errors.WithMessagef(err, "could not create directory '%s'", filepath.Dir(header.Name))
			}

			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return errors.WithMessagef(err, "could not create file '%s'", header.Name)
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			f.Close()
		default:
			return errors.Errorf("invalid file type '%v' contained in archive for file '%s'", header.Typeflag, header.Name)
		}
	}
}

// ValidPath checks to see if the path is absolute, or if it is a
// relative path higher in the tree.  In these cases it returns false.
func ValidPath(uncleanPath string) bool {
	// sanitizedPath will eliminate non-prefix instances of '..', as well
	// as strip './'
	sanitizedPath := filepath.Clean(uncleanPath)

	switch {
	case filepath.IsAbs(sanitizedPath):
		return false
	case strings.HasPrefix(sanitizedPath, ".."+string(filepath.Separator)) || sanitizedPath == "..":
		// Path refers either to the parent, or a directory relative to the parent (but allows ..foo or ... for instance)
		return false
	default:
		// Path appears to be relative without escaping higher
		return true
	}
}
