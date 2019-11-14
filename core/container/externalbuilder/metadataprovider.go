/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type MetadataProvider struct {
	DurablePath string
}

// PackageMetadata returns a set of bytes encoded as a tar file, containing
// the release metadata as provided by the external builder.  If no directory
// with build output from the external builder is found, the tar bytes will
// be nil.  If the build output is found, but there is no metadata, the bytes
// will be an empty tar.  An error is returned only if the build output is
// found but some other error occurs.
func (mp *MetadataProvider) PackageMetadata(ccid string) ([]byte, error) {
	releasePath := filepath.Join(mp.DurablePath, SanitizeCCIDPath(ccid), "release")

	_, err := os.Stat(releasePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithMessage(err, "could not stat path")
	}

	buffer := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buffer)

	logger.Debugf("Walking package release dir '%s'", releasePath)
	err = filepath.Walk(releasePath, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(fi, file)
		if err != nil {
			return err
		}

		name, err := filepath.Rel(releasePath, file)
		if err != nil {
			return err
		}

		header.Name = filepath.Join("META-INF", name)
		if fi.IsDir() {
			header.Name += "/"
		}

		logger.Debugf("Adding file '%s' to tar with header name '%s'", file, header.Name)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		data, err := os.Open(file)
		if err != nil {
			return errors.WithMessage(err, "could not open file")
		}

		if _, err := io.Copy(tw, data); err != nil {
			return errors.WithMessage(err, "could not copy file into tar")
		}

		return nil
	})
	if err != nil {
		return nil, errors.WithMessage(err, "could not walk filepath")
	}

	if err := tw.Close(); err != nil {
		return nil, errors.WithMessage(err, "could not write tar")
	}

	return buffer.Bytes(), err
}
