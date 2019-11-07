/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"fmt"
	"math"
	"sync"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/pkg/errors"
)

type committer struct {
	db             *couchdb.CouchDatabase
	batchUpdateMap map[string]*batchableDocument
}

// buildCommitters builds committers per namespace. Each committer transforms the
// given batch in the form of underlying db and keep it in memory.
func (vdb *VersionedDB) buildCommitters(updates *statedb.UpdateBatch) ([]*committer, error) {
	namespaces := updates.GetUpdatedNamespaces()

	// for each namespace, we build multiple committers (based on maxBatchSize per namespace)
	var wg sync.WaitGroup
	nsCommittersChan := make(chan []*committer, len(namespaces))
	defer close(nsCommittersChan)
	errsChan := make(chan error, len(namespaces))
	defer close(errsChan)

	// for each namespace, we build committers in parallel. This is because,
	// the committer building process requires fetching of missing revisions
	// that in turn, we want to do in parallel
	for _, ns := range namespaces {
		nsUpdates := updates.GetUpdates(ns)
		wg.Add(1)
		go func(ns string) {
			defer wg.Done()
			committers, err := vdb.buildCommittersForNs(ns, nsUpdates)
			if err != nil {
				errsChan <- err
				return
			}
			nsCommittersChan <- committers
		}(ns)
	}
	wg.Wait()

	// collect all committers
	var allCommitters []*committer
	select {
	case err := <-errsChan:
		return nil, err
	default:
		for i := 0; i < len(namespaces); i++ {
			allCommitters = append(allCommitters, <-nsCommittersChan...)
		}
	}

	return allCommitters, nil
}

func (vdb *VersionedDB) buildCommittersForNs(ns string, nsUpdates map[string]*statedb.VersionedValue) ([]*committer, error) {
	db, err := vdb.getNamespaceDBHandle(ns)
	if err != nil {
		return nil, err
	}

	// for each namespace, build mutiple committers based on the maxBatchSize
	maxBatchSize := db.CouchInstance.MaxBatchUpdateSize()
	numCommitters := 1
	if maxBatchSize > 0 {
		numCommitters = int(math.Ceil(float64(len(nsUpdates)) / float64(maxBatchSize)))
	}
	committers := make([]*committer, numCommitters)
	for i := 0; i < numCommitters; i++ {
		committers[i] = &committer{
			db:             db,
			batchUpdateMap: make(map[string]*batchableDocument),
		}
	}

	// for each committer, create a couchDoc per key-value pair present in the update batch
	// which are associated with the committer's namespace.
	revisions, err := vdb.getRevisions(ns, nsUpdates)
	if err != nil {
		return nil, err
	}

	i := 0
	for key, vv := range nsUpdates {
		couchDoc, err := keyValToCouchDoc(&keyValue{key: key, VersionedValue: vv}, revisions[key])
		if err != nil {
			return nil, err
		}
		committers[i].batchUpdateMap[key] = &batchableDocument{CouchDoc: *couchDoc, Deleted: vv.Value == nil}
		if maxBatchSize > 0 && len(committers[i].batchUpdateMap) == maxBatchSize {
			i++
		}
	}
	return committers, nil
}

func (vdb *VersionedDB) getRevisions(ns string, nsUpdates map[string]*statedb.VersionedValue) (map[string]string, error) {
	// for now, getRevisions does not use cache. In FAB-15616, we will ensure that the getRevisions uses
	// the cache which would be introduced in FAB-15537
	revisions := make(map[string]string)
	nsRevs := vdb.committedDataCache.revs[ns]

	var missingKeys []string
	var ok bool
	for key := range nsUpdates {
		if revisions[key], ok = nsRevs[key]; !ok {
			missingKeys = append(missingKeys, key)
		}
	}

	db, err := vdb.getNamespaceDBHandle(ns)
	if err != nil {
		return nil, err
	}

	logger.Debugf("Pulling revisions for the [%d] keys for namsespace [%s] that were not part of the readset", len(missingKeys), db.DBName)
	retrievedMetadata, err := retrieveNsMetadata(db, missingKeys)
	if err != nil {
		return nil, err
	}
	for _, metadata := range retrievedMetadata {
		revisions[metadata.ID] = metadata.Rev
	}

	return revisions, nil
}

func (vdb *VersionedDB) executeCommitter(committers []*committer) error {
	errsChan := make(chan error, len(committers))
	defer close(errsChan)
	var wg sync.WaitGroup
	wg.Add(len(committers))

	for _, c := range committers {
		go func(c *committer) {
			defer wg.Done()
			if err := c.commitUpdates(); err != nil {
				errsChan <- err
			}
		}(c)
	}
	wg.Wait()

	select {
	case err := <-errsChan:
		return err
	default:
		return nil
	}
}

// commitUpdates commits the given updates to couchdb
func (c *committer) commitUpdates() error {
	docs := []*couchdb.CouchDoc{}
	for _, update := range c.batchUpdateMap {
		docs = append(docs, &update.CouchDoc)
	}

	// Do the bulk update into couchdb. Note that this will do retries if the entire bulk update fails or times out
	responses, err := c.db.BatchUpdateDocuments(docs)
	if err != nil {
		return err
	}
	// IF INDIVIDUAL DOCUMENTS IN THE BULK UPDATE DID NOT SUCCEED, TRY THEM INDIVIDUALLY
	// iterate through the response from CouchDB by document
	for _, resp := range responses {
		// If the document returned an error, retry the individual document
		if resp.Ok == true {
			continue
		}
		doc := c.batchUpdateMap[resp.ID]
		var err error
		//Remove the "_rev" from the JSON before saving
		//this will allow the CouchDB retry logic to retry revisions without encountering
		//a mismatch between the "If-Match" and the "_rev" tag in the JSON
		if doc.CouchDoc.JSONValue != nil {
			err = removeJSONRevision(&doc.CouchDoc.JSONValue)
			if err != nil {
				return err
			}
		}
		// Check to see if the document was added to the batch as a delete type document
		if doc.Deleted {
			logger.Warningf("CouchDB batch document delete encountered an problem. Retrying delete for document ID:%s", resp.ID)
			// If this is a deleted document, then retry the delete
			// If the delete fails due to a document not being found (404 error),
			// the document has already been deleted and the DeleteDoc will not return an error
			err = c.db.DeleteDoc(resp.ID, "")
		} else {
			logger.Warningf("CouchDB batch document update encountered an problem. Retrying update for document ID:%s", resp.ID)
			// Save the individual document to couchdb
			// Note that this will do retries as needed
			_, err = c.db.SaveDoc(resp.ID, "", &doc.CouchDoc)
		}

		// If the single document update or delete returns an error, then throw the error
		if err != nil {
			errorString := fmt.Sprintf("error saving document ID: %v. Error: %s,  Reason: %s",
				resp.ID, resp.Error, resp.Reason)

			logger.Errorf(errorString)
			return errors.WithMessage(err, errorString)
		}
	}
	return nil
}

//batchableDocument defines a document for a batch
type batchableDocument struct {
	CouchDoc couchdb.CouchDoc
	Deleted  bool
}
