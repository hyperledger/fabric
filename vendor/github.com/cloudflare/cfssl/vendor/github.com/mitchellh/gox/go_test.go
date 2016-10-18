package main

import (
	"strings"
	"testing"
)

func TestGoVersion(t *testing.T) {
	v, err := GoVersion()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	acceptable := []string{
		"devel", "go1.0", "go1.1", "go1.2", "go1.3", "go1.4.2", "go1.5",
		"go1.5.1",
	}
	found := false
	for _, expected := range acceptable {
		if strings.HasPrefix(v, expected) {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("bad: %#v", v)
	}
}
