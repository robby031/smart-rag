package indexer

import (
	"fmt"
	"slices"
	"strings"
)

type ChunkType int

const (
	ChunkFile ChunkType = iota
	ChunkPackage
	ChunkImport
	ChunkTypeDecl
	ChunkFunc
	ChunkStruct
	ChunkInterface
	ChunkBlock
	ChunkClass
	ChunkEnum
)

type Chunk struct {
	ID            string        `json:"id"`
	FilePath      string        `json:"file_path"`
	ChunkType     ChunkType     `json:"chunk_type"`
	StartLine     int           `json:"start_line"`
	EndLine       int           `json:"end_line"`
	Content       string        `json:"content"`
	SymbolName    string        `json:"symbol_name,omitempty"`
	Signature     string        `json:"signature,omitempty"`
	Package       string        `json:"package"`
	Imports       []string      `json:"imports,omitempty"`
	TestedSymbols []string      `json:"tested_symbols,omitempty"`
	Variables     []VariableRef `json:"variables,omitempty"`
	Types         []string      `json:"types,omitempty"`
}

type FileMeta struct {
	Package string
	Imports []string
	IsTest  bool
}

type Chunker struct {
	maxTokens int
}

func NewChunker(maxTokens int) *Chunker {
	if maxTokens <= 0 {
		maxTokens = 512
	}
	return &Chunker{maxTokens: maxTokens}
}

func (c *Chunker) Chunk(decls []ParsedDecl, filePath string, meta FileMeta) []Chunk {
	return c.ChunkWithVars(decls, filePath, meta, nil, "")
}

func (c *Chunker) ChunkWithVars(decls []ParsedDecl, filePath string, meta FileMeta, ve *VariableExtractor, src string) []Chunk {
	var chunks []Chunk
	tested := extractTestedSymbols(decls)

	for _, d := range decls {
		tokens := tokenizeCode(d.Content)
		if len(tokens) > c.maxTokens {
			sub := c.splitLargeDecl(d, filePath, meta, tested)
			chunks = append(chunks, sub...)
			continue
		}
		chunks = append(chunks, c.makeChunk(d, filePath, meta, tested, d.Content, tokens))
	}

	if len(chunks) == 0 {
		chunks = append(chunks, Chunk{
			ID:        filePath,
			FilePath:  filePath,
			ChunkType: ChunkFile,
			Package:   meta.Package,
			Imports:   meta.Imports,
		})
	}

	if ve != nil && src != "" {
		for i, ch := range chunks {
			for _, d := range decls {
				if d.Name == ch.SymbolName && d.StartLine == ch.StartLine {
					vars := ve.ExtractVariables(d, src, meta.Package)
					chunks[i].Variables = vars
					for _, v := range vars {
						if v.Type != "" {
							chunks[i].Types = addUnique(chunks[i].Types, v.Type)
						}
					}
					if len(chunks[i].Types) == 0 {
						chunks[i].Types = nil
					}
					break
				}
			}
		}
	}

	return chunks
}

func (c *Chunker) makeChunk(d ParsedDecl, filePath string, meta FileMeta, tested []string, content string, _ []string) Chunk {
	return Chunk{
		ID:            fmt.Sprintf("%s:%d-%d", filePath, d.StartLine, d.EndLine),
		FilePath:      filePath,
		ChunkType:     mapDeclKind(d.Kind),
		StartLine:     d.StartLine,
		EndLine:       d.EndLine,
		Content:       content,
		SymbolName:    d.Name,
		Signature:     d.Signature,
		Package:       meta.Package,
		Imports:       meta.Imports,
		TestedSymbols: tested,
	}
}

func (c *Chunker) splitLargeDecl(d ParsedDecl, filePath string, meta FileMeta, tested []string) []Chunk {
	lines := strings.Split(d.Content, "\n")
	base := d.StartLine
	var chunks []Chunk

	depth := 0
	buf := &strings.Builder{}
	tokCount := 0
	subStart := base

	flush := func(end int) {
		if buf.Len() == 0 {
			return
		}
		content := strings.TrimRight(buf.String(), "\n")
		chunks = append(chunks, Chunk{
			ID:            fmt.Sprintf("%s:%d-%d", filePath, subStart, end),
			FilePath:      filePath,
			ChunkType:     ChunkBlock,
			StartLine:     subStart,
			EndLine:       end,
			Content:       content,
			Package:       meta.Package,
			Imports:       meta.Imports,
			TestedSymbols: tested,
			SymbolName:    d.Name,
		})
		buf.Reset()
		tokCount = 0
		subStart = end + 1
	}

	for i, line := range lines {
		prevDepth := depth
		for _, r := range line {
			switch r {
			case '{':
				depth++
			case '}':
				depth--
			}
		}

		lineTokens := tokenizeCode(line)
		willExceed := tokCount+len(lineTokens) > c.maxTokens

		if willExceed && (prevDepth == 0 || depth == 0) && buf.Len() > 0 {
			flush(base + i)
		}
		if willExceed && buf.Len() == 0 {
			chunks = append(chunks, Chunk{
				ID:            fmt.Sprintf("%s:%d-%d", filePath, base+i, base+i),
				FilePath:      filePath,
				ChunkType:     ChunkBlock,
				StartLine:     base + i,
				EndLine:       base + i,
				Content:       line,
				Package:       meta.Package,
				Imports:       meta.Imports,
				TestedSymbols: tested,
			})
			continue
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
		tokCount += len(lineTokens)
	}

	flush(d.EndLine)
	return chunks
}

func mapDeclKind(kind DeclKind) ChunkType {
	switch kind {
	case DeclFunc:
		return ChunkFunc
	case DeclType:
		return ChunkTypeDecl
	case DeclEnum:
		return ChunkEnum
	case DeclStruct:
		return ChunkStruct
	case DeclClass:
		return ChunkClass
	case DeclInterface:
		return ChunkInterface
	default:
		return ChunkFunc
	}
}

func extractTestedSymbols(decls []ParsedDecl) []string {
	var out []string
	for _, d := range decls {
		if d.Kind != DeclFunc {
			continue
		}
		name := d.Name
		if !strings.HasPrefix(name, "Test") {
			continue
		}
		rest := strings.TrimPrefix(name, "Test")
		if rest == "" {
			continue
		}
		if idx := strings.Index(rest, "_"); idx > 0 {
			symbol := rest[:idx]
			out = append(out, symbol)
			sub := rest[idx+1:]
			if sub != "" && sub[0] >= 'A' && sub[0] <= 'Z' {
				out = append(out, symbol+"."+sub)
			}
		} else {
			out = append(out, rest)
		}
	}
	return out
}

func addUnique(slice []string, item string) []string {
	if slices.Contains(slice, item) {
		return slice
	}
	return append(slice, item)
}
