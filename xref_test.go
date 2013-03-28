package xref

import (
	"code.google.com/p/go.exp/go/types"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"testing"
)

var verbose bool = true

func TestPackage(t *testing.T) {
	xs := xrefs("package p")
	if len(xs) != 1 {
		t.Fatalf("want exactly 1 xref, got %v", pp(xs))
	}
	x := xs[0]
	if pretty(x.Expr) != "p" {
		t.Errorf("want expression p, got %v", pretty(x.Expr))
	}
	if pretty(x.Ident) != "p" {
		t.Errorf("want ident p, got %v", pretty(x.Ident))
	}
	if x.ExprType != nil {
		t.Errorf("package clause should have nil type, got %v", x.ExprType)
	}
	if x.ReferPos != token.NoPos {
		t.Errorf("package ReferObj should have NoPos, got %v", x.ReferPos)
	}
	if _, ok := x.ReferObj.(*types.Package); !ok {
		t.Errorf("package ReferObj should be a types.Package, got %v", x.ReferObj)
	}
}

func TestVarDecl(t *testing.T) {
	xs := xrefs("package p; var A string")
	xA := xs[1]
	xstring := xs[2]

	if xA.Universe {
		t.Errorf("A should not be identified as a Universe type")
	}
	if !xA.IsDecl() {
		t.Errorf("A should be a decl, got %v", xA)
	}

	if pretty(xstring.Expr) != "string" || pretty(xstring.Ident) != "string" {
		t.Errorf("want Expr and Ident to be 'string', got Expr=%v and Ident=%v", pretty(xstring.Expr), pretty(xstring.Ident))
	}
	if !xstring.Universe {
		t.Errorf("string should be identified as a Universe type")
	}
	if xstring.IsDecl() {
		t.Errorf("string is not declared here, got %v", xstring)
	}
}

func TestVarDeclWithInferredType(t *testing.T) {
	xs := xrefs(`package p; var A = "a"`)
	xA := xs[1]

	if len(xs) != 2 {
		t.Errorf("want exactly 2 xrefs, got %v", xs)
	}

	if xA.ExprType == nil || xA.ExprType.String() != "string" {
		t.Errorf("want A's type to be 'string', got %v %v", xA.ExprType, xA.ReferObj)
	}
	if !xA.IsDecl() {
		t.Errorf("A should be a decl, got %v", xA)
	}
}

func TestVarCrossPackageXref(t *testing.T) {
	xs := xrefs(`package p; import "flag"; var A = flag.ErrHelp`)

	if len(xs) != 4 {
		t.Fatalf("want exactly 4 xrefs, got %v", xs)
	}

	xflag := xs[2]
	xErrHelp := xs[3]

	if pretty(xflag.Expr) != "flag" {
		t.Errorf("want Expr to be 'flag', got Expr=%v", pretty(xflag.Expr))
	}

	if pretty(xErrHelp.Expr) != "flag.ErrHelp" || pretty(xErrHelp.Ident) != "ErrHelp" {
		t.Errorf("want Expr to be 'flag.ErrHelp' and Ident to be 'ErrHelp', got Ident=%v and Expr=%v", pretty(xErrHelp.Ident), pretty(xErrHelp.Expr))
	}
	if errHelp, ok := xErrHelp.ReferObj.(*types.Var); ok {
		if errHelp.GetPkg().Name != "flag" {
			t.Errorf("want flag.ErrHelp to be in pkg named flag, got %v", errHelp.GetPkg().Name)
		}
		if errHelp.GetPkg().Path != "flag" {
			t.Errorf("want flag.ErrHelp to be in pkg with import path flag, got %v", errHelp.GetPkg().Path)
		}
	} else {
		t.Errorf("want flag.ErrHelp ReferObj to be a types.Var, got %v", xErrHelp.ReferObj)
	}
}

func xrefs(src string) []Xref {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", src, 0)
	return collectXrefs(f)
}

func collectXrefs(f *ast.File) (xs []Xref) {
	c := NewContext()
	c.Logf = func(pos token.Pos, f string, a ...interface{}) {
		if !verbose {
			return
		}
		log.Printf("%v: %s", c.position(pos), fmt.Sprintf(f, a...))
	}

	xs = make([]Xref, 0)
	c.IterateXrefs(f, func(xref *Xref) bool {
		xs = append(xs, *xref)
		return true
	})
	return xs
}

func (ctxt *Context) position(pos token.Pos) token.Position {
	return ctxt.FileSet.Position(pos)
}

func pp(xs []Xref) string {
	s := "["
	for i, x := range xs {
		if i > 0 {
			s += ", "
		}
		s += x.String()
	}
	return s + "]"
}
