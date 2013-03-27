// The go/xref package provides a way to iterate over the
// cross-references in Go source files. It is copied from rog-go's
// go/sym and adds the following features:
//
// * generates a unique, meaningful identifier for each xref
// * establishes cross-references even when there is no xref
// * test coverage
package xref

import (
	"bytes"
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/parser"
	"code.google.com/p/rog-go/exp/go/printer"
	"code.google.com/p/rog-go/exp/go/token"
	"code.google.com/p/rog-go/exp/go/types"
	"go/build"
	"os"
	"path/filepath"
	"sync"
)

// Xref holds information about an xref.
type Xref struct {
	Pos      token.Pos   // position of xref.
	Expr     ast.Expr    // expression for xref (*ast.Ident or *ast.SelectorExpr)
	Ident    *ast.Ident  // identifier in parse tree
	ExprType types.Type  // type of expression.
	ReferPos token.Pos   // position of referred-to thing.
	ReferObj *ast.Object // object referred to.
	Local    bool        // whether referred-to object is function-local.
	Universe bool        // whether referred-to object is in universe.
}

// Context holds the context for IterateXrefs.
type Context struct {
	pkgMutex sync.Mutex
	pkgCache map[string]*ast.Package
	importer types.Importer

	// FileSet holds the fileset used when importing packages.
	FileSet *token.FileSet

	// Logf is used to print warning messages.
	// If it is nil, no warning messages will be printed.
	Logf func(pos token.Pos, f string, a ...interface{})
}

func NewContext() *Context {
	ctxt := &Context{
		pkgCache: make(map[string]*ast.Package),
		FileSet:  token.NewFileSet(),
	}
	ctxt.importer = ctxt.importerFunc()
	return ctxt
}

// Import imports and parses the package with the given path.
// It returns nil if it fails.
func (ctxt *Context) Import(path string) *ast.Package {
	// TODO return error.
	return ctxt.importer(path)
}

func (ctxt *Context) importerFunc() types.Importer {
	return func(path string) *ast.Package {
		ctxt.pkgMutex.Lock()
		defer ctxt.pkgMutex.Unlock()
		if pkg := ctxt.pkgCache[path]; pkg != nil {
			return pkg
		}
		cwd, _ := os.Getwd() // TODO put this into Context?
		bpkg, err := build.Import(path, cwd, 0)
		if err != nil {
			ctxt.logf(token.NoPos, "cannot find %q: %v", path, err)
			return nil
		}
		// Relative paths can have several names
		if pkg := ctxt.pkgCache[bpkg.ImportPath]; pkg != nil {
			ctxt.pkgCache[path] = pkg
			return pkg
		}
		var files []string
		files = append(files, bpkg.GoFiles...)
		files = append(files, bpkg.CgoFiles...)
		files = append(files, bpkg.TestGoFiles...)
		for i, f := range files {
			files[i] = filepath.Join(bpkg.Dir, f)
		}
		pkgs, err := parser.ParseFiles(ctxt.FileSet, files, parser.ParseComments)
		if len(pkgs) == 0 {
			ctxt.logf(token.NoPos, "cannot parse package %q: %v", path, err)
			return nil
		}
		delete(pkgs, "documentation")
		for _, pkg := range pkgs {
			if ctxt.pkgCache[path] == nil {
				ctxt.pkgCache[path] = pkg
				if path != bpkg.ImportPath {
					ctxt.pkgCache[bpkg.ImportPath] = pkg
				}
			} else {
				ctxt.logf(token.NoPos, "unexpected extra package %q in %q", pkg.Name, path)
			}
		}
		return ctxt.pkgCache[path]
	}
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

func (ctxt *Context) visitExpr(f *ast.File, e ast.Expr, local bool, visitf func(*Xref) bool) bool {
	var xref Xref
	xref.Expr = e
	switch e := e.(type) {
	case *ast.Ident:
		if e.Name == "_" {
			return true
		}
		xref.Pos = e.Pos()
		xref.Ident = e
	case *ast.SelectorExpr:
		xref.Pos = e.Sel.Pos()
		xref.Ident = e.Sel
	}
	obj, t := types.ExprType(e, ctxt.importer)
	if obj == nil {
		ctxt.logf(e.Pos(), "no object for %s", pretty(e))
		return true
	}
	xref.ExprType = t
	xref.ReferObj = obj
	if parser.Universe.Lookup(obj.Name) != obj {
		xref.ReferPos = types.DeclPos(obj)
		if xref.ReferPos == token.NoPos {
			name := pretty(e)
			if name != "init" {
				ctxt.logf(e.Pos(), "no declaration for %s", pretty(e))
			}
			return true
		}
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
