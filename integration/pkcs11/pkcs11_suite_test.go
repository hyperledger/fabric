/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"encoding/json"
	"testing"

	bpkcs11 "github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/integration/nwo/fabricconfig"
	"github.com/miekg/pkcs11"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

func TestPKCS11(t *testing.T) {
	RegisterFailHandler(Fail)
	lib, pin, label := bpkcs11.FindPKCS11Lib()
	if lib == "" || pin == "" || label == "" {
		t.Skip("Skipping PKCS11 Suite: Required ENV variables not set")
	}
	RunSpecs(t, "PKCS11 Suite")
}

var (
	buildServer *nwo.BuildServer
	components  *nwo.Components
	ctx         *pkcs11.Ctx
	sess        pkcs11.SessionHandle
	bccspConfig *fabricconfig.BCCSP
)

var _ = SynchronizedBeforeSuite(func() []byte {
	buildServer = nwo.NewBuildServer("-tags=pkcs11")
	buildServer.Serve()

	components = buildServer.Components()
	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	setupPKCS11()

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	buildServer.Shutdown()
	ctx.Destroy()
	ctx.CloseSession(sess)
})

func StartPort() int {
	return integration.PKCS11Port.StartPortForNode()
}

func setupPKCS11() {
	lib, pin, label := bpkcs11.FindPKCS11Lib()
	ctx, sess = setupPKCS11Ctx(lib, label, pin)
	bccspConfig = &fabricconfig.BCCSP{
		Default: "PKCS11",
		PKCS11: &fabricconfig.PKCS11{
			Security: 256,
			Hash:     "SHA2",
			Pin:      pin,
			Label:    label,
			Library:  lib,
		},
	}
}

// Creates pkcs11 context and session
func setupPKCS11Ctx(lib, label, pin string) (*pkcs11.Ctx, pkcs11.SessionHandle) {
	ctx := pkcs11.New(lib)

	err := ctx.Initialize()
	Expect(err).NotTo(HaveOccurred())

	slot := findPKCS11Slot(ctx, label)
	Expect(slot).Should(BeNumerically(">", 0), "Could not find slot with label %s", label)

	sess, err := ctx.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
	Expect(err).NotTo(HaveOccurred())

	// Login
	err = ctx.Login(sess, pkcs11.CKU_USER, pin)
	Expect(err).NotTo(HaveOccurred())

	return ctx, sess
}

// Identifies pkcs11 slot using specified label
func findPKCS11Slot(ctx *pkcs11.Ctx, label string) uint {
	slots, err := ctx.GetSlotList(true)
	Expect(err).NotTo(HaveOccurred())

	for _, s := range slots {
		tokInfo, err := ctx.GetTokenInfo(s)
		Expect(err).NotTo(HaveOccurred())

		if tokInfo.Label == label {
			return s
		}
	}

	return 0
}

func runQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("100"))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer0"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say("90"))
}
