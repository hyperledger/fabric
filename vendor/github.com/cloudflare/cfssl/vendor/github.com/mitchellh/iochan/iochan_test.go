package iochan

import (
	"bytes"
	"reflect"
	"testing"
)

func TestDelimReader(t *testing.T) {
	buf := new(bytes.Buffer)
	buf.WriteString("foo\nbar\nbaz")

	ch := DelimReader(buf, '\n')

	results := make([]string, 0, 3)
	expected := []string{"foo\n", "bar\n", "baz"}
	for v := range ch {
		results = append(results, v)
	}

	if !reflect.DeepEqual(results, expected) {
		t.Fatalf("unexpected results: %#v", results)
	}
}
