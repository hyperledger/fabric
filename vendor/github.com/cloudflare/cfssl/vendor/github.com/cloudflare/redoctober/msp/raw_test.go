package msp

import (
	"testing"
)

func TestRaw(t *testing.T) {
	alice := Condition(Name{"Alice", 0})
	bob := Condition(Name{"Bob", 0})
	carl := Condition(Name{"Carl", 0})

	query1 := Raw{
		NodeType: NodeAnd,
		Left:     alice,
		Right:    bob,
	}

	aliceOrBob := Condition(Raw{
		NodeType: NodeOr,
		Left:     alice,
		Right:    bob,
	})

	query2 := Raw{
		NodeType: NodeAnd,
		Left:     aliceOrBob,
		Right:    carl,
	}

	db := &Database{
		"Alice": [][]byte{[]byte("blah")},
		"Carl":  [][]byte{[]byte("herp")},
	}

	if query1.Ok(db) != false {
		t.Fatalf("Query #1 was wrong.")
	}

	if query2.Ok(db) != true {
		t.Fatalf("Query #2 was wrong.")
	}

	query1String := "Alice & Bob"
	query2String := "(Alice | Bob) & Carl"

	if query1.String() != query1String {
		t.Fatalf("Query #1 String was wrong; %v", query1.String())
	}

	if query2.String() != query2String {
		t.Fatalf("Query #2 String was wrong; %v", query2.String())
	}

	decQuery1, err := StringToRaw(query1String)
	if err != nil || decQuery1.String() != query1String {
		t.Fatalf("Query #1 decoded wrong: %v %v", decQuery1.String(), err)
	}

	decQuery2, err := StringToRaw(query2String)
	if err != nil || decQuery2.String() != query2String {
		t.Fatalf("Query #2 decoded wrong: %v %v", decQuery2.String(), err)
	}

	formattedQuery1String := "(2, Alice, Bob)"
	formattedQuery2String := "(2, (1, Alice, Bob), Carl)"

	if query1.Formatted().String() != formattedQuery1String {
		t.Fatalf("Query #1 formatted wrong: %v", query1.Formatted().String())
	}

	if query2.Formatted().String() != formattedQuery2String {
		t.Fatalf("Query #2 formatted wrong: %v", query2.Formatted().String())
	}
}

func TestOneCondition(t *testing.T) {
	_, err := StringToRaw("(Alice or Bob)")

	if err == nil {
		t.Fatalf("A predicate with only one condition should fail to parse!")
	}
}
