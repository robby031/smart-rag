package indexer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

type DeclKind int

const (
	DeclUnknown DeclKind = iota
	DeclFunc
	DeclType
	DeclStruct
	DeclInterface
	DeclVar
	DeclConst
)

type ParsedDecl struct {
	Kind      DeclKind
	Name      string
	Signature string
	Content   string
	StartLine int
	EndLine   int
}

// FileInfo holds file-level metadata extracted during parsing.
type FileInfo struct {
	Package string
	Imports []string
	IsTest  bool
}

type Parser struct {
	fset *token.FileSet
}

func NewParser() *Parser {
	return &Parser{fset: token.NewFileSet()}
}

func (p *Parser) FileSet() *token.FileSet { return p.fset }

// ParseFile parses a file and returns the AST, declarations, and metadata.
func (p *Parser) ParseFile(filePath, src string) (*ast.File, []ParsedDecl, FileInfo, error) {
	f, err := parser.ParseFile(p.fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, nil, FileInfo{}, err
	}

	pkg := f.Name.Name
	isTest := strings.HasSuffix(filepath.Base(filePath), "_test.go")

	var imports []string
	for _, imp := range f.Imports {
		imports = append(imports, strings.Trim(imp.Path.Value, "\"`"))
	}

	var decls []ParsedDecl
	for _, d := range f.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			decls = append(decls, p.parseFuncDecl(decl))
		case *ast.GenDecl:
			parsed := p.parseGenDecl(decl)
			decls = append(decls, parsed...)
		}
	}

	return f, decls, FileInfo{Package: pkg, Imports: imports, IsTest: isTest}, nil
}

func (p *Parser) parseFuncDecl(d *ast.FuncDecl) ParsedDecl {
	start := p.fset.Position(d.Pos())
	end := p.fset.Position(d.End())

	var sig strings.Builder
	sig.WriteString("func ")
	if d.Recv != nil {
		sig.WriteString("(")
		for i, f := range d.Recv.List {
			if i > 0 {
				sig.WriteString(", ")
			}
			sig.WriteString(exprString(f.Type))
		}
		sig.WriteString(") ")
	}
	sig.WriteString(d.Name.Name)
	sig.WriteString(exprString(d.Type))

	return ParsedDecl{
		Kind:      DeclFunc,
		Name:      d.Name.Name,
		Signature: sig.String(),
		StartLine: start.Line,
		EndLine:   end.Line,
	}
}

func (p *Parser) parseGenDecl(d *ast.GenDecl) []ParsedDecl {
	var decls []ParsedDecl

	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			start := p.fset.Position(s.Pos())
			end := p.fset.Position(s.End())

			kind := DeclType
			switch s.Type.(type) {
			case *ast.StructType:
				kind = DeclStruct
			case *ast.InterfaceType:
				kind = DeclInterface
			}

			decls = append(decls, ParsedDecl{
				Kind:      kind,
				Name:      s.Name.Name,
				Signature: exprString(s.Type),
				StartLine: start.Line,
				EndLine:   end.Line,
			})
		case *ast.ValueSpec:
			for _, name := range s.Names {
				start := p.fset.Position(s.Pos())
				end := p.fset.Position(s.End())
				kind := DeclVar
				if d.Tok == token.CONST {
					kind = DeclConst
				}
				decls = append(decls, ParsedDecl{
					Kind:      kind,
					Name:      name.Name,
					StartLine: start.Line,
					EndLine:   end.Line,
				})
			}
		}
	}
	return decls
}

func SetContent(decls []ParsedDecl, src string) {
	lines := strings.Split(src, "\n")
	for i, d := range decls {
		if d.StartLine > 0 && d.EndLine > 0 && d.StartLine <= len(lines) && d.EndLine <= len(lines) {
			decls[i].Content = strings.Join(lines[d.StartLine-1:d.EndLine], "\n")
		}
	}
}

func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprString(t.Elt)
	case *ast.MapType:
		return "map[" + exprString(t.Key) + "]" + exprString(t.Value)
	case *ast.FuncType:
		params := t.Params
		results := t.Results
		var b strings.Builder
		b.WriteString("(")
		if params != nil {
			for i, f := range params.List {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(exprString(f.Type))
			}
		}
		b.WriteString(")")
		if results != nil && len(results.List) > 0 {
			b.WriteString(" ")
			if len(results.List) > 1 {
				b.WriteString("(")
			}
			for i, f := range results.List {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(exprString(f.Type))
			}
			if len(results.List) > 1 {
				b.WriteString(")")
			}
		}
		return b.String()
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "?"
	}
}

func IsExported(name string) bool {
	if name == "" {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}
