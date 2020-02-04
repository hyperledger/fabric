/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"text/template"

	"github.com/hyperledger/fabric/common/metrics/gendoc"
	"golang.org/x/tools/go/packages"
)

// Gendoc can be used used to discover the metrics options declared at the
// package level in the fabric tree and output a table that can be used in the
// documentation.

var templatePath = flag.String(
	"template",
	"docs/source/metrics_reference.rst.tmpl",
	"The documentation template.",
)

func createFuncMap() template.FuncMap {
	return template.FuncMap{
		"OrdererPrometheusTable": emptyStrFunc,
		"OrdererStatsdTable":     emptyStrFunc,
		"PeerPrometheusTable":    emptyStrFunc,
		"PeerStatsdTable":        emptyStrFunc,
		"IsCA": func() bool {
			return false
		},
		"PrometheusTable": emptyStrFunc,
		"StatsdTable":     emptyStrFunc,
		"PrometheusDescription": func() string {
			return "The following metrics are currently exported for consumption by Prometheus.\n" +
				"Label provides the names of the labels that can be attached to a metric. " +
				"For example, a `channel` label denotes a channel is associated with this metric."
		},
		"StatsDDescription": func() string {
			return fmt.Sprintf("The following metrics are currently emitted for consumption by" +
				"StatsD. The ``%%{variable_name}`` nomenclature represents segments that" +
				"vary based on context.\n\n" +
				"For example, %%{channel} will be replaced with the name of the channel" +
				"associated with the metric.")
		},
	}
}

func emptyStrFunc() string {
	return ""
}

func createCells(patterns []string) (gendoc.Cells, error) {

	pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadSyntax}, patterns...)
	if err != nil {
		return nil, err
	}

	options, err := gendoc.Options(pkgs)
	if err != nil {
		return nil, err
	}

	cells, err := gendoc.NewCells(options)
	if err != nil {
		return nil, err
	}

	return cells, err

}

func createStatsdTable(cells gendoc.Cells) string {
	buf := &bytes.Buffer{}
	gendoc.NewStatsdTable(cells).Generate(buf)
	return buf.String()
}

func createPrometheusTable(cells gendoc.Cells) string {
	buf := &bytes.Buffer{}
	gendoc.NewPrometheusTable(cells).Generate(buf)
	return buf.String()
}

func main() {
	flag.Parse()

	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"github.com/hyperledger/fabric/..."}
	}

	var peerpatterns, ordererpatterns []string
	for i, pattern := range patterns {
		if pattern == "peerpackages" {
			peerpatterns = patterns[i+1:]
			ordererpatterns = patterns[1 : i-1]
			break
		}
	}

	funcMap := createFuncMap()

	if len(peerpatterns) == 0 && len(ordererpatterns) == 0 {

		defaultCells, err := createCells(patterns)
		if err != nil {
			panic(err)
		}

		funcMap["PrometheusTable"] = func() string {
			return createPrometheusTable(defaultCells)
		}

		funcMap["StatsdTable"] = func() string {
			return createStatsdTable(defaultCells)
		}

		funcMap["IsCA"] = func() string {
			return "true"
		}

	} else {

		ordererCells, err := createCells(ordererpatterns)
		if err != nil {
			panic(err)
		}

		peerCells, err := createCells(peerpatterns)
		if err != nil {
			panic(err)
		}

		funcMap["OrdererPrometheusTable"] = func() string {
			return createPrometheusTable(ordererCells)
		}

		funcMap["OrdererStatsdTable"] = func() string {
			return createStatsdTable(ordererCells)
		}

		funcMap["PeerPrometheusTable"] = func() string {
			return createPrometheusTable(peerCells)
		}

		funcMap["PeerStatsdTable"] = func() string {
			return createStatsdTable(peerCells)
		}

	}

	docTemplate, err := ioutil.ReadFile(*templatePath)
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
