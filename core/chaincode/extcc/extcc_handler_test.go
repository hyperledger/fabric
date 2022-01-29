/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package extcc_test

import (
	"net"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/chaincode/extcc"
	"github.com/hyperledger/fabric/core/chaincode/extcc/mock"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/internal/pkg/comm"

	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Extcc", func() {
	var (
		i        *extcc.ExternalChaincodeRuntime
		shandler *mock.StreamHandler
	)

	BeforeEach(func() {
		shandler = &mock.StreamHandler{}
		i = &extcc.ExternalChaincodeRuntime{}
	})

	Context("Run", func() {
		When("chaincode is running", func() {
			var (
				cclist net.Listener
				ccserv *grpc.Server
			)
			BeforeEach(func() {
				var err error
				cclist, err = net.Listen("tcp", "127.0.0.1:0")
				Expect(err).To(BeNil())
				Expect(cclist).To(Not(BeNil()))
				ccserv = grpc.NewServer([]grpc.ServerOption{}...)
				go ccserv.Serve(cclist)
			})

			AfterEach(func() {
				if ccserv != nil {
					ccserv.Stop()
				}
				if cclist != nil {
					cclist.Close()
				}
			})

			It("runs to completion", func() {
				ccinfo := &ccintf.ChaincodeServerInfo{
					Address: cclist.Addr().String(),
					ClientConfig: comm.ClientConfig{
						KaOpts:      comm.DefaultKeepaliveOptions,
						DialTimeout: 10 * time.Second,
					},
				}
				err := i.Stream("ccid", ccinfo, shandler)
				Expect(err).To(BeNil())
				Expect(shandler.HandleChaincodeStreamCallCount()).To(Equal(1))

				streamArg := shandler.HandleChaincodeStreamArgsForCall(0)
				Expect(streamArg).To(Not(BeNil()))
			})
		})
		Context("chaincode info incorrect", func() {
			var ccinfo *ccintf.ChaincodeServerInfo
			BeforeEach(func() {
				ca, err := tlsgen.NewCA()
				Expect(err).NotTo(HaveOccurred())
				ckp, err := ca.NewClientCertKeyPair()
				Expect(err).NotTo(HaveOccurred())
				ccinfo = &ccintf.ChaincodeServerInfo{
					Address: "ccaddress:12345",
					ClientConfig: comm.ClientConfig{
						SecOpts: comm.SecureOptions{
							UseTLS:            true,
							RequireClientCert: true,
							Certificate:       ckp.Cert,
							Key:               ckp.Key,
							ServerRootCAs:     [][]byte{ca.CertBytes()},
						},
						DialTimeout: 10 * time.Second,
					},
				}
			})
			When("address is bad", func() {
				BeforeEach(func() {
					ccinfo.ClientConfig.SecOpts.UseTLS = false
					ccinfo.Address = "<badaddress>"
				})
				It("returns an error", func() {
					err := i.Stream("ccid", ccinfo, shandler)
					Expect(err).To(MatchError(ContainSubstring("error creating grpc connection to <badaddress>")))
				})
			})
			When("unspecified client spec", func() {
				BeforeEach(func() {
					ccinfo.ClientConfig.SecOpts.Key = nil
				})
				It("returns an error", func() {
					err := i.Stream("ccid", ccinfo, shandler)
					Expect(err).To(MatchError(ContainSubstring("both Key and Certificate are required when using mutual TLS")))
				})
			})
		})
	})
})
