/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/cmd/osnadmin/mocks"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("osnadmin", func() {
	var (
		tempDir               string
		ordererCACert         string
		clientCert            string
		clientKey             string
		mockChannelManagement *mocks.ChannelManagement
		testServer            *httptest.Server
		tlsConfig             *tls.Config
		ordererURL            string
		channelID             string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "osnadmin")
		Expect(err).NotTo(HaveOccurred())

		generateCertificates(tempDir)

		ordererCACert = filepath.Join(tempDir, "server-ca.pem")
		clientCert = filepath.Join(tempDir, "client-cert.pem")
		clientKey = filepath.Join(tempDir, "client-key.pem")

		channelID = "testing123"

		config := localconfig.ChannelParticipation{
			Enabled:            true,
			MaxRequestBodySize: 1024 * 1024,
		}
		mockChannelManagement = &mocks.ChannelManagement{}

		h := channelparticipation.NewHTTPHandler(config, mockChannelManagement)
		Expect(h).NotTo(BeNil())
		testServer = httptest.NewUnstartedServer(h)

		cert, err := tls.LoadX509KeyPair(
			filepath.Join(tempDir, "server-cert.pem"),
			filepath.Join(tempDir, "server-key.pem"),
		)
		Expect(err).NotTo(HaveOccurred())

		caCertPool := x509.NewCertPool()
		clientCAPem, err := ioutil.ReadFile(filepath.Join(tempDir, "client-ca.pem"))
		Expect(err).NotTo(HaveOccurred())
		caCertPool.AppendCertsFromPEM(clientCAPem)

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientCAs:    caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}
	})

	JustBeforeEach(func() {
		if tlsConfig != nil {
			testServer.TLS = tlsConfig
			testServer.StartTLS()
		} else {
			testServer.Start()
		}

		u, err := url.Parse(testServer.URL)
		Expect(err).NotTo(HaveOccurred())
		ordererURL = u.Host
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
		testServer.Close()
	})

	Describe("List", func() {
		BeforeEach(func() {
			mockChannelManagement.ChannelListReturns(types.ChannelList{
				Channels: []types.ChannelInfoShort{
					{
						Name: "participation-trophy",
					},
					{
						Name: "another-participation-trophy",
					},
				},
				SystemChannel: &types.ChannelInfoShort{
					Name: "fight-the-system",
				},
			})

			mockChannelManagement.ChannelInfoReturns(types.ChannelInfo{
				Name:              "asparagus",
				ConsensusRelation: "broccoli",
				Status:            "carrot",
				Height:            987,
			}, nil)
		})

		It("uses the channel participation API to list all application channels and the system channel (when it exists)", func() {
			args := []string{
				"channel",
				"list",
				"--orderer-address", ordererURL,
				"--ca-file", ordererCACert,
				"--client-cert", clientCert,
				"--client-key", clientKey,
			}
			output, exit, err := executeForArgs(args)
			expectedOutput := types.ChannelList{
				Channels: []types.ChannelInfoShort{
					{
						Name: "participation-trophy",
						URL:  "/participation/v1/channels/participation-trophy",
					},
					{
						Name: "another-participation-trophy",
						URL:  "/participation/v1/channels/another-participation-trophy",
					},
				},
				SystemChannel: &types.ChannelInfoShort{
					Name: "fight-the-system",
					URL:  "/participation/v1/channels/fight-the-system",
				},
			}
			checkStatusOutput(output, exit, err, 200, expectedOutput)
		})

		It("uses the channel participation API to list the details of a single channel", func() {
			args := []string{
				"channel",
				"list",
				"--orderer-address", ordererURL,
				"--channelID", "tell-me-your-secrets",
				"--ca-file", ordererCACert,
				"--client-cert", clientCert,
				"--client-key", clientKey,
			}
			output, exit, err := executeForArgs(args)
			expectedOutput := types.ChannelInfo{
				Name:              "asparagus",
				URL:               "/participation/v1/channels/asparagus",
				ConsensusRelation: "broccoli",
				Status:            "carrot",
				Height:            987,
			}
			checkStatusOutput(output, exit, err, 200, expectedOutput)
		})

		Context("when the channel does not exist", func() {
			BeforeEach(func() {
				mockChannelManagement.ChannelInfoReturns(types.ChannelInfo{}, errors.New("eat-your-peas"))
			})

			It("returns 404 not found", func() {
				args := []string{
					"channel",
					"list",
					"--orderer-address", ordererURL,
					"--channelID", "tell-me-your-secrets",
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)
				expectedOutput := types.ErrorResponse{
					Error: "eat-your-peas",
				}
				checkStatusOutput(output, exit, err, 404, expectedOutput)
			})
		})

		Context("when TLS is disabled", func() {
			BeforeEach(func() {
				tlsConfig = nil
			})

			It("uses the channel participation API to list all channels", func() {
				args := []string{
					"channel",
					"list",
					"--orderer-address", ordererURL,
				}
				output, exit, err := executeForArgs(args)
				Expect(err).NotTo(HaveOccurred())
				Expect(exit).To(Equal(0))

				expectedOutput := types.ChannelList{
					Channels: []types.ChannelInfoShort{
						{
							Name: "participation-trophy",
							URL:  "/participation/v1/channels/participation-trophy",
						},
						{
							Name: "another-participation-trophy",
							URL:  "/participation/v1/channels/another-participation-trophy",
						},
					},
					SystemChannel: &types.ChannelInfoShort{
						Name: "fight-the-system",
						URL:  "/participation/v1/channels/fight-the-system",
					},
				}
				checkStatusOutput(output, exit, err, 200, expectedOutput)
			})

			It("uses the channel participation API to list the details of a single channel", func() {
				args := []string{
					"channel",
					"list",
					"--orderer-address", ordererURL,
					"--channelID", "tell-me-your-secrets",
				}
				output, exit, err := executeForArgs(args)
				Expect(err).NotTo(HaveOccurred())
				Expect(exit).To(Equal(0))

				expectedOutput := types.ChannelInfo{
					Name:              "asparagus",
					URL:               "/participation/v1/channels/asparagus",
					ConsensusRelation: "broccoli",
					Status:            "carrot",
					Height:            987,
				}
				checkStatusOutput(output, exit, err, 200, expectedOutput)
			})
		})
	})

	Describe("Remove", func() {
		It("uses the channel participation API to remove a channel", func() {
			args := []string{
				"channel",
				"remove",
				"--orderer-address", ordererURL,
				"--channelID", channelID,
				"--ca-file", ordererCACert,
				"--client-cert", clientCert,
				"--client-key", clientKey,
			}
			output, exit, err := executeForArgs(args)
			Expect(err).NotTo(HaveOccurred())
			Expect(exit).To(Equal(0))
			Expect(output).To(Equal("Status: 204\n"))
		})

		It("uses the channel participation API to remove a channel (without status)", func() {
			args := []string{
				"channel",
				"remove",
				"--orderer-address", ordererURL,
				"--channelID", channelID,
				"--ca-file", ordererCACert,
				"--client-cert", clientCert,
				"--client-key", clientKey,
				"--no-status",
			}
			output, exit, err := executeForArgs(args)
			Expect(err).NotTo(HaveOccurred())
			Expect(exit).To(Equal(0))
			Expect(output).To(BeEmpty())
		})

		Context("when the channel does not exist", func() {
			BeforeEach(func() {
				mockChannelManagement.RemoveChannelReturns(types.ErrChannelNotExist)
			})

			It("returns 404 not found", func() {
				args := []string{
					"channel",
					"remove",
					"--ca-file", ordererCACert,
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)
				expectedOutput := types.ErrorResponse{
					Error: "cannot remove: channel does not exist",
				}
				checkStatusOutput(output, exit, err, 404, expectedOutput)
			})
		})

		Context("when TLS is disabled", func() {
			BeforeEach(func() {
				tlsConfig = nil
			})

			It("uses the channel participation API to remove a channel", func() {
				args := []string{
					"channel",
					"remove",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
				}
				output, exit, err := executeForArgs(args)
				Expect(err).NotTo(HaveOccurred())
				Expect(exit).To(Equal(0))
				Expect(output).To(Equal("Status: 204\n"))
			})
		})
	})

	Describe("Join", func() {
		var blockPath string

		BeforeEach(func() {
			configBlock := blockWithGroups(
				map[string]*cb.ConfigGroup{
					"Application": {},
				},
				"testing123",
			)
			blockPath = createBlockFile(tempDir, configBlock)

			mockChannelManagement.JoinChannelReturns(types.ChannelInfo{
				Name:              "apple",
				ConsensusRelation: "banana",
				Status:            "orange",
				Height:            123,
			}, nil)
		})

		It("uses the channel participation API to join a channel", func() {
			args := []string{
				"channel",
				"join",
				"--orderer-address", ordererURL,
				"--channelID", channelID,
				"--config-block", blockPath,
				"--ca-file", ordererCACert,
				"--client-cert", clientCert,
				"--client-key", clientKey,
			}
			output, exit, err := executeForArgs(args)
			expectedOutput := types.ChannelInfo{
				Name:              "apple",
				URL:               "/participation/v1/channels/apple",
				ConsensusRelation: "banana",
				Status:            "orange",
				Height:            123,
			}
			checkStatusOutput(output, exit, err, 201, expectedOutput)
		})

		Context("when the block is empty", func() {
			BeforeEach(func() {
				blockPath = createBlockFile(tempDir, &cb.Block{})
			})

			It("returns with exit code 1 and prints the error", func() {
				args := []string{
					"channel",
					"join",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--config-block", blockPath,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)

				checkFlagError(output, exit, err, "failed to retrieve channel id - block is empty")
			})
		})

		Context("when the --channelID does not match the channel ID in the block", func() {
			BeforeEach(func() {
				channelID = "not-the-channel-youre-looking-for"
			})

			It("returns with exit code 1 and prints the error", func() {
				args := []string{
					"channel",
					"join",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--config-block", blockPath,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)

				checkFlagError(output, exit, err, "specified --channelID not-the-channel-youre-looking-for does not match channel ID testing123 in config block")
			})
		})

		Context("when the block isn't a valid config block", func() {
			BeforeEach(func() {
				block := &cb.Block{
					Data: &cb.BlockData{
						Data: [][]byte{
							protoutil.MarshalOrPanic(&cb.Envelope{
								Payload: protoutil.MarshalOrPanic(&cb.Payload{
									Header: &cb.Header{
										ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
											Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
											ChannelId: channelID,
										}),
									},
								}),
							}),
						},
					},
				}
				blockPath = createBlockFile(tempDir, block)
			})

			It("returns 405 bad request", func() {
				args := []string{
					"channel",
					"join",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--config-block", blockPath,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)
				Expect(err).NotTo(HaveOccurred())
				Expect(exit).To(Equal(0))

				expectedOutput := types.ErrorResponse{
					Error: "invalid join block: block is not a config block",
				}
				checkStatusOutput(output, exit, err, 400, expectedOutput)
			})
		})

		Context("when joining the channel fails", func() {
			BeforeEach(func() {
				mockChannelManagement.JoinChannelReturns(types.ChannelInfo{}, types.ErrChannelAlreadyExists)
			})

			It("returns 405 not allowed", func() {
				args := []string{
					"channel",
					"join",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--config-block", blockPath,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)
				expectedOutput := types.ErrorResponse{
					Error: "cannot join: channel already exists",
				}
				checkStatusOutput(output, exit, err, 405, expectedOutput)
			})

			It("returns 405 not allowed (without status)", func() {
				args := []string{
					"channel",
					"join",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--config-block", blockPath,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
					"--no-status",
				}
				output, exit, err := executeForArgs(args)
				expectedOutput := types.ErrorResponse{
					Error: "cannot join: channel already exists",
				}
				checkOutput(output, exit, err, expectedOutput)
			})
		})

		Context("when TLS is disabled", func() {
			BeforeEach(func() {
				tlsConfig = nil
			})

			It("uses the channel participation API to join a channel", func() {
				args := []string{
					"channel",
					"join",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--config-block", blockPath,
				}
				output, exit, err := executeForArgs(args)
				expectedOutput := types.ChannelInfo{
					Name:              "apple",
					URL:               "/participation/v1/channels/apple",
					ConsensusRelation: "banana",
					Status:            "orange",
					Height:            123,
				}
				checkStatusOutput(output, exit, err, 201, expectedOutput)
			})
		})
	})

	Describe("Flags", func() {
		It("accepts short versions of the --orderer-address, --channelID, and --config-block flags", func() {
			configBlock := blockWithGroups(
				map[string]*cb.ConfigGroup{
					"Application": {},
				},
				"testing123",
			)
			blockPath := createBlockFile(tempDir, configBlock)
			mockChannelManagement.JoinChannelReturns(types.ChannelInfo{
				Name:              "apple",
				ConsensusRelation: "banana",
				Status:            "orange",
				Height:            123,
			}, nil)

			args := []string{
				"channel",
				"join",
				"-o", ordererURL,
				"-c", channelID,
				"-b", blockPath,
				"--ca-file", ordererCACert,
				"--client-cert", clientCert,
				"--client-key", clientKey,
			}
			output, exit, err := executeForArgs(args)
			expectedOutput := types.ChannelInfo{
				Name:              "apple",
				URL:               "/participation/v1/channels/apple",
				ConsensusRelation: "banana",
				Status:            "orange",
				Height:            123,
			}
			checkStatusOutput(output, exit, err, 201, expectedOutput)
		})

		Context("when an unknown flag is used", func() {
			It("returns an error for long flags", func() {
				_, _, err := executeForArgs([]string{"channel", "list", "--bad-flag"})
				Expect(err).To(MatchError("unknown long flag '--bad-flag'"))
			})

			It("returns an error for short flags", func() {
				_, _, err := executeForArgs([]string{"channel", "list", "-z"})
				Expect(err).To(MatchError("unknown short flag '-z'"))
			})
		})

		Context("when the ca cert cannot be read", func() {
			BeforeEach(func() {
				ordererCACert = "not-the-ca-cert-youre-looking-for"
			})

			It("returns with exit code 1 and prints the error", func() {
				args := []string{
					"channel",
					"list",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)
				checkFlagError(output, exit, err, "reading orderer CA certificate: open not-the-ca-cert-youre-looking-for: no such file or directory")
			})
		})

		Context("when the ca-file contains a private key instead of certificate(s)", func() {
			BeforeEach(func() {
				ordererCACert = clientKey
			})

			It("returns with exit code 1 and prints the error", func() {
				args := []string{
					"channel",
					"remove",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)
				checkFlagError(output, exit, err, "failed to add ca-file PEM to cert pool")
			})
		})

		Context("when the client cert/key pair fail to load", func() {
			BeforeEach(func() {
				clientKey = "brussel-sprouts"
			})

			It("returns with exit code 1 and prints the error", func() {
				args := []string{
					"channel",
					"list",
					"--orderer-address", ordererURL,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)
				checkFlagError(output, exit, err, "loading client cert/key pair: open brussel-sprouts: no such file or directory")
			})
		})

		Context("when the config block cannot be read", func() {
			var configBlockPath string

			BeforeEach(func() {
				configBlockPath = "not-the-config-block-youre-looking-for"
			})

			It("returns with exit code 1 and prints the error", func() {
				args := []string{
					"channel",
					"join",
					"--orderer-address", ordererURL,
					"--channelID", channelID,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
					"--config-block", configBlockPath,
				}
				output, exit, err := executeForArgs(args)
				checkFlagError(output, exit, err, "reading config block: open not-the-config-block-youre-looking-for: no such file or directory")
			})
		})
	})

	Describe("Server using intermediate CA", func() {
		BeforeEach(func() {
			cert, err := tls.LoadX509KeyPair(
				filepath.Join(tempDir, "server-intermediate-cert.pem"),
				filepath.Join(tempDir, "server-intermediate-key.pem"),
			)
			Expect(err).NotTo(HaveOccurred())
			tlsConfig.Certificates = []tls.Certificate{cert}

			ordererCACert = filepath.Join(tempDir, "server-ca+intermediate-ca.pem")
		})

		It("uses the channel participation API to list all application and the system channel (when it exists)", func() {
			args := []string{
				"channel",
				"list",
				"--orderer-address", ordererURL,
				"--ca-file", ordererCACert,
				"--client-cert", clientCert,
				"--client-key", clientKey,
			}
			output, exit, err := executeForArgs(args)
			expectedOutput := types.ChannelList{
				Channels:      nil,
				SystemChannel: nil,
			}
			checkStatusOutput(output, exit, err, 200, expectedOutput)
		})

		Context("when the ca-file does not include the intermediate CA", func() {
			BeforeEach(func() {
				ordererCACert = filepath.Join(tempDir, "server-ca.pem")
			})
			It("returns with exit code 1 and prints the error", func() {
				args := []string{
					"channel",
					"list",
					"--orderer-address", ordererURL,
					"--ca-file", ordererCACert,
					"--client-cert", clientCert,
					"--client-key", clientKey,
				}
				output, exit, err := executeForArgs(args)
				checkCLIError(output, exit, err, fmt.Sprintf("Get \"%s/participation/v1/channels\": tls: failed to verify certificate: x509: certificate signed by unknown authority", testServer.URL))
			})
		})
	})
})

