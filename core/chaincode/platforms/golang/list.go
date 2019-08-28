/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/pkg/errors"
)

const listFormat = `
{{- if eq .Goroot false -}}
{
    "import_path": "{{ .ImportPath }}",
    "incomplete": {{ .Incomplete }},
    "dir": "{{ .Dir }}",
    "go_files" : [{{ range $i, $file := .GoFiles  }}{{ if $i }}, {{ end }}"{{ $file }}"{{end}}],
    "c_files":   [{{ range $i, $file := .CFiles   }}{{ if $i }}, {{ end }}"{{ $file }}"{{end}}],
    "cgo_files": [{{ range $i, $file := .CgoFiles }}{{ if $i }}, {{ end }}"{{ $file }}"{{end}}],
    "h_files":   [{{ range $i, $file := .HFiles   }}{{ if $i }}, {{ end }}"{{ $file }}"{{end}}],
    "s_files":   [{{ range $i, $file := .SFiles   }}{{ if $i }}, {{ end }}"{{ $file }}"{{end}}],
    "ignored_go_files": [{{ range $i, $file := .IgnoredGoFiles }}{{ if $i }}, {{ end }}"{{ $file }}"{{end}}]
}
{{- end -}}`

type PackageInfo struct {
	ImportPath     string   `json:"import_path,omitempty"`
	Dir            string   `json:"dir,omitempty"`
	GoFiles        []string `json:"go_files,omitempty"`
	CFiles         []string `json:"c_files,omitempty"`
	CgoFiles       []string `json:"cgo_files,omitempty"`
	HFiles         []string `json:"h_files,omitempty"`
	SFiles         []string `json:"s_files,omitempty"`
	IgnoredGoFiles []string `json:"ignored_go_files,omitempty"`
	Incomplete     bool     `json:"incomplete,omitempty"`
}

func (p PackageInfo) Files() []string {
	var files []string
	files = append(files, p.GoFiles...)
	files = append(files, p.CFiles...)
	files = append(files, p.CgoFiles...)
	files = append(files, p.HFiles...)
	files = append(files, p.SFiles...)
	files = append(files, p.IgnoredGoFiles...)
	return files
}

func dependencyPackageInfo(goos, goarch, pkg string) ([]PackageInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", listFormat, pkg)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(stdout)

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	var list []PackageInfo
	for {
		var packageInfo PackageInfo
		err := decoder.Decode(&packageInfo)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if packageInfo.Incomplete {
			return nil, fmt.Errorf("failed to calculate dependencies: incomplete package: %s", packageInfo.ImportPath)
		}

		list = append(list, packageInfo)
	}

	err = cmd.Wait()
	if err != nil {
		return nil, errors.Wrapf(err, "listing deps for pacakge %s failed", pkg)
	}

	return list, nil
}
