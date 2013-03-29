package xref

import (
	"code.google.com/p/qslack-gotypes/go/types"
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

var fset = token.NewFileSet()

var testPkgPaths = []string{
	"foo",
}

func TestXref(t *testing.T) {
	build.Default.GOPATH, _ = filepath.Abs("test_gopath/")
	for _, pkgPath := range testPkgPaths {
		pkgs, err := parser.ParseDir(fset, filepath.Join(build.Default.GOPATH, "src", pkgPath), goFilesOnly, parser.AllErrors|parser.DeclarationErrors)
		if err != nil {
			t.Errorf("Error parsing %s: %v", pkgPath, err)
			continue
		}

		for _, pkg := range pkgs {
			xrefs := collectXrefs(pkg)
			xrefsByFilename := make(map[string][]Xref, 0)
			for _, x := range xrefs {
				filename := fset.Position(x.Ident.Pos()).Filename
				if xrefsByFilename[filename] == nil {
					xrefsByFilename[filename] = make([]Xref, 0)
				}
				xrefsByFilename[filename] = append(xrefsByFilename[filename], x)
			}

			for filename, xs := range xrefsByFilename {
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

func collectXrefs(pkg *ast.Package) (xs []Xref) {
	c := NewContext()
	c.FileSet = fset
	c.Logf = func(pos token.Pos, f string, a ...interface{}) {
		if !verbose {
			return
		}
		log.Printf("%v: %s", c.position(pos), fmt.Sprintf(f, a...))
	}

	xs = make([]Xref, 0)
	c.IterateXrefs(pkg, func(xref *Xref) bool {
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
			IdentPos interface{}
			ExprType string
			Pkg      interface{}
			FileName string
			ReferPos token.Position
			ReferObj interface{}
			Local    bool
			Universe bool
			IsDecl   bool
		}{
			Expr:     pretty(x.Expr),
			Ident:    pretty(x.Ident),
			IdentPos: relativePosition(fset.Position(x.Ident.Pos())),
			ExprType: exprType,
			Pkg:      typePackageToJson(x.Pkg),
			FileName: x.File.Name.Name,
			ReferPos: relativePosition(fset.Position(x.ReferPos)),
			ReferObj: typeObjectToJson(&x.ReferObj),
			Local:    x.Local,
			Universe: x.Universe,
			IsDecl:   x.IsDecl(),
		}
		js = append(js, j)
	}
	return js
}

func relativePosition(p token.Position) token.Position {
	cwd, _ := os.Getwd()
	p.Filename, _ = filepath.Rel(cwd, p.Filename)
	return p
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
	if t != nil {
		return t.String()
	} else {
		return nil
	}
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
