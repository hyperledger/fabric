/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// testclient is a small end-to-end test tool for the BiniBFT orderer.
//
//	testclient broadcast -server 127.0.0.1:7050 -channel ch -ca ca.crt -mspdir <Admin msp> -mspid OrdererMSP -n 10
//	testclient verify    -server 127.0.0.1:7050 -channel ch -ca ca.crt -mspdir <Admin msp> -mspid OrdererMSP \
//	                     -genesis genesis.block -to 10
//
// `verify` downloads blocks with the Deliver API and checks every one with
// Fabric's own BFT block verifier (common/deliverclient), i.e. the exact code
// a peer or an orderer follower uses to accept blocks from a BFT ordering
// service. It prints the header hash of each block so that several orderers can
// be compared.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliverclient"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type options struct {
	server, channel, caFile, mspDir, mspID, genesis string
	n, from, to                                     int
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: testclient broadcast|verify [flags]")
	}
	cmd := os.Args[1]

	var o options
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.StringVar(&o.server, "server", "127.0.0.1:7050", "orderer address")
	fs.StringVar(&o.channel, "channel", "mychannel", "channel ID")
	fs.StringVar(&o.caFile, "ca", "", "TLS CA certificate of the orderer (PEM)")
	fs.StringVar(&o.mspDir, "mspdir", "", "local MSP directory of the signing identity")
	fs.StringVar(&o.mspID, "mspid", "OrdererMSP", "MSP ID of the signing identity")
	fs.StringVar(&o.genesis, "genesis", "", "genesis (config) block file, for verify")
	fs.IntVar(&o.n, "n", 1, "number of transactions, for broadcast")
	fs.IntVar(&o.from, "from", 1, "first block to verify")
	fs.IntVar(&o.to, "to", 1, "last block to verify")
	_ = fs.Parse(os.Args[2:])

	signer := loadSigner(o)
	conn := dial(o)
	defer conn.Close()

	switch cmd {
	case "broadcast":
		broadcast(conn, signer, o)
	case "verify":
		verify(conn, signer, o)
	default:
		fatalf("unknown command %q", cmd)
	}
}

func loadSigner(o options) identity.SignerSerializer {
	cfg, err := msp.GetLocalMspConfig(o.mspDir, nil, o.mspID)
	if err != nil {
		fatalf("loading MSP config: %v", err)
	}
	if err := mspmgmt.GetLocalMSP(factory.GetDefault()).Setup(cfg); err != nil {
		fatalf("setting up the local MSP: %v", err)
	}
	signer, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	if err != nil {
		fatalf("loading the signing identity: %v", err)
	}
	return signer
}

func dial(o options) *grpc.ClientConn {
	pem, err := os.ReadFile(o.caFile)
	if err != nil {
		fatalf("reading CA file: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		fatalf("no certificate in %s", o.caFile)
	}
	creds := credentials.NewTLS(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	conn, err := grpc.NewClient(o.server, grpc.WithTransportCredentials(creds))
	if err != nil {
		fatalf("connecting to %s: %v", o.server, err)
	}
	return conn
}

func broadcast(conn *grpc.ClientConn, signer identity.SignerSerializer, o options) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	stream, err := ab.NewAtomicBroadcastClient(conn).Broadcast(ctx)
	if err != nil {
		fatalf("opening the broadcast stream: %v", err)
	}

	for i := 0; i < o.n; i++ {
		payload := &cb.ConfigValue{Value: []byte(fmt.Sprintf("binibft-tx-%d-%d", time.Now().UnixNano(), i))}
		env, err := protoutil.CreateSignedEnvelope(cb.HeaderType_ENDORSER_TRANSACTION, o.channel, signer, payload, 0, 0)
		if err != nil {
			fatalf("creating the envelope: %v", err)
		}
		if err := stream.Send(env); err != nil {
			fatalf("sending tx %d: %v", i, err)
		}
		resp, err := stream.Recv()
		if err != nil {
			fatalf("receiving the ack of tx %d: %v", i, err)
		}
		if resp.Status != cb.Status_SUCCESS {
			fatalf("tx %d rejected: %v %s", i, resp.Status, resp.Info)
		}
	}
	_ = stream.CloseSend()
	fmt.Printf("broadcast OK: %d transactions accepted by %s\n", o.n, o.server)
}

func verify(conn *grpc.ClientConn, signer identity.SignerSerializer, o options) {
	raw, err := os.ReadFile(o.genesis)
	if err != nil {
		fatalf("reading the genesis block: %v", err)
	}
	genesis, err := protoutil.UnmarshalBlock(raw)
	if err != nil {
		fatalf("parsing the genesis block: %v", err)
	}
	verifier, err := deliverclient.NewBlockVerificationAssistant(genesis, genesis, factory.GetDefault(), flogging.MustGetLogger("testclient"))
	if err != nil {
		fatalf("creating the block verifier: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	stream, err := ab.NewAtomicBroadcastClient(conn).Deliver(ctx)
	if err != nil {
		fatalf("opening the deliver stream: %v", err)
	}
	seek := func(n uint64) *ab.SeekPosition {
		return &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: n}}}
	}
	env, err := protoutil.CreateSignedEnvelope(cb.HeaderType_DELIVER_SEEK_INFO, o.channel, signer, &ab.SeekInfo{
		Start:    seek(uint64(o.from)),
		Stop:     seek(uint64(o.to)),
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	}, 0, 0)
	if err != nil {
		fatalf("creating the seek request: %v", err)
	}
	if err := stream.Send(env); err != nil {
		fatalf("sending the seek request: %v", err)
	}

	expected := uint64(o.from)
	for expected <= uint64(o.to) {
		resp, err := stream.Recv()
		if err != nil {
			fatalf("receiving block %d: %v", expected, err)
		}
		switch t := resp.Type.(type) {
		case *ab.DeliverResponse_Status:
			fatalf("deliver ended with status %v before block %d", t.Status, expected)
		case *ab.DeliverResponse_Block:
			block := t.Block
			if block.Header.Number != expected {
				fatalf("got block %d, expected %d", block.Header.Number, expected)
			}
			if err := verifier.VerifyBlock(block); err != nil {
				fatalf("BLOCK %d FAILED BFT VERIFICATION: %v", expected, err)
			}
			md, err := protoutil.GetMetadataFromBlock(block, cb.BlockMetadataIndex_SIGNATURES)
			if err != nil {
				fatalf("block %d has no signatures metadata: %v", expected, err)
			}
			fmt.Printf("block %d  verified  signatures=%d  txs=%d  hash=%s\n", expected,
				len(md.Signatures), len(block.Data.Data), hex.EncodeToString(protoutil.BlockHeaderHash(block.Header)))
			expected++
		}
	}
	fmt.Printf("verify OK: blocks %d..%d from %s pass Fabric's BFT block verification\n", o.from, o.to, o.server)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}