func checkStatusOutput(output string, exit int, err error, expectedStatus int, expectedOutput interface{}) {
	Expect(err).NotTo(HaveOccurred())
	Expect(exit).To(Equal(0))
	json, err := json.MarshalIndent(expectedOutput, "", "\t")
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(fmt.Sprintf("Status: %d\n%s\n", expectedStatus, string(json))))
}

func checkOutput(output string, exit int, err error, expectedOutput interface{}) {
	Expect(err).NotTo(HaveOccurred())
	Expect(exit).To(Equal(0))
	json, err := json.MarshalIndent(expectedOutput, "", "\t")
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(string(json) + "\n"))
}

func checkFlagError(output string, exit int, err error, expectedError string) {
	Expect(err).To(MatchError(ContainSubstring(expectedError)))
	Expect(exit).To(Equal(1))
	Expect(output).To(BeEmpty())
}

func checkCLIError(output string, exit int, err error, expectedError string) {
	Expect(err).NotTo(HaveOccurred())
	Expect(exit).To(Equal(1))
	Expect(output).To(Equal(fmt.Sprintf("Error: %s\n", expectedError)))
}

func generateCertificates(tempDir string) {
	serverCA, err := tlsgen.NewCA()
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-ca.pem"), serverCA.CertBytes(), 0o640)
	Expect(err).NotTo(HaveOccurred())
	serverKeyPair, err := serverCA.NewServerCertKeyPair("127.0.0.1")
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-cert.pem"), serverKeyPair.Cert, 0o640)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-key.pem"), serverKeyPair.Key, 0o640)
	Expect(err).NotTo(HaveOccurred())

	serverIntermediateCA, err := serverCA.NewIntermediateCA()
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-intermediate-ca.pem"), serverIntermediateCA.CertBytes(), 0o640)
	Expect(err).NotTo(HaveOccurred())
	serverIntermediateKeyPair, err := serverIntermediateCA.NewServerCertKeyPair("127.0.0.1")
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-intermediate-cert.pem"), serverIntermediateKeyPair.Cert, 0o640)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-intermediate-key.pem"), serverIntermediateKeyPair.Key, 0o640)
	Expect(err).NotTo(HaveOccurred())

	serverAndIntermediateCABytes := append(serverCA.CertBytes(), serverIntermediateCA.CertBytes()...)
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-ca+intermediate-ca.pem"), serverAndIntermediateCABytes, 0o640)
	Expect(err).NotTo(HaveOccurred())

	clientCA, err := tlsgen.NewCA()
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "client-ca.pem"), clientCA.CertBytes(), 0o640)
	Expect(err).NotTo(HaveOccurred())
	clientKeyPair, err := clientCA.NewClientCertKeyPair()
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "client-cert.pem"), clientKeyPair.Cert, 0o640)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "client-key.pem"), clientKeyPair.Key, 0o640)
	Expect(err).NotTo(HaveOccurred())
}

