/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"flag"
	"os"
	"text/template"

	"github.com/hyperledger/fabric/common/metrics/gendoc"
	"golang.org/x/tools/go/packages"
)

// Gendoc can be used to discover the metrics options declared at the
// package level in the fabric tree and output a table that can be used in the
// documentation.

var templatePath = flag.String(
	"template",
	"docs/source/metrics_reference.rst.tmpl",
	"The documentation template.",
)

func main() {
	flag.Parse()

	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"github.com/hyperledger/fabric/..."}
	}

	mode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedSyntax | packages.NeedTypesInfo
	pkgs, err := packages.Load(&packages.Config{Mode: mode}, patterns...)
	if err != nil {
		panic(err)
	}

	options, err := gendoc.Options(pkgs)
	if err != nil {
		panic(err)
	}

	cells, err := gendoc.NewCells(options)
	if err != nil {
		panic(err)
	}

	funcMap := template.FuncMap{
		"PrometheusTable": func() string {
			buf := &bytes.Buffer{}
			gendoc.NewPrometheusTable(cells).Generate(buf)
			return buf.String()
		},
		"StatsdTable": func() string {
			buf := &bytes.Buffer{}
			gendoc.NewStatsdTable(cells).Generate(buf)
			return buf.String()
		},
	}

	docTemplate, err := os.ReadFile(*templatePath)
	if err != nil {
		panic(err)
	}

	tmpl, err := template.New("metrics_reference").Funcs(funcMap).Parse(string(docTemplate))
	if err != nil {
		panic(err)
	}

	if err := tmpl.Execute(os.Stdout, ""); err != nil {
		panic(err)
	}
}
