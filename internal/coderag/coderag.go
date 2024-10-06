package coderag

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FunctionInfo stores information about a function, method, or variable
type FunctionInfo struct {
	Name        string
	FilePath    string
	CalledBy    map[string]struct{}
	Calls       map[string]struct{}
	LineNumber  int
	Node        *ast.FuncDecl
	DirectCalls []string
	Package     string
	Type        string // e.g., function, method, variable
}

// CodeIndex stores the indexed information about the codebase
type CodeIndex struct {
	Functions map[string]*FunctionInfo
	fset      *token.FileSet
	Packages  map[string]map[string]struct{}
	mu        sync.RWMutex // For concurrent access
}

// NewCodeIndex creates a new instance of CodeIndex
func NewCodeIndex() *CodeIndex {
	return &CodeIndex{
		Functions: make(map[string]*FunctionInfo),
		fset:      token.NewFileSet(),
		Packages:  make(map[string]map[string]struct{}),
	}
}

// analyzeCallExpr processes the CallExpr AST node to gather call relationships between functions
func (idx *CodeIndex) analyzeCallExpr(callExpr *ast.CallExpr, currentFunc *FunctionInfo) {
	if currentFunc == nil {
		return
	}

	var calledFunc string
	var isMethodCall bool

	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		// Direct function call
		calledFunc = fn.Name
	case *ast.SelectorExpr:
		// Method call or package function
		switch x := fn.X.(type) {
		case *ast.Ident:
			// Could be either package.Function or variable.Method
			calledFunc = fn.Sel.Name
			isMethodCall = true
			if _, exists := idx.Packages[x.Name]; exists {
				// Package function call
				calledFunc = x.Name + "." + calledFunc
				isMethodCall = false
			}
		}
	}

	if calledFunc != "" {
		// Record the call in both DirectCalls and Calls map
		currentFunc.DirectCalls = append(currentFunc.DirectCalls, calledFunc)
		if !isMethodCall {
			idx.mu.Lock()
			currentFunc.Calls[calledFunc] = struct{}{}
			if called, exists := idx.Functions[calledFunc]; exists {
				called.CalledBy[currentFunc.Name] = struct{}{}
			}
			idx.mu.Unlock()
		}
	}

	// Recursively analyze arguments for nested calls
	for _, arg := range callExpr.Args {
		if nested, ok := arg.(*ast.CallExpr); ok {
			idx.analyzeCallExpr(nested, currentFunc)
		}
	}
}

// IndexRepository walks through the repository and indexes all Go files for function relationships
func (idx *CodeIndex) IndexRepository(repoPath string) error {
	// First pass: collect all function declarations and their packages
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".go") || strings.Contains(path, "vendor/") {
			return nil
		}

		file, err := parser.ParseFile(idx.fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %v", path, err)
		}

		packageName := file.Name.Name
		if _, exists := idx.Packages[packageName]; !exists {
			idx.Packages[packageName] = make(map[string]struct{})
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				idx.extractFunction(node, path, packageName)
			case *ast.GenDecl:
				idx.extractVariables(node, path, packageName)
			}
			return true
		})

		return nil
	})

	if err != nil {
		return err
	}

	// Second pass: analyze function calls and relationships
	return filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(path, ".go") || strings.Contains(path, "vendor/") {
			return err
		}

		file, err := parser.ParseFile(idx.fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		var currentFunc *FunctionInfo

		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				funcName := idx.getFunctionName(x)
				idx.mu.RLock()
				currentFunc = idx.Functions[funcName]
				idx.mu.RUnlock()

			case *ast.CallExpr:
				idx.analyzeCallExpr(x, currentFunc)
			}
			return true
		})

		return nil
	})
}

// extractFunction extracts function/method declarations from the AST
func (idx *CodeIndex) extractFunction(fn *ast.FuncDecl, path, packageName string) {
	funcName := idx.getFunctionName(fn)
	position := idx.fset.Position(fn.Pos())

	idx.mu.Lock()
	idx.Functions[funcName] = &FunctionInfo{
		Name:        funcName,
		FilePath:    path,
		CalledBy:    make(map[string]struct{}),
		Calls:       make(map[string]struct{}),
		LineNumber:  position.Line,
		Node:        fn,
		DirectCalls: make([]string, 0),
		Package:     packageName,
		Type:        "function",
	}
	if ast.IsExported(funcName) {
		idx.Packages[packageName][funcName] = struct{}{}
	}
	idx.mu.Unlock()
}

