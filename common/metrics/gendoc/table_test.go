/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gendoc_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric/common/metrics/gendoc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Table", func() {
	It("generates a markdown document the prometheus metrics", func() {
		var options []interface{}

		filepath.Walk("testdata", func(path string, info os.FileInfo, err error) error {
			defer GinkgoRecover()
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
				f, err := ParseFile(path)
				Expect(err).NotTo(HaveOccurred())
				opts, err := gendoc.FileOptions(f)
				Expect(err).NotTo(HaveOccurred())
				options = append(options, opts...)
			}
			return nil
		})

		cells, err := gendoc.NewCells(options)
		Expect(err).NotTo(HaveOccurred())

		buf := &bytes.Buffer{}
		w := io.MultiWriter(buf, GinkgoWriter)

		gendoc.NewPrometheusTable(cells).Generate(w)
		Expect(buf.String()).To(Equal(strings.TrimPrefix(goldenPromTable, "\n")))
	})

	It("generates a markdown document the statsd metrics", func() {
		var options []interface{}

		filepath.Walk("testdata", func(path string, info os.FileInfo, err error) error {
			defer GinkgoRecover()
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
				f, err := ParseFile(path)
				Expect(err).NotTo(HaveOccurred())
				opts, err := gendoc.FileOptions(f)
				Expect(err).NotTo(HaveOccurred())
				options = append(options, opts...)
			}
			return nil
		})

		cells, err := gendoc.NewCells(options)
		Expect(err).NotTo(HaveOccurred())

		buf := &bytes.Buffer{}
		w := io.MultiWriter(buf, GinkgoWriter)

		gendoc.NewStatsdTable(cells).Generate(w)
		Expect(buf.String()).To(Equal(strings.TrimPrefix(goldenStatsdTable, "\n")))
	})
})

const goldenPromTable = `
+--------------------------+-----------+------------------------------------------------------------+--------------------------------------------------------------------------------+
| Name                     | Type      | Description                                                | Labels                                                                         |
+==========================+===========+============================================================+==============+=================================================================+
| fixtures_counter         | counter   | This is some help text that is more than a few words long. | label_one    | this is a really cool label that is the first of many           |
|                          |           | It really can be quite long. Really long.                  +--------------+-----------------------------------------------------------------+
|                          |           |                                                            | label_two    | short and sweet                                                 |
|                          |           |                                                            +--------------+-----------------------------------------------------------------+
|                          |           |                                                            | missing_help |                                                                 |
+--------------------------+-----------+------------------------------------------------------------+--------------+-----------------------------------------------------------------+
| fixtures_gauge           | gauge     | This is some help text that is more than a few words long. | label_one    |                                                                 |
|                          |           | It really can be quite long. Really long. This is some     +--------------+-----------------------------------------------------------------+
|                          |           | help text that is more than a few words long. It really    | label_two    |                                                                 |
|                          |           | can be quite long. Really long.                            |              |                                                                 |
+--------------------------+-----------+------------------------------------------------------------+--------------+-----------------------------------------------------------------+
| fixtures_histogram       | histogram | This is some help text                                     | label_one    | This is a very long help message for label_one, which could be  |
|                          |           |                                                            |              | really, really long, and it may never end...                    |
|                          |           |                                                            +--------------+-----------------------------------------------------------------+
|                          |           |                                                            | label_two    |                                                                 |
+--------------------------+-----------+------------------------------------------------------------+--------------+-----------------------------------------------------------------+
| namespace_counter_name   | counter   | This is some help text                                     | label_one    |                                                                 |
|                          |           |                                                            +--------------+-----------------------------------------------------------------+
|                          |           |                                                            | label_two    |                                                                 |
+--------------------------+-----------+------------------------------------------------------------+--------------+-----------------------------------------------------------------+
| namespace_gauge_name     | gauge     | This is some help text                                     | label_one    |                                                                 |
|                          |           |                                                            +--------------+-----------------------------------------------------------------+
|                          |           |                                                            | label_two    |                                                                 |
+--------------------------+-----------+------------------------------------------------------------+--------------+-----------------------------------------------------------------+
| namespace_histogram_name | histogram | This is some help text                                     | label_one    |                                                                 |
|                          |           |                                                            +--------------+-----------------------------------------------------------------+
|                          |           |                                                            | label_two    |                                                                 |
+--------------------------+-----------+------------------------------------------------------------+--------------+-----------------------------------------------------------------+
`

const goldenStatsdTable = `
+----------------------------------------------------+-----------+------------------------------------------------------------+
| Bucket                                             | Type      | Description                                                |
+====================================================+===========+============================================================+
| fixtures.counter.%{label_one}.%{label_two}         | counter   | This is some help text that is more than a few words long. |
|                                                    |           | It really can be quite long. Really long.                  |
+----------------------------------------------------+-----------+------------------------------------------------------------+
| fixtures.gauge.%{label_one}.%{label_two}           | gauge     | This is some help text that is more than a few words long. |
|                                                    |           | It really can be quite long. Really long. This is some     |
|                                                    |           | help text that is more than a few words long. It really    |
|                                                    |           | can be quite long. Really long.                            |
+----------------------------------------------------+-----------+------------------------------------------------------------+
| fixtures.histogram.%{label_one}.%{label_two}       | histogram | This is some help text                                     |
+----------------------------------------------------+-----------+------------------------------------------------------------+
| namespace.counter.name.%{label_one}.%{label_two}   | counter   | This is some help text                                     |
+----------------------------------------------------+-----------+------------------------------------------------------------+
| namespace.gauge.name.%{label_one}.%{label_two}     | gauge     | This is some help text                                     |
+----------------------------------------------------+-----------+------------------------------------------------------------+
| namespace.histogram.name.%{label_one}.%{label_two} | histogram | This is some help text                                     |
+----------------------------------------------------+-----------+------------------------------------------------------------+
`
