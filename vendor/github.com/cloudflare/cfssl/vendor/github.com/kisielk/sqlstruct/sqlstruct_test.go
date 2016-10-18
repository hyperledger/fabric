// Copyright 2012 Kamil Kisiel. All rights reserved.
// Use of this source code is governed by the MIT
// license which can be found in the LICENSE file.
package sqlstruct

import (
	"reflect"
	"testing"
)

type EmbeddedType struct {
	FieldE string `sql:"field_e"`
}

type testType struct {
	FieldA  string `sql:"field_a"`
	FieldB  string `sql:"-"`       // Ignored
	FieldC  string `sql:"field_C"` // Different letter case
	Field_D string // Field name is used
	EmbeddedType
}

type testType2 struct {
	FieldA   string `sql:"field_a"`
	FieldSec string `sql:"field_sec"`
}

// testRows is a mock version of sql.Rows which can only scan strings
type testRows struct {
	columns []string
	values  []interface{}
}

func (r testRows) Scan(dest ...interface{}) error {
	for i := range r.values {
		v := reflect.ValueOf(dest[i])
		if v.Kind() != reflect.Ptr {
			panic("Not a pointer!")
		}

		switch dest[i].(type) {
		case *string:
			*(dest[i].(*string)) = r.values[i].(string)
		default:
			// Do nothing. We assume the tests only use strings here
		}
	}
	return nil
}

func (r testRows) Columns() ([]string, error) {
	return r.columns, nil
}

func (r *testRows) addValue(c string, v interface{}) {
	r.columns = append(r.columns, c)
	r.values = append(r.values, v)
}

func TestColumns(t *testing.T) {
	var v testType
	e := "field_a, field_c, field_d, field_e"
	c := Columns(v)

	if c != e {
		t.Errorf("expected %q got %q", e, c)
	}
}

func TestColumnsAliased(t *testing.T) {
	var t1 testType
	var t2 testType2

	expected := "t1.field_a AS t1_field_a, t1.field_c AS t1_field_c, "
	expected += "t1.field_d AS t1_field_d, t1.field_e AS t1_field_e"
	actual := ColumnsAliased(t1, "t1")

	if expected != actual {
		t.Errorf("Expected %q got %q", expected, actual)
	}

	expected = "t2.field_a AS t2_field_a, t2.field_sec AS t2_field_sec"
	actual = ColumnsAliased(t2, "t2")

	if expected != actual {
		t.Errorf("Expected %q got %q", expected, actual)
	}
}

func TestScan(t *testing.T) {
	rows := testRows{}
	rows.addValue("field_a", "a")
	rows.addValue("field_b", "b")
	rows.addValue("field_c", "c")
	rows.addValue("field_d", "d")
	rows.addValue("field_e", "e")

	e := testType{"a", "", "c", "d", EmbeddedType{"e"}}

	var r testType
	err := Scan(&r, rows)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if r != e {
		t.Errorf("expected %q got %q", e, r)
	}
}

func TestScanAliased(t *testing.T) {
	rows := testRows{}
	rows.addValue("t1_field_a", "a")
	rows.addValue("t1_field_b", "b")
	rows.addValue("t1_field_c", "c")
	rows.addValue("t1_field_d", "d")
	rows.addValue("t1_field_e", "e")
	rows.addValue("t2_field_a", "a2")
	rows.addValue("t2_field_sec", "sec")

	expected := testType{"a", "", "c", "d", EmbeddedType{"e"}}
	var actual testType
	err := ScanAliased(&actual, rows, "t1")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if expected != actual {
		t.Errorf("expected %q got %q", expected, actual)
	}

	expected2 := testType2{"a2", "sec"}
	var actual2 testType2

	err = ScanAliased(&actual2, rows, "t2")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if expected2 != actual2 {
		t.Errorf("expected %q got %q", expected2, actual2)
	}
}

func TestToSnakeCase(t *testing.T) {
	var s string
	s = ToSnakeCase("FirstName")
	if "first_name" != s {
		t.Errorf("expected first_name got %q", s)
	}

	s = ToSnakeCase("First")
	if "first" != s {
		t.Errorf("expected first got %q", s)
	}

	s = ToSnakeCase("firstName")
	if "first_name" != s {
		t.Errorf("expected first_name got %q", s)
	}
}