func blockWithGroups(groups map[string]*cb.ConfigGroup, channelID string) *cb.Block {
	return &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{
				protoutil.MarshalOrPanic(&cb.Envelope{
					Payload: protoutil.MarshalOrPanic(&cb.Payload{
						Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
							Config: &cb.Config{
								ChannelGroup: &cb.ConfigGroup{
									Groups: groups,
									Values: map[string]*cb.ConfigValue{
										"HashingAlgorithm": {
											Value: protoutil.MarshalOrPanic(&cb.HashingAlgorithm{
												Name: bccsp.SHA256,
											}),
										},
										"BlockDataHashingStructure": {
											Value: protoutil.MarshalOrPanic(&cb.BlockDataHashingStructure{
												Width: math.MaxUint32,
											}),
										},
										"OrdererAddresses": {
											Value: protoutil.MarshalOrPanic(&cb.OrdererAddresses{
												Addresses: []string{"localhost"},
											}),
										},
									},
								},
							},
						}),
						Header: &cb.Header{
							ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
								Type:      int32(cb.HeaderType_CONFIG),
								ChannelId: channelID,
							}),
						},
					}),
				}),
			},
		},
	}
}

func createBlockFile(tempDir string, configBlock *cb.Block) string {
	blockBytes, err := proto.Marshal(configBlock)
	Expect(err).NotTo(HaveOccurred())
	blockPath := filepath.Join(tempDir, "block.pb")
	err = ioutil.WriteFile(blockPath, blockBytes, 0o644)
	Expect(err).NotTo(HaveOccurred())
	return blockPath
}
