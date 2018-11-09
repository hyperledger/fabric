/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statsd

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("This is the thing", func() {
	var n *namer

	BeforeEach(func() {
		n = &namer{
			namespace:  "namespace",
			subsystem:  "subsystem",
			name:       "name",
			nameFormat: "prefix.%{#namespace}.%{#subsystem}.%{#name}.%{alpha}.bravo.%{bravo}.suffix",
			labelNames: map[string]struct{}{
				"alpha": {},
				"bravo": {},
			},
		}
	})

	It("formats names from labels", func() {
		name := n.Format("alpha", "a", "bravo", "b")
		Expect(name).To(Equal("prefix.namespace.subsystem.name.a.bravo.b.suffix"))
	})

	Context("when the wrong labels are provided", func() {
		It("panics", func() {
			recovered := func() (recovered interface{}) {
				defer func() { recovered = recover() }()
				n.Format("charlie", "c", "delta", "d")
				return
			}()
			Expect(recovered).To(Equal("invalid label name: charlie"))
		})
	})

	Context("when the format references an unknown label", func() {
		BeforeEach(func() {
			n.nameFormat = "%{bad_label}"
		})

		It("panics", func() {
			recovered := func() (recovered interface{}) {
				defer func() { recovered = recover() }()
				n.Format("alpha", "a", "bravo", "b")
				return
			}()
			Expect(recovered).To(Equal("invalid label in name format: bad_label"))
		})
	})

	Context("when labels are missing", func() {
		It("uses unknown for the missing value", func() {
			name := n.Format("alpha", "a", "bravo")
			Expect(name).To(Equal("prefix.namespace.subsystem.name.a.bravo.unknown.suffix"))
		})
	})

	Context("when label values contain invalid characters", func() {
		It("replaces them with underscores", func() {
			name := n.Format("alpha", ":colon:colon:", "bravo", "|bar|bar|")
			Expect(name).To(Equal("prefix.namespace.subsystem.name._colon_colon_.bravo._bar_bar_.suffix"))
		})
	})

	Context("when label values contain new line, spaces, or tabs", func() {
		It("replaces them with underscores", func() {
			name := n.Format("alpha", "a\nb\tc", "bravo", "b c")
			Expect(name).To(Equal("prefix.namespace.subsystem.name.a_b_c.bravo.b_c.suffix"))
		})
	})

	Context("when label values contain periods", func() {
		It("replaces them with underscores", func() {
			name := n.Format("alpha", "period.period", "bravo", "...")
			Expect(name).To(Equal("prefix.namespace.subsystem.name.period_period.bravo.___.suffix"))
		})
	})

	Context("when label values contain multi-byte utf8 runes", func() {
		It("leaves them alone", func() {
			name := n.Format("alpha", "Ʊpsilon", "bravo", "b")
			Expect(name).To(Equal("prefix.namespace.subsystem.name.Ʊpsilon.bravo.b.suffix"))
		})
	})

	DescribeTable("#fqname",
		func(n *namer, expectedName string) {
			n.nameFormat = "%{#fqname}"
			Expect(n.Format()).To(Equal(expectedName))
		},
		Entry("missing nothing", &namer{namespace: "namespace", subsystem: "subsystem", name: "name"}, "namespace.subsystem.name"),
		Entry("missing namespace", &namer{namespace: "", subsystem: "subsystem", name: "name"}, "subsystem.name"),
		Entry("missing subsystem", &namer{namespace: "namespace", subsystem: "", name: "name"}, "namespace.name"),
		Entry("missing namespace and subsystem", &namer{namespace: "", subsystem: "", name: "name"}, "name"),
	)
})
