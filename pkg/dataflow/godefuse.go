package dataflow

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

type scopeFrame struct {
	defs map[string]VarDef
}

type DefUseExtractor struct {
	variables  map[string]*Variable
	defs       []VarDef
	uses       []VarUse
	defRefs    map[string]string
	scopeStack []*scopeFrame
	pkg        string
	file       string
	fset       *token.FileSet
	funcID     string
	funcPkg    string
	funcFile   string
}

func ExtractDefUseFromSource(src, filePath, pkg string) ([]*DefUseChain, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}
	e := &DefUseExtractor{}
	return e.ExtractDefUse(f, fset, filePath, pkg)
}

func (e *DefUseExtractor) ExtractDefUse(astFile *ast.File, fset *token.FileSet, filePath, pkg string) ([]*DefUseChain, error) {
	e.fset = fset
	e.pkg = pkg
	e.file = filePath
	e.funcPkg = pkg
	e.funcFile = filePath
	e.variables = make(map[string]*Variable)
	e.defRefs = make(map[string]string)
	e.defs = nil
	e.uses = nil
	e.scopeStack = nil
	e.funcID = ""

	e.pushScope()

	ast.Inspect(astFile, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			e.processFuncDecl(node)
			return false
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					e.processGlobalValueSpec(vs)
				}
			}
		}
		return true
	})

	e.popScope()

	return e.buildChains(), nil
}

func (e *DefUseExtractor) pushScope() {
	e.scopeStack = append(e.scopeStack, &scopeFrame{defs: make(map[string]VarDef)})
}

func (e *DefUseExtractor) popScope() {
	e.scopeStack = e.scopeStack[:len(e.scopeStack)-1]
}

func (e *DefUseExtractor) addDef(name string, startLine, endLine int, scope ScopeType) VarDef {
	id := fmt.Sprintf("%s.%s:%d:%s", e.funcPkg, e.funcFile, startLine, name)
	def := VarDef{
		ID:        id,
		Variable:  name,
		Pkg:       e.funcPkg,
		File:      e.funcFile,
		StartLine: startLine,
		EndLine:   endLine,
	}
	e.defs = append(e.defs, def)

	if len(e.scopeStack) > 0 {
		e.scopeStack[len(e.scopeStack)-1].defs[name] = def
	}

	if _, exists := e.variables[name]; !exists {
		e.variables[name] = &Variable{
			Name:    name,
			Scope:   scope,
			Pkg:     e.funcPkg,
			File:    e.funcFile,
			DefLine: startLine,
		}
	}

	return def
}

func (e *DefUseExtractor) addUse(name string, line int, kind UseKind) {
	def := e.resolveIdent(name)
	if def == nil {
		return
	}

	useID := fmt.Sprintf("%s.%s:%d:%s:%d", e.funcPkg, e.funcFile, line, name, int(kind))
	use := VarUse{
		ID:       useID,
		Variable: name,
		File:     e.funcFile,
		Line:     line,
		Kind:     kind,
		FuncID:   e.funcID,
	}
	e.uses = append(e.uses, use)
	e.defRefs[useID] = def.ID
}

func (e *DefUseExtractor) resolveIdent(name string) *VarDef {
	for i := len(e.scopeStack) - 1; i >= 0; i-- {
		if def, ok := e.scopeStack[i].defs[name]; ok {
			return &def
		}
	}
	return nil
}

func (e *DefUseExtractor) processFuncDecl(fn *ast.FuncDecl) {
	e.funcID = fmt.Sprintf("%s.%s", e.pkg, fn.Name.Name)
	e.pushScope()

	if fn.Recv != nil {
		for _, field := range fn.Recv.List {
			for _, name := range field.Names {
				e.addDef(name.Name, e.fset.Position(name.Pos()).Line,
					e.fset.Position(name.Pos()).Line, ScopeParam)
			}
		}
	}

	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, name := range field.Names {
				e.addDef(name.Name, e.fset.Position(name.Pos()).Line,
					e.fset.Position(name.Pos()).Line, ScopeParam)
			}
		}
	}

	if fn.Body != nil {
		e.walkBody(fn.Body)
	}

	e.popScope()
}