// extractVariables extracts variable declarations from the AST
func (idx *CodeIndex) extractVariables(genDecl *ast.GenDecl, path, packageName string) {
	for _, spec := range genDecl.Specs {
		if valueSpec, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range valueSpec.Names {
				position := idx.fset.Position(name.Pos())
				idx.mu.Lock()
				idx.Functions[name.Name] = &FunctionInfo{
					Name:       name.Name,
					FilePath:   path,
					LineNumber: position.Line,
					Type:       "variable",
					Package:    packageName,
				}
				idx.mu.Unlock()
			}
		}
	}
}

// getFunctionName returns the fully qualified function name (including receiver)
func (idx *CodeIndex) getFunctionName(fn *ast.FuncDecl) string {
	funcName := fn.Name.Name
	if fn.Recv != nil {
		if len(fn.Recv.List) > 0 {
			switch t := fn.Recv.List[0].Type.(type) {
			case *ast.StarExpr:
				if ident, ok := t.X.(*ast.Ident); ok {
					return ident.Name + "." + funcName
				}
			case *ast.Ident:
				return t.Name + "." + funcName
			}
		}
	}
	return funcName
}

// GetFunctionInfo retrieves information about a specific function or variable by name
func (idx *CodeIndex) GetFunctionInfo(funcName string) (*FunctionInfo, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if info, exists := idx.Functions[funcName]; exists {
		return info, nil
	}
	return nil, fmt.Errorf("function %s not found", funcName)
}

// GetRelatedFunctions retrieves functions related to a specific function through calls or being called by
func (idx *CodeIndex) GetRelatedFunctions(funcName string) ([]*FunctionInfo, error) {
	info, err := idx.GetFunctionInfo(funcName)
	if err != nil {
		return nil, err
	}

	related := make(map[string]*FunctionInfo)

	// Add functions that this function calls
	for called := range info.Calls {
		if calledInfo, exists := idx.Functions[called]; exists {
			related[called] = calledInfo
		}
	}

	// Add functions that call this function
	for caller := range info.CalledBy {
		if callerInfo, exists := idx.Functions[caller]; exists {
			related[caller] = callerInfo
		}
	}

	result := make([]*FunctionInfo, 0, len(related))
	for _, relInfo := range related {
		result = append(result, relInfo)
	}

	return result, nil
}

// PrintFunctionSource prints the source code of a function or variable by name
func (idx *CodeIndex) PrintFunctionSource(funcName string) error {
	info, err := idx.GetFunctionInfo(funcName)
	if err != nil {
		return err
	}

	fmt.Printf("// Function: %s\n", funcName)
	fmt.Printf("// Package: %s\n", info.Package)
	fmt.Printf("// File: %s\n", info.FilePath)
	fmt.Printf("// Line: %d\n\n", info.LineNumber)

	cfg := printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 4,
	}

	if err := cfg.Fprint(os.Stdout, idx.fset, info.Node); err != nil {
		return fmt.Errorf("failed to print function source: %v", err)
	}
	fmt.Println()

	return nil
}

// PrintCallTree prints the function call tree for a function starting at the provided depth
func (idx *CodeIndex) PrintCallTree(funcName string, depth int, visited map[string]bool) {
	if visited == nil {
		visited = make(map[string]bool)
	}

	if visited[funcName] {
		fmt.Printf("%s↺ %s (recursive call)\n", strings.Repeat("  ", depth), funcName)
		return
	}
	visited[funcName] = true

	info, exists := idx.Functions[funcName]
	if !exists {
		return
	}

	indent := strings.Repeat("  ", depth)
	for _, calledFunc := range info.DirectCalls {
		if calledInfo, exists := idx.Functions[calledFunc]; exists {
			fmt.Printf("%s→ %s (internal function, Line: %d)\n", indent, calledFunc, calledInfo.LineNumber)
			idx.PrintCallTree(calledFunc, depth+1, visited)
		} else {
			fmt.Printf("%s→ %s (external/library call)\n", indent, calledFunc)
		}
	}
}
