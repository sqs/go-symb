package xref

import (
	"code.google.com/p/go.exp/go/types"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

func TestFuncSignatureXrefs(t *testing.T) {
	xs := xrefs(`package p; func A(b, c string, d bool) (e, f int, g uint) { panic() }`)

	if len(xs) != 13 {
		t.Fatalf("want exactly 13 xrefs, got %v", xs)
	}

	wantps := "[A, b, c, string, d, bool, e, f, int, g, uint]"
	if prettys(xs[1:12]) != wantps {
		t.Errorf("want func sig exprs %v, got %v", wantps, prettys(xs[1:12]))
	}
}

var testPkgPaths = []string{
	"foo",
}

func TestXref(t *testing.T) {
	build.Default.GOPATH, _ = filepath.Abs("test_gopath/")
	for _, pkgPath := range testPkgPaths {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, filepath.Join(build.Default.GOPATH, "src", pkgPath), goFilesOnly, 0)
		if err != nil {
			t.Errorf("Error parsing %s: %v", pkgPath, err)
			continue
		}

		for _, pkg := range pkgs {
			for filename, file := range pkg.Files {
				xs := collectXrefs(file)
				checkOutput(filename, xs, t)
			}
		}
	}
}

func goFilesOnly(file os.FileInfo) bool {
	return file.Mode().IsRegular() && path.Ext(file.Name()) == ".go"
}

func checkOutput(srcFilename string, xs []Xref, t *testing.T) {
	actualFilename := srcFilename + "_actual.json"
	expectedFilename := srcFilename + "_expected.json"

	// write actual output
	writeJson(actualFilename, xrefsToJson(xs))

	// diff
	cmd := exec.Command("diff", "-u", expectedFilename, actualFilename)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	cmd.Wait()
	if !cmd.ProcessState.Success() {
		t.Errorf("%s: actual output did not match expected output", srcFilename)
	}
}

func writeJson(filename string, v interface{}) {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
	if err != nil {
		panic("Error opening file: " + err.Error())
	}
	defer f.Close()

	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic("Error printing JSON: " + err.Error())
	}

	_, err = f.Write(enc)
	if err != nil {
		panic("Error writing JSON: " + err.Error())
	}
	f.Write([]byte{'\n'})
}

var fset = token.NewFileSet()

func xrefs(src string) []Xref {
	f, _ := parser.ParseFile(fset, "test.go", src, 0)
	return collectXrefs(f)
}

func collectXrefs(f *ast.File) (xs []Xref) {
	c := NewContext()
	c.FileSet = fset
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

func xrefsToJson(xs []Xref) []interface{} {
	js := make([]interface{}, 0)
	for _, x := range xs {
		var exprType string
		if x.ExprType != nil {
			exprType = x.ExprType.String()
		}
		j := struct {
			Expr     string
			Ident    string
			ExprType string
			Pkg      interface{}
			ReferPos token.Position
			ReferObj interface{}
			Local    bool
			Universe bool
		}{
			Expr:     pretty(x.Expr),
			Ident:    pretty(x.Ident),
			ExprType: exprType,
			Pkg:      typePackageToJson(x.Pkg),
			ReferPos: fset.Position(x.ReferPos),
			ReferObj: typeObjectToJson(&x.ReferObj),
			Local:    x.Local,
			Universe: x.Universe,
		}
		js = append(js, j)
	}
	return js
}

func typePackageToJson(p *types.Package) interface{} {
	if p == nil {
		return nil
	} else {
		return struct {
			Isa, Name, ImportPath string
		}{
			"Package", p.Name, p.Path,
		}
	}
}

func typeTypeToJson(t types.Type) interface{} {
	return t.String()
}

func typeObjectToJson(o *types.Object) interface{} {
	switch o := (*o).(type) {
	case *types.Package:
		return typePackageToJson(o)
	case *types.Const:
		return struct {
			Isa  string
			Pkg  interface{}
			Name string
			Type interface{}
			Val  interface{}
		}{
			"Const", typePackageToJson(o.Pkg), o.Name, typeTypeToJson(o.Type), o.Val,
		}
	case *types.TypeName:
		return struct {
			Isa  string
			Pkg  interface{}
			Name string
			Type interface{}
		}{
			"TypeName", typePackageToJson(o.Pkg), o.Name, typeTypeToJson(o.Type),
		}
	case *types.Var:
		return struct {
			Isa  string
			Pkg  interface{}
			Name string
			Type interface{}
		}{
			"Var", typePackageToJson(o.Pkg), o.Name, typeTypeToJson(o.Type),
		}
	case *types.Func:
		return struct {
			Isa  string
			Pkg  interface{}
			Name string
			Type interface{}
		}{
			"Func", typePackageToJson(o.Pkg), o.Name, typeTypeToJson(o.Type),
		}
	default:
		if o != nil {
			return nil
		} else {
			return "UNKNOWN"
		}
	}
	return nil
}

func prettys(xs []Xref) string {
	s := "["
	for i, x := range xs {
		if i > 0 {
			s += ", "
		}
		s += pretty(x.Expr)
	}
	return s + "]"
}
