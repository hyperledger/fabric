/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpclogging_test

import (
	"encoding/json"

	"github.com/golang/protobuf/jsonpb"
	"github.com/hyperledger/fabric/common/grpclogging"
	"github.com/hyperledger/fabric/common/grpclogging/testpb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("Fields", func() {
	Describe("ProtoMessage", func() {
		var message *testpb.Message

		BeforeEach(func() {
			message = &testpb.Message{
				Message:  "Je suis une pizza avec du fromage.",
				Sequence: 1337,
			}
		})

		It("creates a reflect field for zap", func() {
			field := grpclogging.ProtoMessage("field-key", message)
			Expect(field.Key).To(Equal("field-key"))
			_, ok := field.Interface.(json.Marshaler)
			Expect(ok).To(BeTrue())
		})

		It("marshals messages compatible with jsonpb", func() {
			field := grpclogging.ProtoMessage("field-key", message)
			marshaler := field.Interface.(json.Marshaler)

			marshaled, err := marshaler.MarshalJSON()
			Expect(err).NotTo(HaveOccurred())

			protoMarshaler := &jsonpb.Marshaler{}
			protoJson, err := protoMarshaler.MarshalToString(message)
			Expect(err).NotTo(HaveOccurred())

			Expect(marshaled).To(MatchJSON(protoJson))
		})

		It("works with zap's json encoder", func() {
			encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
				MessageKey: "message",
			})
			buf, err := encoder.EncodeEntry(
				zapcore.Entry{Message: "Oh là là"},
				[]zapcore.Field{grpclogging.ProtoMessage("proto-message", message)},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(MatchJSON(`{"message": "Oh là là", "proto-message": {"message": "Je suis une pizza avec du fromage.", "sequence": 1337}}`))
		})

		Context("when marshaling the message fails", func() {
			It("it returns the error from marshaling", func() {
				field := grpclogging.ProtoMessage("field-key", badProto{err: errors.New("Boom!")})
				marshaler := field.Interface.(json.Marshaler)

				_, err := marshaler.MarshalJSON()
				Expect(err).To(MatchError("Boom!"))
			})
		})

		Context("when something other than a proto.Message is provided", func() {
			It("creates an any field", func() {
				field := grpclogging.ProtoMessage("field-key", "Je ne suis pas une pizza.")
				Expect(field).To(Equal(zap.Any("field-key", "Je ne suis pas une pizza.")))
			})
		})
	})

	Describe("Error", func() {
		It("creates an error field for zap", func() {
			err := errors.New("error")
			field := grpclogging.Error(err)
			Expect(field.Key).To(Equal("error"))
			Expect(field.Type).To(Equal(zapcore.ErrorType))
			Expect(field.Interface).To(Equal(struct{ error }{err}))
		})

		Context("when the error is nil", func() {
			It("creates a skip field", func() {
				field := grpclogging.Error(nil)
				Expect(field.Type).To(Equal(zapcore.SkipType))
			})
		})

		It("omits the verboseError field", func() {
			encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
				MessageKey: "message",
			})
			buf, err := encoder.EncodeEntry(
				zapcore.Entry{Message: "the message"},
				[]zapcore.Field{grpclogging.Error(errors.New("the error"))},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(MatchJSON(`{"message": "the message", "error": "the error"}`))
		})
	})
})

type badProto struct{ err error }

func (b badProto) Reset()         {}
func (b badProto) String() string { return "" }
func (b badProto) ProtoMessage()  {}
func (b badProto) MarshalJSONPB(*jsonpb.Marshaler) ([]byte, error) {
	return nil, b.err
}
