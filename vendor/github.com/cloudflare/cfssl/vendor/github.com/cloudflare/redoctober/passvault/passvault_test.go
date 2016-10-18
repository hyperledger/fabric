// passvault_test: tests for passvault.go
//
// Copyright (c) 2013 CloudFlare, Inc.

package passvault

import (
	"os"
	"testing"
)

func TestStaticVault(t *testing.T) {
	// Creates a temporary on-disk database to test if passvault can read and
	// write from/to disk.  It's deleted at the bottom of the function--this
	// should be the only test that requires touching disk.
	records, err := InitFrom("/tmp/redoctober.json")
	if err != nil {
		t.Fatalf("Error reading record")
	}

	_, err = records.AddNewRecord("test", "bad pass", true, DefaultRecordType)
	if err != nil {
		t.Fatalf("Error creating record")
	}

	// Reads data written last time.
	_, err = InitFrom("/tmp/redoctober.json")
	if err != nil {
		t.Fatalf("Error reading record")
	}

	os.Remove("/tmp/redoctober.json")
}

func TestRSAEncryptDecrypt(t *testing.T) {
	records, err := InitFrom("memory")
	if err != nil {
		t.Fatalf("%v", err)
	}

	myRec, err := records.AddNewRecord("user", "weakpassword", true, RSARecord)
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, err = myRec.GetKeyRSAPub()
	if err != nil {
		t.Fatalf("Error extracting RSA Pub")
	}

	rsaPriv, err := myRec.GetKeyRSA("mypasswordiswrong")
	if err == nil {
		t.Fatalf("Incorrect password did not fail")
	}

	rsaPriv, err = myRec.GetKeyRSA("weakpassword")
	if err != nil {
		t.Fatalf("Error decrypting RSA key")
	}

	err = rsaPriv.Validate()
	if err != nil {
		t.Fatalf("Error validating RSA key")
	}
}

func TestECCEncryptDecrypt(t *testing.T) {
	records, err := InitFrom("memory")
	if err != nil {
		t.Fatalf("%v", err)
	}

	myRec, err := records.AddNewRecord("user", "weakpassword", true, ECCRecord)
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, err = myRec.GetKeyECCPub()
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, err = myRec.GetKeyECC("mypasswordiswrong")
	if err == nil {
		t.Fatalf("Incorrect password did not fail")
	}

	_, err = myRec.GetKeyECC("weakpassword")
	if err != nil {
		t.Fatalf("%v", err)
	}
}

func TestChangePassword(t *testing.T) {
	records, err := InitFrom("memory")
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Check changing the password for a non-existent user
	err = records.ChangePassword("user", "weakpassword", "newpassword", "")
	if err == nil {
		t.Fatalf("%v", err)
	}

	_, err = records.AddNewRecord("user", "weakpassword", true, ECCRecord)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = records.ChangePassword("user", "weakpassword", "newpassword", "")
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, err = records.AddNewRecord("user2", "weakpassword", true, RSARecord)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = records.ChangePassword("user2", "weakpassword", "newpassword", "")
	if err != nil {
		t.Fatalf("%v", err)
	}
}

func TestNumRecords(t *testing.T) {
	records, err := InitFrom("memory")
	if err != nil {
		t.Fatalf("%v", err)
	}

	if recordCount := records.NumRecords(); recordCount != 0 {
		t.Fatalf("Error in number of records. Expected 0, got %d.", recordCount)
	}

	if _, err = records.AddNewRecord("user", "password", true, RSARecord); err != nil {
		t.Fatalf("%v", err)
	}

	if recordCount := records.NumRecords(); recordCount != 1 {
		t.Fatalf("Error in number of records. Expected 1, got %d.", recordCount)
	}
}

func TestGetSummary(t *testing.T) {
	records, err := InitFrom("memory")
	if err != nil {
		t.Fatalf("%v", err)
	}

	if summary := records.GetSummary(); len(summary) != 0 {
		t.Fatalf("Expected 0 elements in summary but got %d instead.", len(summary))
	}

	if _, err := records.AddNewRecord("user1", "password", true, RSARecord); err != nil {
		t.Fatalf("%v", err)
	}

	if _, err := records.AddNewRecord("user2", "password", false, ECCRecord); err != nil {
		t.Fatalf("%v", err)
	}

	summary := records.GetSummary()
	if len(summary) != 2 {
		t.Fatalf("Expected 2 elements in summary but got %d instead.", len(summary))
	}

	user := summary["user1"]
	if user.Admin != true || user.Type != RSARecord {
		t.Fatalf("Data retrieved for user1 invalid. Expected {Admin:true Type:%s}, got %+v", RSARecord, user)
	}

	user = summary["user2"]
	if user.Admin != false || user.Type != ECCRecord {
		t.Fatalf("Data retrieved for user1 invalid. Expected {Admin:false Type:%s}, got %+v", ECCRecord, user)
	}
}

func TestDeleteRecord(t *testing.T) {
	records, err := InitFrom("memory")
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, err = records.AddNewRecord("user", "weakpassword", true, ECCRecord)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = records.DeleteRecord("user")
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, retVal := records.GetRecord("user")
	if retVal == true {
		t.Fatalf("Record not deleting properly")
	}
}

func TestMakeRevokeAdmin(t *testing.T) {
	records, err := InitFrom("memory")
	if err != nil {
		t.Fatalf("%v", err)
	}

	myRec, err := records.AddNewRecord("user", "weakpassword", false, ECCRecord)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = records.MakeAdmin("user")
	if err != nil {
		t.Fatalf("%v", err)
	}

	myRec, _ = records.GetRecord("user")
	retval := myRec.IsAdmin()
	if retval != true {
		t.Fatalf("Incorrect Admin value")
	}

	err = records.RevokeRecord("user")
	if err != nil {
		t.Fatalf("%v", err)
	}

	myRec, _ = records.GetRecord("user")
	retval = myRec.IsAdmin()
	if retval != false {
		t.Fatalf("Incorrect Admin value")
	}

}