func (e *DefUseExtractor) processGlobalValueSpec(vs *ast.ValueSpec) {
	line := e.fset.Position(vs.Pos()).Line
	for _, name := range vs.Names {
		e.addDef(name.Name, line, line, ScopeGlobal)
	}
	for _, val := range vs.Values {
		e.walkExpr(val, UseRead)
	}
}

func (e *DefUseExtractor) walkBody(body *ast.BlockStmt) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		e.walkStmt(stmt)
	}
}

func (e *DefUseExtractor) walkStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		e.walkAssignStmt(s)
	case *ast.RangeStmt:
		e.walkRangeStmt(s)
	case *ast.ReturnStmt:
		for _, result := range s.Results {
			e.walkExpr(result, UseReturn)
		}
	case *ast.BlockStmt:
		e.pushScope()
		e.walkBody(s)
		e.popScope()
	case *ast.ExprStmt:
		e.walkExpr(s.X, UseRead)
	case *ast.IfStmt:
		if s.Init != nil {
			e.walkStmt(s.Init)
		}
		if s.Cond != nil {
			e.walkExpr(s.Cond, UseRead)
		}
		e.walkBody(s.Body)
		if s.Else != nil {
			e.walkStmt(s.Else)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			e.walkStmt(s.Init)
		}
		if s.Cond != nil {
			e.walkExpr(s.Cond, UseRead)
		}
		if s.Post != nil {
			e.walkStmt(s.Post)
		}
		e.walkBody(s.Body)
	case *ast.SwitchStmt:
		if s.Init != nil {
			e.walkStmt(s.Init)
		}
		if s.Tag != nil {
			e.walkExpr(s.Tag, UseRead)
		}
		for _, cc := range s.Body.List {
			clause := cc.(*ast.CaseClause)
			for _, expr := range clause.List {
				e.walkExpr(expr, UseRead)
			}
			e.pushScope()
			for _, st := range clause.Body {
				e.walkStmt(st)
			}
			e.popScope()
		}
	case *ast.TypeSwitchStmt:
		if s.Init != nil {
			e.walkStmt(s.Init)
		}
		e.walkStmt(s.Assign)
		if s.Body != nil {
			for _, cc := range s.Body.List {
				clause := cc.(*ast.CaseClause)
				e.pushScope()
				for _, st := range clause.Body {
					e.walkStmt(st)
				}
				e.popScope()
			}
		}
	case *ast.DeclStmt:
		if decl, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range decl.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					e.walkValueSpec(vs)
				}
			}
		}
	case *ast.GoStmt:
		for _, arg := range s.Call.Args {
			e.walkExpr(arg, UseCallArg)
		}
	case *ast.DeferStmt:
		for _, arg := range s.Call.Args {
			e.walkExpr(arg, UseCallArg)
		}
	case *ast.SendStmt:
		e.walkExpr(s.Chan, UseRead)
		e.walkExpr(s.Value, UseRead)
	case *ast.IncDecStmt:
		if ident, ok := s.X.(*ast.Ident); ok {
			e.addUse(ident.Name, e.fset.Position(s.Pos()).Line, UseWrite)
		}
	case *ast.SelectStmt:
		for _, cc := range s.Body.List {
			clause := cc.(*ast.CommClause)
			if clause.Comm != nil {
				e.walkStmt(clause.Comm)
			}
			for _, st := range clause.Body {
				e.walkStmt(st)
			}
		}
	}
}

func (e *DefUseExtractor) walkAssignStmt(s *ast.AssignStmt) {
	line := e.fset.Position(s.Pos()).Line

	if s.Tok == token.DEFINE {
		for _, rhs := range s.Rhs {
			e.walkExpr(rhs, UseRead)
		}
		for _, lhs := range s.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				e.addDef(ident.Name, line, line, ScopeLocal)
			}
		}
	} else {
		for _, rhs := range s.Rhs {
			e.walkExpr(rhs, UseRead)
		}
		for _, lhs := range s.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				e.addUse(ident.Name, line, UseWrite)
			}
		}
	}
}

