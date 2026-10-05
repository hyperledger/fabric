/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dbfactory

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	db "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/stretchr/testify/require"
)

// choicePoints are the functions that turn a database type into a working store.
var choicePoints = []string{"CreateDB", "NewProvider", "NewFileLock", "RetrieveDataFormatInfo"}

func TestCreateDBKnownType(t *testing.T) {
	dbPath := t.TempDir()

	dbInst := CreateDB(db.GoLevelDB, dbPath, "")
	dbInst.Open()
	require.NoError(t, dbInst.Put([]byte("key1"), []byte("value1"), true))
	require.NoError(t, dbInst.Put([]byte("key2"), []byte("value2"), true))
	dbInst.Close()

	dbInst = CreateDB(db.GoLevelDB, dbPath, "")
	dbInst.Open()
	defer dbInst.Close()

	dbEmpty, err := dbInst.IsEmpty()
	require.NoError(t, err)
	require.False(t, dbEmpty)

	itr, err := dbInst.GetIterator(nil, nil)
	require.NoError(t, err)
	defer itr.Release()

	require.True(t, itr.Next())
	require.Equal(t, "key1", string(itr.Key()))
	require.Equal(t, "value1", string(itr.Value()))
	require.True(t, itr.Next())
	require.Equal(t, "key2", string(itr.Key()))
	require.Equal(t, "value2", string(itr.Value()))
	require.False(t, itr.Next())
	require.NoError(t, itr.Error())
}

func TestCreateDBUnknownTypeOpensTheStoreOfTheLevelDBType(t *testing.T) {
	// An empty or a misspelled type has to open the very same store as the
	// leveldb type, in both directions: nothing may be written to a store the
	// leveldb type cannot read, and the other way round.
	for _, dbType := range []string{"", "goLevelDB"} {
		t.Run(fmt.Sprintf("dbType=%q", dbType), func(t *testing.T) {
			dbPath := t.TempDir()

			levelDB := CreateDB(db.GoLevelDB, dbPath, "")
			levelDB.Open()
			require.NoError(t, levelDB.Put([]byte("key1"), []byte("written as goleveldb"), true))
			levelDB.Close()

			otherDB := CreateDB(dbType, dbPath, "")
			otherDB.Open()
			val, err := otherDB.Get([]byte("key1"))
			require.NoError(t, err)
			require.Equal(t, "written as goleveldb", string(val))
			require.NoError(t, otherDB.Put([]byte("key2"), []byte(fmt.Sprintf("written as %q", dbType)), true))
			otherDB.Close()

			levelDB = CreateDB(db.GoLevelDB, dbPath, "")
			levelDB.Open()
			defer levelDB.Close()

			val, err = levelDB.Get([]byte("key2"))
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("written as %q", dbType), string(val))
		})
	}
}

func TestNewProviderUnknownTypeOpensTheStoreOfTheLevelDBType(t *testing.T) {
	dbPath := t.TempDir()

	provider, err := NewProvider(db.GoLevelDB, dbPath, dataformat.CurrentFormat)
	require.NoError(t, err)
	require.NoError(t, provider.GetDBHandle("channel").Put([]byte("key"), []byte("value"), true))
	dbFormat, err := provider.GetDataFormat()
	require.NoError(t, err)
	require.Equal(t, dataformat.CurrentFormat, dbFormat)
	provider.Close()

	for _, dbType := range []string{"", "goLevelDB"} {
		t.Run(fmt.Sprintf("dbType=%q", dbType), func(t *testing.T) {
			otherProvider, err := NewProvider(dbType, dbPath, dataformat.CurrentFormat)
			require.NoError(t, err)
			defer otherProvider.Close()

			val, err := otherProvider.GetDBHandle("channel").Get([]byte("key"))
			require.NoError(t, err)
			require.Equal(t, "value", string(val))
		})
	}
}

