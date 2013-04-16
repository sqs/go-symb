package symb

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
	"bar",
}

func TestSymb(t *testing.T) {
	build.Default.GOPATH, _ = filepath.Abs("test_gopath/")
	for _, pkgPath := range testPkgPaths {
		pkgs, err := parser.ParseDir(fset, filepath.Join(build.Default.GOPATH, "src", pkgPath), goFilesOnly, parser.AllErrors|parser.DeclarationErrors)
		if err != nil {
			t.Errorf("Error parsing %s: %v", pkgPath, err)
			continue
		}

		for _, pkg := range pkgs {
			symbs := collectSymbs(pkg)
			symbsByFilename := make(map[string][]Symb, 0)
			for _, x := range symbs {
				filename := fset.Position(x.Ident.Pos()).Filename
				if symbsByFilename[filename] == nil {
					symbsByFilename[filename] = make([]Symb, 0)
				}
				symbsByFilename[filename] = append(symbsByFilename[filename], x)
			}

			for filename, symbs := range symbsByFilename {
				checkOutput(filename, symbs, t)
			}
		}
	}
}

func goFilesOnly(file os.FileInfo) bool {
	return file.Mode().IsRegular() && path.Ext(file.Name()) == ".go"
}

func checkOutput(srcFilename string, symbs []Symb, t *testing.T) {
	actualFilename := srcFilename + "_actual.json"
	expectedFilename := srcFilename + "_expected.json"

	// write actual output
	writeJson(actualFilename, symbsToJson(symbs))

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

func collectSymbs(pkg *ast.Package) (symbs []Symb) {
	c := NewContext()
	c.FileSet = fset
	c.Logf = func(pos token.Pos, f string, a ...interface{}) {
		if !verbose {
			return
		}
		log.Printf("%v: %s", c.position(pos), fmt.Sprintf(f, a...))
	}

	symbs = make([]Symb, 0)
	err := c.IterateSymbs(pkg, func(symb *Symb) bool {
		symbs = append(symbs, *symb)
		return true
	})
	if err != nil {
		panic("error iterating over symbols: " + err.Error())
	}
	return symbs
}

func (ctxt *Context) position(pos token.Pos) token.Position {
	return ctxt.FileSet.Position(pos)
}

func pp(symbs []Symb) string {
	s := "["
	for i, x := range symbs {
		if i > 0 {
			s += ", "
		}
		s += x.String()
	}
	return s + "]"
}

func symbsToJson(symbs []Symb) []interface{} {
	js := make([]interface{}, 0)
	for _, x := range symbs {
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

func prettys(symbs []Symb) string {
	s := "["
	for i, x := range symbs {
		if i > 0 {
			s += ", "
		}
		s += pretty(x.Expr)
	}
	return s + "]"
}
