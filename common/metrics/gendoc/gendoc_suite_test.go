/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gendoc_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGendoc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gendoc Suite")
}

func ParseFile(filename string) (*ast.File, error) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	return f, nil
}
