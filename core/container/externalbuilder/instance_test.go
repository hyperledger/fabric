/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("Instance", func() {
	var (
		logger   *flogging.FabricLogger
		instance *externalbuilder.Instance
	)

	BeforeEach(func() {
		enc := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{MessageKey: "msg"})
		core := zapcore.NewCore(enc, zapcore.AddSync(GinkgoWriter), zap.NewAtomicLevel())
		logger = flogging.NewFabricLogger(zap.New(core).Named("logger"))

		instance = &externalbuilder.Instance{
			PackageID: "test-ccid",
			Builder: &externalbuilder.Builder{
				Location: "testdata/goodbuilder",
				Logger:   logger,
				MSPID:    "mspid",
			},
		}
	})

	Describe("ChaincodeServerInfo", func() {
		BeforeEach(func() {
			var err error
			instance.ReleaseDir, err = ioutil.TempDir("", "cc-conn-test")
			Expect(err).NotTo(HaveOccurred())

			err = os.MkdirAll(filepath.Join(instance.ReleaseDir, "chaincode", "server"), 0o755)
			Expect(err).NotTo(HaveOccurred())
			// initialize with a well-formed, all fields set, connection.json file
			ccdata := `{"address": "ccaddress:12345", "domain": "ccaddress", "tls_required": true, "dial_timeout": "10s", "client_auth_required": true, "client_key": "fake-key", "client_cert": "fake-cert", "root_cert": "fake-root-cert"}`
			err = ioutil.WriteFile(filepath.Join(instance.ChaincodeServerReleaseDir(), "connection.json"), []byte(ccdata), 0o600)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(instance.ReleaseDir)
		})

		It("returns chaincode connection", func() {
			ccinfo, err := instance.ChaincodeServerInfo()
			Expect(err).NotTo(HaveOccurred())
			Expect(ccinfo).To(Equal(&ccintf.ChaincodeServerInfo{
				Address: "ccaddress:12345",
				ClientConfig: comm.ClientConfig{
					SecOpts: comm.SecureOptions{
						UseTLS:             true,
						RequireClientCert:  true,
						Certificate:        []byte("fake-cert"),
						Key:                []byte("fake-key"),
						ServerRootCAs:      [][]byte{[]byte("fake-root-cert")},
						ServerNameOverride: "ccaddress",
					},
					KaOpts:      comm.DefaultKeepaliveOptions,
					DialTimeout: 10 * time.Second,
				},
			}))
		})

		When("connection.json is not provided", func() {
			BeforeEach(func() {
				err := os.Remove(filepath.Join(instance.ChaincodeServerReleaseDir(), "connection.json"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns nil server info", func() {
				ccinfo, err := instance.ChaincodeServerInfo()
				Expect(err).NotTo(HaveOccurred())
				Expect(ccinfo).To(BeNil())
			})
		})

		When("chaincode info is badly formed", func() {
			BeforeEach(func() {
				ccdata := `{"badly formed chaincode"}`
				err := ioutil.WriteFile(filepath.Join(instance.ChaincodeServerReleaseDir(), "connection.json"), []byte(ccdata), 0o600)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a malformed chaincode error", func() {
				_, err := instance.ChaincodeServerInfo()
				Expect(err).To(MatchError(ContainSubstring("malformed chaincode info")))
			})
		})
	})

	Describe("ChaincodeServerUserData", func() {
		var (
			ccuserdata *externalbuilder.ChaincodeServerUserData
			releaseDir string
		)

		BeforeEach(func() {
			var err error
			releaseDir, err = ioutil.TempDir("", "cc-conn-test")
			Expect(err).NotTo(HaveOccurred())

			err = os.MkdirAll(filepath.Join(releaseDir, "chaincode", "server"), 0o755)
			Expect(err).NotTo(HaveOccurred())

			ccuserdata = &externalbuilder.ChaincodeServerUserData{
				Address:            "ccaddress:12345",
				Domain:             "ccaddress",
				DialTimeout:        externalbuilder.Duration(10 * time.Second),
				TLSRequired:        true,
				ClientAuthRequired: true,
				ClientKey:          "fake-key",
				ClientCert:         "fake-cert",
				RootCert:           "fake-root-cert",
			}
		})

		AfterEach(func() {
			os.RemoveAll(releaseDir)
		})

		When("chaincode does not provide all info", func() {
			Context("tls is not provided", func() {
				It("returns TLS without client auth information", func() {
					ccuserdata.TLSRequired = false

					ccinfo, err := ccuserdata.ChaincodeServerInfo(releaseDir)
					Expect(err).NotTo(HaveOccurred())
					Expect(ccinfo).To(Equal(&ccintf.ChaincodeServerInfo{
						Address: "ccaddress:12345",
						ClientConfig: comm.ClientConfig{
							DialTimeout: 10 * time.Second,
							KaOpts:      comm.DefaultKeepaliveOptions,
						},
					}))
				})
			})

			Context("client auth is not provided", func() {
				It("returns TLS without client auth information", func() {
					ccuserdata.ClientAuthRequired = false

					ccinfo, err := ccuserdata.ChaincodeServerInfo(releaseDir)
					Expect(err).NotTo(HaveOccurred())
					Expect(ccinfo).To(Equal(&ccintf.ChaincodeServerInfo{
						Address: "ccaddress:12345",
						ClientConfig: comm.ClientConfig{
							SecOpts: comm.SecureOptions{
								UseTLS:        true,
								ServerRootCAs: [][]byte{[]byte("fake-root-cert")},
							},
							KaOpts:      comm.DefaultKeepaliveOptions,
							DialTimeout: 10 * time.Second,
						},
					}))
				})
			})

			Context("dial timeout not provided", func() {
				It("returns default dial timeout without dialtimeout", func() {
					ccuserdata.DialTimeout = 0

					ccinfo, err := ccuserdata.ChaincodeServerInfo(releaseDir)
					Expect(err).NotTo(HaveOccurred())
					Expect(ccinfo).To(Equal(&ccintf.ChaincodeServerInfo{
						Address: "ccaddress:12345",
						ClientConfig: comm.ClientConfig{
							SecOpts: comm.SecureOptions{
								UseTLS:             true,
								RequireClientCert:  true,
								Certificate:        []byte("fake-cert"),
								Key:                []byte("fake-key"),
								ServerRootCAs:      [][]byte{[]byte("fake-root-cert")},
								ServerNameOverride: "ccaddress",
							},
							KaOpts:      comm.DefaultKeepaliveOptions,
							DialTimeout: 3 * time.Second,
						},
					}))
				})
			})

			Context("address is not provided", func() {
				It("returns missing address error", func() {
					ccuserdata.Address = ""

					_, err := ccuserdata.ChaincodeServerInfo(releaseDir)
					Expect(err).To(MatchError("chaincode address not provided"))
				})
			})

			Context("domain is not provided", func() {
				It("returns domain is not provided", func() {
					ccuserdata.Domain = ""

					ccinfo, err := ccuserdata.ChaincodeServerInfo(releaseDir)
					Expect(err).NotTo(HaveOccurred())
					Expect(ccinfo).To(Equal(&ccintf.ChaincodeServerInfo{
						Address: "ccaddress:12345",
						ClientConfig: comm.ClientConfig{
							SecOpts: comm.SecureOptions{
								UseTLS:             true,
								RequireClientCert:  true,
								Certificate:        []byte("fake-cert"),
								Key:                []byte("fake-key"),
								ServerRootCAs:      [][]byte{[]byte("fake-root-cert")},
								ServerNameOverride: "",
							},
							KaOpts:      comm.DefaultKeepaliveOptions,
							DialTimeout: 10 * time.Second,
						},
					}))
				})
			})

			Context("key is not provided", func() {
				It("returns missing key error", func() {
					ccuserdata.ClientKey = ""

					_, err := ccuserdata.ChaincodeServerInfo(releaseDir)
					Expect(err).To(MatchError("chaincode tls key not provided"))
				})
			})

			Context("cert is not provided", func() {
				It("returns missing key error", func() {
					ccuserdata.ClientCert = ""

					_, err := ccuserdata.ChaincodeServerInfo(releaseDir)
					Expect(err).To(MatchError("chaincode tls cert not provided"))
				})
			})

			Context("root cert is not provided", func() {
				It("returns missing root cert error", func() {
					ccuserdata.RootCert = ""

					_, err := ccuserdata.ChaincodeServerInfo(releaseDir)
					Expect(err).To(MatchError("chaincode tls root cert not provided"))
				})
			})
		})
	})

	Describe("Duration", func() {
		DescribeTable("Unmarshal",
			func(input string, expected externalbuilder.Duration, errMatcher types.GomegaMatcher) {
				var d externalbuilder.Duration
				err := json.Unmarshal([]byte(input), &d)
				Expect(err).To(errMatcher)
			},
			Entry("Number", `100`, externalbuilder.Duration(100), BeNil()),
			Entry("Duration", `"1s"`, externalbuilder.Duration(time.Second), BeNil()),
			Entry("List", `[1, 2, 3]`, externalbuilder.Duration(time.Second), MatchError("invalid duration")),
			Entry("Nonsense", `"nonsense"`, externalbuilder.Duration(time.Second), MatchError(MatchRegexp(`time: invalid duration "?nonsense"?`))),
		)

		DescribeTable("Round Trip",
			func(d time.Duration) {
				marshalled, err := json.Marshal(externalbuilder.Duration(d))
				Expect(err).NotTo(HaveOccurred())

				var unmarshalled externalbuilder.Duration
				err = json.Unmarshal(marshalled, &unmarshalled)
				Expect(err).NotTo(HaveOccurred())

				Expect(unmarshalled).To(Equal(externalbuilder.Duration(d)))
			},
			Entry("10ms", 10*time.Millisecond),
			Entry("10s", 10*time.Second),
			Entry("10m", 10*time.Minute),
			Entry("10h", 10*time.Hour),
		)
	})

	Describe("Start", func() {
		It("invokes the builder's run command and sets the run status", func() {
			err := instance.Start(&ccintf.PeerConnection{
				Address: "fake-peer-address",
				TLSConfig: &ccintf.TLSConfig{
					ClientCert: []byte("fake-client-cert"),
					ClientKey:  []byte("fake-client-key"),
					RootCert:   []byte("fake-root-cert"),
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.Session).NotTo(BeNil())

			errCh := make(chan error)
			go func(sess *externalbuilder.Session) { errCh <- sess.Wait() }(instance.Session)
			Eventually(errCh).Should(Receive(BeNil()))
		})
	})

	Describe("Stop", func() {
		It("terminates the process", func() {
			cmd := exec.Command("sleep", "90")
			sess, err := externalbuilder.Start(logger, cmd)
			Expect(err).NotTo(HaveOccurred())
			instance.Session = sess
			instance.TermTimeout = time.Minute

			errCh := make(chan error)
			go func() { errCh <- sess.Wait() }()
			Consistently(errCh).ShouldNot(Receive())

			err = instance.Stop()
			Expect(err).NotTo(HaveOccurred())
			Eventually(errCh).Should(Receive(MatchError("signal: terminated")))
		})

		Context("when the process doesn't respond to SIGTERM within TermTimeout", func() {
			It("kills the process with malice", func() {
				cmd := exec.Command("testdata/ignoreterm.sh")
				sess, err := externalbuilder.Start(logger, cmd)
				Expect(err).NotTo(HaveOccurred())

				instance.Session = sess
				instance.TermTimeout = time.Second

				errCh := make(chan error)
				go func() { errCh <- sess.Wait() }()
				Consistently(errCh).ShouldNot(Receive())

				err = instance.Stop()
				Expect(err).NotTo(HaveOccurred())
				Eventually(errCh).Should(Receive(MatchError("signal: killed")))
			})
		})

		Context("when the instance session has not been started", func() {
			It("returns an error", func() {
				instance.Session = nil
				err := instance.Stop()
				Expect(err).To(MatchError("instance has not been started"))
			})
		})
	})

	Describe("Wait", func() {
		BeforeEach(func() {
			err := instance.Start(&ccintf.PeerConnection{
				Address: "fake-peer-address",
				TLSConfig: &ccintf.TLSConfig{
					ClientCert: []byte("fake-client-cert"),
					ClientKey:  []byte("fake-client-key"),
					RootCert:   []byte("fake-root-cert"),
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the exit status of the run", func() {
			code, err := instance.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal(0))
		})

		Context("when run exits with a non-zero status", func() {
			BeforeEach(func() {
				instance.Builder.Location = "testdata/failbuilder"
				instance.Builder.Name = "failbuilder"
				err := instance.Start(&ccintf.PeerConnection{
					Address: "fake-peer-address",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the exit status of the run and accompanying error", func() {
				code, err := instance.Wait()
				Expect(err).To(MatchError("builder 'failbuilder' run failed: exit status 1"))
				Expect(code).To(Equal(1))
			})
		})

		Context("when the instance session has not been started", func() {
			It("returns an error", func() {
				instance.Session = nil
				exitCode, err := instance.Wait()
				Expect(err).To(MatchError("instance was not successfully started"))
				Expect(exitCode).To(Equal(-1))
			})
		})
	})
})
