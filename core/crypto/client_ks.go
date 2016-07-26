/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package crypto

import (
	"database/sql"
	"os"
)

func (client *clientImpl) initKeyStore() error {
	// Create TCerts directory
	os.MkdirAll(client.conf.getTCertsPath(), 0755)

	// create tables
	client.Debugf("Create Table if not exists [TCert] at [%s].", client.conf.getKeyStorePath())
	if _, err := client.ks.sqlDB.Exec("CREATE TABLE IF NOT EXISTS TCerts (id INTEGER, attrhash VARCHAR, cert BLOB, prkz BLOB, PRIMARY KEY (id))"); err != nil {
		client.Errorf("Failed creating table [%s].", err)
		return err
	}

	client.Debugf("Create Table if not exists [UsedTCert] at [%s].", client.conf.getKeyStorePath())
	if _, err := client.ks.sqlDB.Exec("CREATE TABLE IF NOT EXISTS UsedTCert (id INTEGER, attrhash VARCHAR, cert BLOB, prkz BLOB, PRIMARY KEY (id))"); err != nil {
		client.Errorf("Failed creating table [%s].", err)
		return err
	}

	return nil
}

func (ks *keyStore) storeUsedTCert(tCertBlck *TCertBlock) (err error) {
	ks.m.Lock()
	defer ks.m.Unlock()

	ks.node.Debug("Storing used TCert...")

	// Open transaction
	tx, err := ks.sqlDB.Begin()
	if err != nil {
		ks.node.Errorf("Failed beginning transaction [%s].", err)

		return
	}

	// Insert into UsedTCert
	if _, err = tx.Exec("INSERT INTO UsedTCert (attrhash, cert, prkz) VALUES (?, ?, ?)", tCertBlck.attributesHash, tCertBlck.tCert.GetCertificate().Raw, tCertBlck.tCert.GetPreK0()); err != nil {
		ks.node.Errorf("Failed inserting TCert to UsedTCert: [%s].", err)

		tx.Rollback()

		return
	}

	// Finalize
	err = tx.Commit()
	if err != nil {
		ks.node.Errorf("Failed commiting [%s].", err)
		tx.Rollback()

		return
	}

	ks.node.Debug("Storing used TCert...done!")

	//name, err := utils.TempFile(ks.conf.getTCertsPath(), "tcert_")
	//if err != nil {
	//	ks.node.error("Failed storing TCert: [%s]", err)
	//	return
	//}
	//
	//err = ioutil.WriteFile(name, tCert.GetCertificate().Raw, 0700)
	//if err != nil {
	//	ks.node.error("Failed storing TCert: [%s]", err)
	//	return
	//}

	return
}

func (ks *keyStore) storeUnusedTCerts(tCertBlocks []*TCertBlock) (err error) {
	ks.node.Debug("Storing unused TCerts...")

	if len(tCertBlocks) == 0 {
		ks.node.Debug("Empty list of unused TCerts.")
		return
	}

	// Open transaction
	tx, err := ks.sqlDB.Begin()
	if err != nil {
		ks.node.Errorf("Failed beginning transaction [%s].", err)

		return
	}

	for _, tCertBlck := range tCertBlocks {
		// Insert into UsedTCert
		if _, err = tx.Exec("INSERT INTO TCerts (attrhash, cert, prkz) VALUES (?, ?, ?)", tCertBlck.attributesHash, tCertBlck.tCert.GetCertificate().Raw, tCertBlck.tCert.GetPreK0()); err != nil {
			ks.node.Errorf("Failed inserting unused TCert to TCerts: [%s].", err)

			tx.Rollback()

			return
		}
	}

	// Finalize
	err = tx.Commit()
	if err != nil {
		ks.node.Errorf("Failed commiting [%s].", err)
		tx.Rollback()

		return
	}

	ks.node.Debug("Storing unused TCerts...done!")

	return
}

//Used by the MT pool
func (ks *keyStore) loadUnusedTCert() ([]byte, error) {
	// Get the first row available
	var id int
	var cert []byte
	row := ks.sqlDB.QueryRow("SELECT id, cert FROM TCerts")
	err := row.Scan(&id, &cert)

	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		ks.node.Errorf("Error during select [%s].", err.Error())

		return nil, err
	}

	// Remove from TCert
	if _, err := ks.sqlDB.Exec("DELETE FROM TCerts WHERE id = ?", id); err != nil {
		ks.node.Errorf("Failed removing row [%d] from TCert: [%s].", id, err.Error())

		return nil, err
	}

	return cert, nil
}

func (ks *keyStore) loadUnusedTCerts() ([]*TCertDBBlock, error) {
	// Get unused TCerts
	rows, err := ks.sqlDB.Query("SELECT attrhash, cert, prkz FROM TCerts")
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		ks.node.Errorf("Error during select [%s].", err)

		return nil, err
	}

	tCertDBBlocks := []*TCertDBBlock{}

	for {
		if rows.Next() {
			var tCertDER []byte
			var attributeHash string
			var prek0 []byte
			if err := rows.Scan(&attributeHash, &tCertDER, &prek0); err != nil {
				ks.node.Errorf("Error during scan [%s].", err)

				continue
			}

			var tCertBlk = new(TCertDBBlock)
			tCertBlk.attributesHash = attributeHash
			tCertBlk.preK0 = prek0
			tCertBlk.tCertDER = tCertDER

			tCertDBBlocks = append(tCertDBBlocks, tCertBlk)
		} else {
			break
		}
	}

	// Delete all entries
	if _, err = ks.sqlDB.Exec("DELETE FROM TCerts"); err != nil {
		ks.node.Errorf("Failed cleaning up unused TCert entries: [%s].", err)

		return nil, err
	}

	return tCertDBBlocks, nil
}