func TestRetrieveDataFormatInfoUnknownTypeReadsTheStoreOfTheLevelDBType(t *testing.T) {
	dbPath := t.TempDir()

	provider, err := NewProvider(db.GoLevelDB, dbPath, dataformat.CurrentFormat)
	require.NoError(t, err)
	provider.Close()

	for _, dbType := range []string{"", "goLevelDB"} {
		t.Run(fmt.Sprintf("dbType=%q", dbType), func(t *testing.T) {
			format, dbEmpty, err := RetrieveDataFormatInfo(dbType, dbPath)
			require.NoError(t, err)
			require.Equal(t, dataformat.CurrentFormat, format)
			require.False(t, dbEmpty)
		})
	}
}

func TestNewFileLockUnknownTypeLocksTheFileOfTheLevelDBType(t *testing.T) {
	// A lock taken for an unknown type has to exclude the lock taken for the
	// leveldb type on the same path, so that both types name one lock.
	for _, dbType := range []string{"", "goLevelDB"} {
		t.Run(fmt.Sprintf("dbType=%q", dbType), func(t *testing.T) {
			dbPath := t.TempDir()

			levelDBLock := NewFileLock(db.GoLevelDB, dbPath)
			require.NoError(t, levelDBLock.Lock())
			require.True(t, levelDBLock.IsLocked())
			defer levelDBLock.Unlock()

			otherLock := NewFileLock(dbType, dbPath)
			require.Error(t, otherLock.Lock())
			require.False(t, otherLock.IsLocked())

			levelDBLock.Unlock()
			require.NoError(t, otherLock.Lock())
			require.True(t, otherLock.IsLocked())
			otherLock.Unlock()
		})
	}
}

// A known type and the fallback cannot be told apart by the values the choice
// point returns, because a single store is left to return. The shape of the
// choice point is therefore asserted from the source instead: each of the four
// functions has to keep a branch for the known type beside the fallback branch,
// and to name the leveldb store once, so that a branch for the next store has
// somewhere to go.
func TestChoicePointHasABranchForTheKnownTypeAndAFallback(t *testing.T) {
	funcs := packageFunctions(t)

	for _, name := range choicePoints {
		t.Run(name, func(t *testing.T) {
			funcDecl, ok := funcs[name]
			require.True(t, ok, "%s is not declared by the package", name)

			var switches []*ast.SwitchStmt
			ast.Inspect(funcDecl, func(node ast.Node) bool {
				if switchStmt, ok := node.(*ast.SwitchStmt); ok {
					switches = append(switches, switchStmt)
				}
				return true
			})
			require.Len(t, switches, 1, "%s must select the store in one place", name)

			var knownType, fallback bool
			for _, clause := range switches[0].Body.List {
				caseClause, ok := clause.(*ast.CaseClause)
				require.True(t, ok, "%s selects with a switch on the database type", name)
				if caseClause.List == nil {
					fallback = true
					continue
				}
				for _, expr := range caseClause.List {
					knownType = knownType || isTypeConstant(expr)
				}
			}

			require.True(t, knownType, "%s has no branch for the type %q", name, db.GoLevelDB)
			require.True(t, fallback, "%s has no branch for an empty or unknown type", name)
			require.Equal(t, 1, levelDBStores(funcDecl), "%s must build the leveldb store once", name)
		})
	}
}

// isTypeConstant reports whether expr is a reference to the database type
// constant that the ledger contract declares as db.GoLevelDB.
func isTypeConstant(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	packageIdent, ok := selector.X.(*ast.Ident)
	return ok && packageIdent.Name == "db" && selector.Sel.Name == "GoLevelDB"
}

// levelDBStores returns the number of times the function builds a leveldb store.
func levelDBStores(funcDecl *ast.FuncDecl) int {
	var count int
	ast.Inspect(funcDecl, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		packageIdent, ok := selector.X.(*ast.Ident)
		if ok && packageIdent.Name == "leveldbhelper" {
			count++
		}
		return true
	})
	return count
}

func packageFunctions(t *testing.T) map[string]*ast.FuncDecl {
	t.Helper()

	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)
	pkg, ok := packages["dbfactory"]
	require.True(t, ok, "the package dbfactory is not parsed")

	funcs := map[string]*ast.FuncDecl{}
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Recv == nil {
				funcs[funcDecl.Name.Name] = funcDecl
			}
		}
	}
	return funcs
}
