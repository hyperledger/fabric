/*
Copyright Hitachi, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgerutil

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protoutil"
)

// IdentifyTxs - Iterate through transactions in a specified block storage,
// and outputs the relevant data in the transactions, i.e. transaction information
// and write sets, in order to find the transactions that resulted in a potential
// differences in snapshots
func IdentifyTxs(ledgerPath string, channelName string, outputFile string) error {
	conf := blkstorage.NewConf(ledgerPath, 0)
	index := blkstorage.IndexConfig{
		AttrsToIndex: []blkstorage.IndexableAttr{
			blkstorage.IndexableAttrBlockNum,
			blkstorage.IndexableAttrTxID,
		},
	}
	provider, err := blkstorage.NewProvider(conf, &index, &disabled.Provider{})
	if err != nil {
		return err
	}

	store, err := provider.Open(channelName)
	if err != nil {
		return err
	}

	info, err := store.GetBlockchainInfo()
	if err != nil {
		return err
	}

	iterator, err := store.RetrieveBlocks(0)
	if err != nil {
		return err
	}
	defer iterator.Close()

	writer, err := newJSONTransactionInfoFileWriter(outputFile, channelName)
	if err != nil {
		return err
	}
	defer writer.close()

	for i := uint64(0); i < info.Height; i++ {
		result, err := iterator.Next()
		if err != nil {
			return err
		}
		if result == nil {
			break
		}

		if block, ok := result.(*common.Block); ok {
			fmt.Printf("Block Number = %d\n", block.Header.Number)

			err := inspectBlock(block, writer)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func inspectBlock(block *common.Block, writer *jsonTransactionInfoFileWriter) error {
	numEnvelopes := len(block.Data.Data)

	if numEnvelopes > 0 {
		err := writer.startBlock(block.Header.Number)
		if err != nil {
			return err
		}
		defer writer.endBlock()
	}

	for i := 0; i < numEnvelopes; i++ {
		envelope, err := protoutil.ExtractEnvelope(block, i)
		if err != nil {
			return err
		}

		header, err := protoutil.ChannelHeader(envelope)
		if err != nil {
			return err
		}

		err = inspectTransaction(envelope, header, writer)
		if err != nil {
			return err
		}

	}

	return nil
}

type nsWriteRecord struct {
	Namespace string `json:"namespace"`
	IsDelete  bool   `json:"isDelete"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

// inspectTransaction - Inspect a transaction
func inspectTransaction(envelope *common.Envelope, header *common.ChannelHeader, writer *jsonTransactionInfoFileWriter) error {
	// For now, only endorser transactions are inspected and config transactions are ignored
	switch header.Type {
	case int32(common.HeaderType_ENDORSER_TRANSACTION):

		err := writer.startTransaction(header.TxId, timestampToStr(header.Timestamp))
		if err != nil {
			return err
		}
		defer writer.endTransaction()

		transaction := &peer.Transaction{}

		_, err = protoutil.UnmarshalEnvelopeOfType(envelope, common.HeaderType_ENDORSER_TRANSACTION, transaction)
		if err != nil {
			return err
		}

		err = inspectEndorserTransaction(transaction, writer)
		if err != nil {
			return err
		}
	}

	return nil
}

// inspectEndorserTransaction - Inspect the actions in an endorser transaction
func inspectEndorserTransaction(transaction *peer.Transaction, writer *jsonTransactionInfoFileWriter) error {
	for _, action := range transaction.Actions {
		ccActionPayload, _, err := protoutil.GetPayloads(action)
		if err != nil {
			return err
		}

		responsePayload, err := protoutil.UnmarshalProposalResponsePayload(ccActionPayload.Action.ProposalResponsePayload)
		if err != nil {
			return err
		}

		ccAction, err := protoutil.UnmarshalChaincodeAction(responsePayload.Extension)
		if err != nil {
			return err
		}

		rwset := rwsetutil.TxRwSet{}
		err = rwset.FromProtoBytes(ccAction.Results)
		if err != nil {
			return err
		}

		for _, rwset := range rwset.NsRwSets {
			namespace := rwset.NameSpace

			for _, w := range rwset.KvRwSet.Writes {
				record := &nsWriteRecord{
					Namespace: namespace,
					Key:       hex.EncodeToString([]byte(w.Key)),
				}

				// When this Write is deleting the key, the record will not contain the value
				if w.IsDelete {
					record.IsDelete = true
				} else {
					record.IsDelete = false
					record.Value = hex.EncodeToString(w.Value)
				}

				err := writer.writeWriteSet(record)
				if err != nil {
					return err
				}
			}
		}

	}

	return nil
}

// JSON writer for results of the "identifytx" command
type jsonTransactionInfoFileWriter struct {
	file                 *os.File
	buffer               *bufio.Writer
	firstBlockWritten    bool
	firstTxWritten       bool
	firstWriteSetWritten bool
}

func newJSONTransactionInfoFileWriter(filePath string, ledgerId string) (*jsonTransactionInfoFileWriter, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}

	buffer := bufio.NewWriter(file)
	// Output the head portion of the JSON
	openingStr := fmt.Sprintf("{\"ledgerid\":\"%s\",\"blocks\":[\n", ledgerId)
	_, err = buffer.Write([]byte(openingStr))
	if err != nil {
		return nil, err
	}

	firstBlockWritten := false
	firstTxWritten := false
	firstWriteSetWritten := false

	return &jsonTransactionInfoFileWriter{
		file,
		buffer,
		firstBlockWritten,
		firstTxWritten,
		firstWriteSetWritten,
	}, nil
}

// Start writing of information of a new block. Before a call to another startBlock(), endBlock() must be called.
func (w *jsonTransactionInfoFileWriter) startBlock(blockNumber uint64) error {
	if w.firstBlockWritten {
		_, err := w.buffer.Write([]byte(",\n"))
		if err != nil {
			return err
		}
	}

	blockHeader := fmt.Sprintf("{\"number\":%d,\"transactions\":[\n", blockNumber)
	_, err := w.buffer.Write([]byte(blockHeader))

	w.firstTxWritten = false

	return err
}

// End writing of the current block.
func (w *jsonTransactionInfoFileWriter) endBlock() error {
	_, err := w.buffer.Write([]byte("]}"))

	w.firstBlockWritten = true

	return err
}

// Start writing of information of a new transaction in the current block. startBlock() must be called beforehand.
// Before a call to another startTransaction(), endTransaction() must be called.
func (w *jsonTransactionInfoFileWriter) startTransaction(transactionId string, timestampStr string) error {
	if w.firstTxWritten {
		_, err := w.buffer.Write([]byte(",\n"))
		if err != nil {
			return err
		}
	}

	txHeader := fmt.Sprintf("{\"id\":\"%s\",\"timestamp\":\"%s\",\"writes\":[\n", transactionId, timestampStr)
	_, err := w.buffer.Write([]byte(txHeader))

	w.firstWriteSetWritten = false

	return err
}

// End writing of the current transaction information
func (w *jsonTransactionInfoFileWriter) endTransaction() error {
	_, err := w.buffer.Write([]byte("]}"))

	w.firstTxWritten = true

	return err
}

// Write a write set in the current transaction
func (w *jsonTransactionInfoFileWriter) writeWriteSet(record *nsWriteRecord) error {
	if w.firstWriteSetWritten {
		_, err := w.buffer.Write([]byte(",\n"))
		if err != nil {
			return err
		}
	}

	json, _ := json.Marshal(record)
	_, err := w.buffer.Write(json)
	if err != nil {
		return err
	}

	w.firstWriteSetWritten = true

	return nil
}

func (w *jsonTransactionInfoFileWriter) close() error {
	_, err := w.buffer.Write([]byte("\n]}"))
	if err != nil {
		return err
	}

	err = w.buffer.Flush()
	if err != nil {
		return err
	}

	err = w.file.Sync()
	if err != nil {
		return err
	}

	return w.file.Close()
}

// timestampToStr -- convert Timestamp type to string (e.g. "1633074375.833477438")
func timestampToStr(timestamp *(timestamp.Timestamp)) string {
	return fmt.Sprintf("%d.%09d", timestamp.Seconds, timestamp.Nanos)
}
