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
	"github.com/pkg/errors"
)

type committer struct {
	db             *couchDatabase
	batchUpdateMap map[string]*batchableDocument
	namespace      string
	cacheKVs       cacheKVs
	cacheEnabled   bool
}

func (c *committer) addToCacheUpdate(kv *keyValue) {
	if !c.cacheEnabled {
		return
	}

	if kv.Value == nil {
		// nil value denotes a delete operation
		c.cacheKVs[kv.key] = nil
		return
	}

	c.cacheKVs[kv.key] = &CacheValue{
		Version:        kv.Version.ToBytes(),
		Value:          kv.Value,
		Metadata:       kv.Metadata,
		AdditionalInfo: []byte(kv.revision),
	}
}

func (c *committer) updateRevisionInCacheUpdate(key, rev string) {
	if !c.cacheEnabled {
		return
	}
	cv := c.cacheKVs[key]
	if cv == nil {
		// nil value denotes a delete
		return
	}
	cv.AdditionalInfo = []byte(rev)
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
		return nil, errors.WithStack(err)
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
	maxBatchSize := db.couchInstance.maxBatchUpdateSize()
	numCommitters := 1
	if maxBatchSize > 0 {
		numCommitters = int(math.Ceil(float64(len(nsUpdates)) / float64(maxBatchSize)))
	}
	committers := make([]*committer, numCommitters)

	cacheEnabled := vdb.cache.enabled(ns)

	for i := 0; i < numCommitters; i++ {
		committers[i] = &committer{
			db:             db,
			batchUpdateMap: make(map[string]*batchableDocument),
			namespace:      ns,
			cacheKVs:       make(cacheKVs),
			cacheEnabled:   cacheEnabled,
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
		kv := &keyValue{key: key, revision: revisions[key], VersionedValue: vv}
		couchDoc, err := keyValToCouchDoc(kv)
		if err != nil {
			return nil, err
		}
		committers[i].batchUpdateMap[key] = &batchableDocument{CouchDoc: *couchDoc, Deleted: vv.Value == nil}
		committers[i].addToCacheUpdate(kv)
		if maxBatchSize > 0 && len(committers[i].batchUpdateMap) == maxBatchSize {
			i++
		}
	}
	return committers, nil
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
		return errors.WithStack(err)
	default:
		return nil
	}
}

// commitUpdates commits the given updates to couchdb
func (c *committer) commitUpdates() error {
	docs := []*couchDoc{}
	for _, update := range c.batchUpdateMap {
		docs = append(docs, &update.CouchDoc)
	}

	// Do the bulk update into couchdb. Note that this will do retries if the entire bulk update fails or times out
	responses, err := c.db.batchUpdateDocuments(docs)
	if err != nil {
		return err
	}

	// IF INDIVIDUAL DOCUMENTS IN THE BULK UPDATE DID NOT SUCCEED, TRY THEM INDIVIDUALLY
	// iterate through the response from CouchDB by document
	for _, resp := range responses {
		// If the document returned an error, retry the individual document
		if resp.Ok {
			c.updateRevisionInCacheUpdate(resp.ID, resp.Rev)
			continue
		}
		doc := c.batchUpdateMap[resp.ID]

		var err error
		// Remove the "_rev" from the JSON before saving
		// this will allow the CouchDB retry logic to retry revisions without encountering
		// a mismatch between the "If-Match" and the "_rev" tag in the JSON
		if doc.CouchDoc.jsonValue != nil {
			err = removeJSONRevision(&doc.CouchDoc.jsonValue)
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
			err = c.db.deleteDoc(resp.ID, "")
		} else {
			logger.Warningf("CouchDB batch document update encountered an problem. Reason:%s, Retrying update for document ID:%s", resp.Reason, resp.ID)
			// Save the individual document to couchdb
			// Note that this will do retries as needed
			var revision string
			revision, err = c.db.saveDoc(resp.ID, "", &doc.CouchDoc)
			c.updateRevisionInCacheUpdate(resp.ID, revision)
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

func (vdb *VersionedDB) getRevisions(ns string, nsUpdates map[string]*statedb.VersionedValue) (map[string]string, error) {
	revisions := make(map[string]string)
	nsRevs := vdb.committedDataCache.revs[ns]

	var missingKeys []string
	var ok bool
	for key := range nsUpdates {
		if revisions[key], ok = nsRevs[key]; !ok {
			missingKeys = append(missingKeys, key)
		}
	}

	if len(missingKeys) == 0 {
		// all revisions were present in the committedDataCache
		return revisions, nil
	}

	missingKeys, err := vdb.addMissingRevisionsFromCache(ns, missingKeys, revisions)
	if err != nil {
		return nil, err
	}

	if len(missingKeys) == 0 {
		// remaining revisions were present in the state cache
		return revisions, nil
	}

	// don't update the cache for missing entries as
	// revisions are going to get changed after the commit
	if err := vdb.addMissingRevisionsFromDB(ns, missingKeys, revisions); err != nil {
		return nil, err
	}
	return revisions, nil
}

func (vdb *VersionedDB) addMissingRevisionsFromCache(ns string, keys []string, revs map[string]string) ([]string, error) {
	if !vdb.cache.enabled(ns) {
		return keys, nil
	}

	var missingKeys []string
	for _, k := range keys {
		cv, err := vdb.cache.getState(vdb.chainName, ns, k)
		if err != nil {
			return nil, err
		}
		if cv == nil {
			missingKeys = append(missingKeys, k)
			continue
		}
		revs[k] = string(cv.AdditionalInfo)
	}
	return missingKeys, nil
}

func (vdb *VersionedDB) addMissingRevisionsFromDB(ns string, missingKeys []string, revisions map[string]string) error {
	db, err := vdb.getNamespaceDBHandle(ns)
	if err != nil {
		return err
	}

	logger.Debugf("Pulling revisions for the [%d] keys for namsespace [%s] that were not part of the readset", len(missingKeys), db.dbName)
	retrievedMetadata, err := retrieveNsMetadata(db, missingKeys)
	if err != nil {
		return err
	}
	for _, metadata := range retrievedMetadata {
		revisions[metadata.ID] = metadata.Rev
	}

	return nil
}

// batchableDocument defines a document for a batch
type batchableDocument struct {
	CouchDoc couchDoc
	Deleted  bool
}
