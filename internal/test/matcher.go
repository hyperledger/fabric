/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"fmt"
	"strings"

	"github.com/onsi/gomega/types"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// ProtoEqual provides a Gomega matcher that uses proto.Equal to compare actual with expected.
func ProtoEqual(expected proto.Message) types.GomegaMatcher {
	return &protoEqualMatcher{
		expected: expected,
		marshal: prototext.MarshalOptions{
			Multiline:    true,
			Indent:       "\t",
			AllowPartial: true,
		},
	}
}

type protoEqualMatcher struct {
	expected proto.Message
	marshal  prototext.MarshalOptions
}

func (m *protoEqualMatcher) Match(actual interface{}) (bool, error) {
	switch message := actual.(type) {
	case proto.Message:
		return proto.Equal(m.expected, message), nil
	default:
		return false, fmt.Errorf("ProtoEqual expects a proto.Message, got a %T", actual)
	}
}

func (m *protoEqualMatcher) FailureMessage(actual interface{}) string {
	switch message := actual.(type) {
	case proto.Message:
		return fmt.Sprintf("Expected\n%s\nto equal\n%s", m.indent(m.format(message)), m.indent(m.format(m.expected)))
	default:
		return fmt.Sprintf("Expected\n\t%v\nto equal\n%s", message, m.indent(m.format(m.expected)))
	}
}

func (m *protoEqualMatcher) indent(text string) string {
	return m.marshal.Indent + strings.ReplaceAll(text, "\n", "\n"+m.marshal.Indent)
}

func (m *protoEqualMatcher) NegatedFailureMessage(actual interface{}) string {
	switch message := actual.(type) {
	case proto.Message:
		return fmt.Sprintf("Expected\n%s\nnot to equal\n%s", m.indent(m.format(message)), m.indent(m.format(m.expected)))
	default:
		return fmt.Sprintf("Expected\n\t%v\nnot to equal\n%s", message, m.indent(m.format(m.expected)))
	}
}

func (m *protoEqualMatcher) format(message proto.Message) string {
	formatted := strings.TrimSpace(m.marshal.Format(message))
	return fmt.Sprintf("%s{\n%s\n}", messageType(message), m.indent(formatted))
}

func messageType(message proto.Message) string {
	return string(message.ProtoReflect().Descriptor().Name())
}
