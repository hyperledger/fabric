package patcher

import (
	"reflect"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/checker/nature"
	"github.com/expr-lang/expr/conf"
)

// WithContext adds WithContext.Name argument to all functions calls with a context.Context argument.
type WithContext struct {
	Name      string
	Functions conf.FunctionsTable // Optional: used to look up function types when callee type is unknown.
	Env       *nature.Nature      // Optional: used to look up method types when callee type is unknown.
	NtCache   *nature.Cache       // Optional: cache for nature lookups.
}

// Visit adds WithContext.Name argument to all functions calls with a context.Context argument.
func (w WithContext) Visit(node *ast.Node) {
	switch call := (*node).(type) {
	case *ast.CallNode:
		fn := call.Callee.Type()
		if fn == nil {
			return
		}
		// If callee type is interface{} (unknown), look up the function type from
		// the Functions table or Env. This handles cases where the checker returns early
		// without visiting nested call arguments (e.g., Date2() in Now2().After(Date2()))
		// because the outer call's type is unknown due to missing context arguments.
		if fn.Kind() == reflect.Interface {
			if ident, ok := call.Callee.(*ast.IdentifierNode); ok {
				if w.Functions != nil {
					if f, ok := w.Functions[ident.Value]; ok {
						fn = f.Type()
					}
				}
				if fn.Kind() == reflect.Interface && w.Env != nil {
					if m, ok := w.Env.MethodByName(w.NtCache, ident.Value); ok {
						fn = m.Type
					}
				}
			}
		}
		if fn.Kind() != reflect.Func {
			return
		}
		switch fn.NumIn() {
		case 0:
			return
		case 1:
			if fn.In(0).String() != "context.Context" {
				return
			}
		default:
			if fn.In(0).String() != "context.Context" &&
				fn.In(1).String() != "context.Context" {
				return
			}
		}
		ast.Patch(node, &ast.CallNode{
			Callee: call.Callee,
			Arguments: append([]ast.Node{
				&ast.IdentifierNode{Value: w.Name},
			}, call.Arguments...),
		})
	}
}
