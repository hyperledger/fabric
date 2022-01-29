/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gendoc_test

import (
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/gendoc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Options", func() {
	It("finds standard options", func() {
		f, err := ParseFile("testdata/basic.go")
		Expect(err).NotTo(HaveOccurred())
		Expect(f).NotTo(BeNil())

		options, err := gendoc.FileOptions(f)
		Expect(err).NotTo(HaveOccurred())
		Expect(options).To(HaveLen(3))

		for _, opt := range options {
			switch opt := opt.(type) {
			case metrics.CounterOpts:
				Expect(opt).To(Equal(metrics.CounterOpts{
					Namespace:  "fixtures",
					Name:       "counter",
					Help:       "This is some help text that is more than a few words long. It really can be quite long. Really long.",
					LabelNames: []string{"label_one", "label_two", "missing_help"},
					LabelHelp: map[string]string{
						"label_one": "this is a really cool label that is the first of many",
						"label_two": "short and sweet",
					},
					StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
				}))

			case metrics.GaugeOpts:
				Expect(opt).To(Equal(metrics.GaugeOpts{
					Namespace:    "fixtures",
					Name:         "gauge",
					Help:         "This is some help text that is more than a few words long. It really can be quite long. Really long. This is some help text that is more than a few words long. It really can be quite long. Really long.",
					LabelNames:   []string{"label_one", "label_two"},
					StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
				}))

			case metrics.HistogramOpts:
				Expect(opt).To(Equal(metrics.HistogramOpts{
					Namespace:  "fixtures",
					Name:       "histogram",
					Help:       "This is some help text",
					LabelNames: []string{"label_one", "label_two"},
					LabelHelp: map[string]string{
						"label_one": "This is a very long help message for label_one, which could be really, really long, and it may never end...",
					},
					StatsdFormat: "%{#fqname}.%{label_one}.%{label_two}",
				}))

			default:
				Fail("unexpected option")
			}
		}
	})

	It("finds options that use named imports", func() {
		f, err := ParseFile("testdata/named_import.go")
		Expect(err).NotTo(HaveOccurred())
		Expect(f).NotTo(BeNil())

		options, err := gendoc.FileOptions(f)
		Expect(err).NotTo(HaveOccurred())
		Expect(options).To(HaveLen(3))
	})

	It("ignores variables that are tagged", func() {
		f, err := ParseFile("testdata/ignored.go")
		Expect(err).NotTo(HaveOccurred())
		Expect(f).NotTo(BeNil())

		options, err := gendoc.FileOptions(f)
		Expect(err).NotTo(HaveOccurred())
		Expect(options).To(BeEmpty())
	})
})
