package msp

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
)

type Database map[string][][]byte

func (d *Database) ValidUser(name string) bool {
	_, ok := (*d)[name]
	return ok
}

func (d *Database) CanGetShare(name string) bool {
	_, ok := (*d)[name]
	return ok
}

func (d *Database) GetShare(name string) ([][]byte, error) {
	out, ok := (*d)[name]

	if ok {
		return out, nil
	} else {
		return nil, errors.New("Not found!")
	}
}

func TestMSP(t *testing.T) {
	db := &Database{
		"Alice": [][]byte{},
		"Bob":   [][]byte{},
		"Carl":  [][]byte{},
	}

	sec := make([]byte, 16)
	rand.Read(sec)
	sec[0] &= 63 // Removes first 2 bits of key.

	predicate, _ := StringToMSP("(2, (1, Alice, Bob), Carl)")

	shares1, _ := predicate.DistributeShares(sec, db)
	shares2, _ := predicate.DistributeShares(sec, db)

	alice := bytes.Compare(shares1["Alice"][0], shares2["Alice"][0])
	bob := bytes.Compare(shares1["Bob"][0], shares2["Bob"][0])
	carl := bytes.Compare(shares1["Carl"][0], shares2["Carl"][0])

	if alice == 0 && bob == 0 && carl == 0 {
		t.Fatalf("Key splitting isn't random! %v %v", shares1, shares2)
	}

	db1 := Database(shares1)
	db2 := Database(shares2)

	sec1, err := predicate.RecoverSecret(&db1)
	if err != nil {
		t.Fatalf("#1: %v", err)
	}

	sec2, err := predicate.RecoverSecret(&db2)
	if err != nil {
		t.Fatalf("#2: %v", err)
	}

	if !(bytes.Compare(sec, sec1) == 0 && bytes.Compare(sec, sec2) == 0) {
		t.Fatalf("Secrets derived differed:  %v %v %v", sec, sec1, sec2)
	}
}