func (e *DefUseExtractor) walkRangeStmt(s *ast.RangeStmt) {
	line := e.fset.Position(s.Pos()).Line

	if s.Key != nil {
		if ident, ok := s.Key.(*ast.Ident); ok && ident.Name != "_" {
			e.addDef(ident.Name, line, line, ScopeLocal)
		}
	}
	if s.Value != nil {
		if ident, ok := s.Value.(*ast.Ident); ok && ident.Name != "_" {
			e.addDef(ident.Name, line, line, ScopeLocal)
		}
	}

	e.walkExpr(s.X, UseRead)
	e.walkBody(s.Body)
}

func (e *DefUseExtractor) walkValueSpec(vs *ast.ValueSpec) {
	line := e.fset.Position(vs.Pos()).Line
	for _, name := range vs.Names {
		e.addDef(name.Name, line, line, ScopeLocal)
	}
	for _, val := range vs.Values {
		e.walkExpr(val, UseRead)
	}
}

func (e *DefUseExtractor) walkExpr(expr ast.Expr, kind UseKind) {
	if expr == nil {
		return
	}
	switch ex := expr.(type) {
	case *ast.Ident:
		e.addUse(ex.Name, e.fset.Position(ex.Pos()).Line, kind)

	case *ast.SelectorExpr:
		e.walkExpr(ex.X, kind)

	case *ast.CallExpr:
		e.walkExpr(ex.Fun, UseRead)
		for _, arg := range ex.Args {
			e.walkExpr(arg, UseCallArg)
		}

	case *ast.BinaryExpr:
		e.walkExpr(ex.X, kind)
		e.walkExpr(ex.Y, kind)

	case *ast.UnaryExpr:
		e.walkExpr(ex.X, kind)

	case *ast.ParenExpr:
		e.walkExpr(ex.X, kind)

	case *ast.IndexExpr:
		e.walkExpr(ex.X, kind)
		if ex.Index != nil {
			e.walkExpr(ex.Index, kind)
		}

	case *ast.StarExpr:
		e.walkExpr(ex.X, kind)

	case *ast.CompositeLit:
		for _, elt := range ex.Elts {
			e.walkExpr(elt, kind)
		}

	case *ast.KeyValueExpr:
		e.walkExpr(ex.Key, kind)
		e.walkExpr(ex.Value, kind)

	case *ast.SliceExpr:
		e.walkExpr(ex.X, kind)
		if ex.Low != nil {
			e.walkExpr(ex.Low, kind)
		}
		if ex.High != nil {
			e.walkExpr(ex.High, kind)
		}
		if ex.Max != nil {
			e.walkExpr(ex.Max, kind)
		}

	case *ast.TypeAssertExpr:
		e.walkExpr(ex.X, kind)

	case *ast.FuncLit:
		e.pushScope()
		if ex.Type.Params != nil {
			for _, field := range ex.Type.Params.List {
				for _, name := range field.Names {
					e.addDef(name.Name, e.fset.Position(name.Pos()).Line,
						e.fset.Position(name.Pos()).Line, ScopeParam)
				}
			}
		}
		e.walkBody(ex.Body)
		e.popScope()

	case *ast.BasicLit:
	case *ast.ArrayType:
	case *ast.StructType:
	case *ast.MapType:
	case *ast.ChanType:
	case *ast.FuncType:
	case *ast.InterfaceType:
	}
}

func (e *DefUseExtractor) buildChains() []*DefUseChain {
	chainMap := make(map[string]*DefUseChain)
	for i := range e.defs {
		chainMap[e.defs[i].ID] = &DefUseChain{
			Def:  e.defs[i],
			Uses: nil,
		}
	}

	for _, use := range e.uses {
		if defID, ok := e.defRefs[use.ID]; ok {
			if chain, ok := chainMap[defID]; ok {
				chain.Uses = append(chain.Uses, use)
			}
		}
	}

	chains := make([]*DefUseChain, 0, len(chainMap))
	for _, chain := range chainMap {
		chains = append(chains, chain)
	}
	return chains
}
