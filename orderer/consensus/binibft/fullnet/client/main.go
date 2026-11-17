// BiniBFT end-to-end client: a Fabric Gateway application that drives one
// transaction through Fabric's execute-order-validate (EOV) flow, step by step,
// on a network whose ordering service is BiniBFT.
//
//	go run . -org 1 -from a -to b -amount 10
package main

import (
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/hash"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
)

func main() {
	net := flag.String("net", "..", "fullnet directory (holds crypto-config)")
	org := flag.Int("org", 1, "organisation of the client (1 or 2)")
	user := flag.String("user", "User1", "client identity inside the org")
	channel := flag.String("channel", "mychannel", "channel")
	ccName := flag.String("cc", "simple", "chaincode")
	from := flag.String("from", "a", "account to debit")
	to := flag.String("to", "b", "account to credit")
	amount := flag.String("amount", "10", "amount to move")
	flag.Parse()

	orgDir := filepath.Join(*net, "crypto-config/peerOrganizations", fmt.Sprintf("org%d.example.com", *org))
	mspID := fmt.Sprintf("Org%dMSP", *org)
	peerAddr := fmt.Sprintf("127.0.0.1:%d", 7301+10**org)

	userMSP := filepath.Join(orgDir, "users", fmt.Sprintf("%s@org%d.example.com", *user, *org), "msp")
	certPEM := must(readFirst(filepath.Join(userMSP, "signcerts")))
	keyPEM := must(readFirst(filepath.Join(userMSP, "keystore")))
	cert := must(identity.CertificateFromPEM(certPEM))
	id := must(identity.NewX509Identity(mspID, cert))
	key := must(identity.PrivateKeyFromPEM(keyPEM))
	sign := must(identity.NewPrivateKeySign(key))

	tlsCA := must(os.ReadFile(filepath.Join(orgDir, "peers", fmt.Sprintf("peer0.org%d.example.com", *org), "tls/ca.crt")))
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(tlsCA)
	conn := must(grpc.NewClient(peerAddr, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(pool, ""))))
	defer conn.Close()

	gw := must(client.Connect(id, client.WithSign(sign), client.WithHash(hash.SHA256), client.WithClientConnection(conn),
		client.WithEvaluateTimeout(10*time.Second), client.WithEndorseTimeout(20*time.Second),
		client.WithSubmitTimeout(20*time.Second), client.WithCommitStatusTimeout(60*time.Second)))
	defer gw.Close()
	network := gw.GetNetwork(*channel)
	contract := network.GetContract(*ccName)

	fmt.Printf("client   %s@%s  ->  gateway peer %s, channel %s, chaincode %s\n\n", *user, mspID, peerAddr, *channel, *ccName)

	balances := func() string {
		a := must(contract.EvaluateTransaction("query", *from))
		b := must(contract.EvaluateTransaction("query", *to))
		return fmt.Sprintf("%s=%s %s=%s", *from, a, *to, b)
	}
	fmt.Printf("before   %s   (query = evaluate only, nothing is ordered)\n\n", balances())

	// Subscribe before submitting, so the block carrying the transaction is not missed.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocks := must(network.BlockEvents(ctx))

	// 1. Execute: endorsing peers simulate the transaction and sign the result.
	proposal := must(contract.NewProposal("invoke", client.WithArguments(*from, *to, *amount)))
	fmt.Printf("1 EXECUTE  proposal  tx %s  invoke(%s -> %s, %s)\n", proposal.TransactionID(), *from, *to, *amount)
	tx := must(proposal.Endorse())
	fmt.Printf("           endorsed  by the peers the endorsement policy needs (MAJORITY of Org1, Org2)\n")

	// 2. Order: BiniBFT orders the endorsed transaction into a block.
	commit := must(tx.Submit())
	fmt.Printf("2 ORDER    submitted to the BiniBFT ordering service\n")

	// 3. Validate: peers check the block and commit it.
	status := must(commit.Status())
	fmt.Printf("3 VALIDATE committed  block %d  status %s\n", status.BlockNumber, status.Code)
	if !status.Successful {
		fmt.Println("RESULT: FAIL, transaction was not valid")
		os.Exit(1)
	}

	for blk := range blocks {
		if blk.GetHeader().GetNumber() < status.BlockNumber {
			continue
		}
		md := &common.Metadata{}
		_ = proto.Unmarshal(blk.GetMetadata().GetMetadata()[common.BlockMetadataIndex_SIGNATURES], md)
		fmt.Printf("           block %d  txs %d  orderer signatures %d\n",
			blk.GetHeader().GetNumber(), len(blk.GetData().GetData()), len(md.GetSignatures()))
		break
	}

	fmt.Printf("\nafter    %s\n", balances())
	fmt.Println("RESULT: OK, transaction endorsed, ordered by BiniBFT, validated and committed")
}

func readFirst(dir string) ([]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s is empty", dir)
	}
	return os.ReadFile(filepath.Join(dir, entries[0].Name()))
}

func must[T any](v T, err error) T {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	return v
}
