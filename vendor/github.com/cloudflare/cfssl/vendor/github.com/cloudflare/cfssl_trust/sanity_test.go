package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/ubiquity"
)

var metadataFiles = []string{
	"ca-bundle.crt.metadata",
}

func TestMetadataFormat(t *testing.T) {
	for _, file := range metadataFiles {
		if err := ubiquity.LoadPlatforms(file); err != nil {
			t.Fatal(err)
		}
	}
}

var bundleFiles = []string{
	"ca-bundle.crt",
	"int-bundle.crt",
}

func TestParseBundles(t *testing.T) {
	for _, file := range bundleFiles {
		if _, err := helpers.LoadPEMCertPool(file); err != nil {
			t.Fatal(err)
		}
	}
}

var certDirs = []string{
	"ca-bundle",
	"intermediate_ca",
}

func TestParseCertDirs(t *testing.T) {
	for _, dir := range certDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}

			certPEM, err := ioutil.ReadFile(path)
			if err != nil {
				return fmt.Errorf("%s: %v", path, err)
			}

			if _, err = helpers.ParseCertificatePEM(certPEM); err != nil {
				return fmt.Errorf("%s: %v", path, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
