// The go/xref package provides a way to iterate over the
// cross-references in Go source files. It is copied from rog-go's
// go/sym and adds the following features:
//
// * generates a unique, meaningful identifier for each xref
// * establishes cross-references even when there is no symbol
// * test coverage
package xref

import (
	"bytes"
	"code.google.com/p/go.exp/go/types"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
)

// Xref holds information about an xref.
type Xref struct {
	Expr     ast.Expr     // expression for xref (*ast.Ident or *ast.SelectorExpr)
	Ident    *ast.Ident   // identifier in parse tree
	ExprType types.Type   // type of expression.
	ReferPos token.Pos    // position of referred-to thing.
	ReferObj types.Object // object referred to.
	Local    bool         // whether referred-to object is function-local.
	Universe bool         // whether referred-to object is in universe.
}

// Context holds the context for IterateXrefs.
type Context struct {
	// FileSet holds the fileset used when importing packages.
	FileSet *token.FileSet

	// idObjs stores off go/types typecheck results for each ident.
	idObjs map[*ast.Ident]types.Object

	// exprTypes stores off go/types typecheck results for each expr.
	exprTypes map[ast.Expr]types.Type

	typesCtxt types.Context

	// Logf is used to print warning messages.
	// If it is nil, no warning messages will be printed.
	Logf func(pos token.Pos, f string, a ...interface{})
}

func NewContext() *Context {
	var ctxt *Context
	ctxt = &Context{
		FileSet:   token.NewFileSet(),
		idObjs:    make(map[*ast.Ident]types.Object, 0),
		exprTypes: make(map[ast.Expr]types.Type, 0),
		typesCtxt: types.Context{
			Ident: func(id *ast.Ident, obj types.Object) {
				ctxt.idObjs[id] = obj
			},
			Expr: func(e ast.Expr, typ types.Type, val interface{}) {
				ctxt.exprTypes[astBaseType(e)] = typeBaseType(typ)
			},
		},
	}

	return ctxt
}

func (ctxt *Context) logf(pos token.Pos, f string, a ...interface{}) {
	if ctxt.Logf == nil {
		return
	}
	ctxt.Logf(pos, f, a...)
}

// IterateXRefs calls visitf for each xref in the given file.  If
// visitf returns false, the iteration stops.
func (ctxt *Context) IterateXrefs(f *ast.File, visitf func(xref *Xref) bool) {
	ctxt.typesCtxt.Check(ctxt.FileSet, []*ast.File{f})

	var visit astVisitor
	ok := true
	local := false // TODO set to true inside function body
	visit = func(n ast.Node) bool {
		if !ok {
			return false
		}
		switch n := n.(type) {
		case *ast.ImportSpec:
			// If the file imports a package to ".", abort
			// because we don't support that (yet).
			if n.Name != nil && n.Name.Name == "." {
				ctxt.logf(n.Pos(), "import to . not supported")
				ok = false
				return false
			}
			return true

		case *ast.FuncDecl:
			// add object for init functions
			if n.Recv == nil && n.Name.Name == "init" {
				n.Name.Obj = ast.NewObj(ast.Fun, "init")
			}
			if n.Recv != nil {
				ast.Walk(visit, n.Recv)
			}
			var e ast.Expr = n.Name
			if n.Recv != nil {
				// It's a method, so we need to synthesise a
				// selector expression so that visitExpr doesn't
				// just see a blank name.
				if len(n.Recv.List) != 1 {
					ctxt.logf(n.Pos(), "expected one receiver only!")
					return true
				}
				e = &ast.SelectorExpr{
					X:   n.Recv.List[0].Type,
					Sel: n.Name,
				}
			}
			ok = ctxt.visitExpr(f, e, false, visitf)
			local = true
			ast.Walk(visit, n.Type)
			if n.Body != nil {
				ast.Walk(visit, n.Body)
			}
			local = false
			return false

		case *ast.Ident:
			ok = ctxt.visitExpr(f, n, local, visitf)
			return false

		case *ast.KeyValueExpr:
			// don't try to resolve the key part of a key-value
			// because it might be a map key which doesn't
			// need resolving, and we can't tell without being
			// complicated with types.
			ast.Walk(visit, n.Value)
			return false

		case *ast.SelectorExpr:
			ast.Walk(visit, n.X)
			ok = ctxt.visitExpr(f, n, local, visitf)
			return false

		case *ast.File:
			ok = ctxt.visitExpr(f, n.Name, false, visitf)
			for _, d := range n.Decls {
				ast.Walk(visit, d)
			}
			return false
		}

		return true
	}
	ast.Walk(visit, f)
}

func (ctxt *Context) filename(f *ast.File) string {
	return ctxt.FileSet.Position(f.Package).Filename
}

func (ctxt *Context) exprInfo(e ast.Expr) (obj types.Object, typ types.Type) {
	if id, ok := e.(*ast.Ident); ok {
		obj = ctxt.idObjs[id]
	}
	typ = ctxt.exprTypes[e]
	if typ == nil && obj != nil && obj.GetType() != types.Typ[types.Invalid] {
		typ = obj.GetType()
	}
	return
}

func (ctxt *Context) visitExpr(f *ast.File, e ast.Expr, local bool, visitf func(*Xref) bool) bool {
	var xref Xref
	xref.Expr = e
	switch e := e.(type) {
	case *ast.Ident:
		if e.Name == "_" {
			return true
		}
		xref.Ident = e
	case *ast.SelectorExpr:
		xref.Ident = e.Sel
	}
	obj, t := ctxt.exprInfo(xref.Ident)
	if obj == nil {
		ctxt.logf(xref.Ident.Pos(), "no object for %s", pretty(e))
		return true
	}
	xref.ExprType = t
	xref.ReferObj = obj
	if types.Universe.Lookup(obj.GetName()) != obj {
		xref.ReferPos = obj.GetPos()
	} else {
		xref.Universe = true
	}
	xref.Local = local
	return visitf(&xref)
}

type astVisitor func(n ast.Node) bool

func (f astVisitor) Visit(n ast.Node) ast.Visitor {
	if f(n) {
		return f
	}
	return nil
}

var emptyFileSet = token.NewFileSet()

func pretty(n ast.Node) string {
	var b bytes.Buffer
	printer.Fprint(&b, emptyFileSet, n)
	return b.String()
}

// astBaseType returns the base type expr for AST type expr x.
func astBaseType(e ast.Expr) ast.Expr {
	switch t := e.(type) {
	case *ast.ArrayType:
		return astBaseType(t.Elt)
	case *ast.MapType:
		return astBaseType(t.Value)
	case *ast.StarExpr:
		return astBaseType(t.X)
	}
	return e
}

// typeBaseType returns the base type for a types.Type.
func typeBaseType(t types.Type) types.Type {
	switch t := t.(type) {
	case *types.Array:
		return typeBaseType(t.Elt)
	case *types.Pointer:
		return typeBaseType(t.Base)
	case *types.Map:
		return typeBaseType(t.Elt) // TODO(sqs): also return Key type; typeBaseType needs to return multiple results?
	case *types.Slice:
		return typeBaseType(t.Elt)
	}
	return t
}

func (x *Xref) IsDecl() bool {
	return x.ReferPos == x.Ident.Pos()
}

func (x *Xref) String() string {
	return fmt.Sprintf("Xref{Expr=%v, Ident=%v, ExprType=%v}", x.Expr, x.Ident, x.ExprType)
}
