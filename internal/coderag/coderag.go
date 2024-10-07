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

// FunctionInfo stores information about a function, method, or variable.
type FunctionInfo struct {
	Name        string
	FilePath    string
	CalledBy    map[string]struct{}
	Calls       map[string]struct{}
	LineNumber  int
	Node        ast.Node
	DirectCalls []string
	Package     string
	Type        string // e.g., function, method, variable
}

// CodeIndex stores the indexed information about the codebase.
type CodeIndex struct {
	Functions map[string]*FunctionInfo
	Files     map[string][]*FunctionInfo
	Packages  map[string]map[string]struct{}
	fset      *token.FileSet
	mu        sync.RWMutex // For concurrent access
}

// NewCodeIndex creates a new instance of CodeIndex.
func NewCodeIndex() *CodeIndex {
	return &CodeIndex{
		Functions: make(map[string]*FunctionInfo),
		Files:     make(map[string][]*FunctionInfo),
		Packages:  make(map[string]map[string]struct{}),
		fset:      token.NewFileSet(),
	}
}

// IndexRepository walks through the repository and indexes all Go files for function relationships.
func (idx *CodeIndex) IndexRepository(repoPath string) error {
	// First pass: collect all function and variable declarations.
	if err := filepath.Walk(repoPath, idx.indexDeclarations); err != nil {
		return err
	}

	// Second pass: analyze function calls and relationships.
	if err := filepath.Walk(repoPath, idx.indexCallRelationships); err != nil {
		return err
	}

	return nil
}

// indexDeclarations processes each file to extract function and variable declarations.
func (idx *CodeIndex) indexDeclarations(path string, info os.FileInfo, err error) error {
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
	idx.mu.Lock()
	if _, exists := idx.Packages[packageName]; !exists {
		idx.Packages[packageName] = make(map[string]struct{})
	}
	idx.mu.Unlock()

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
}

// indexCallRelationships processes each file to analyze call expressions within functions.
func (idx *CodeIndex) indexCallRelationships(path string, info os.FileInfo, err error) error {
	if err != nil || !strings.HasSuffix(path, ".go") || strings.Contains(path, "vendor/") {
		return err
	}

	file, err := parser.ParseFile(idx.fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %v", path, err)
	}

	var currentFunc *FunctionInfo

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			funcName := idx.getFunctionName(node)
			idx.mu.RLock()
			currentFunc = idx.Functions[funcName]
			idx.mu.RUnlock()
		case *ast.CallExpr:
			idx.analyzeCallExpr(node, currentFunc)
		}
		return true
	})

	return nil
}

// extractFunction extracts function/method declarations from the AST.
func (idx *CodeIndex) extractFunction(fn *ast.FuncDecl, path, packageName string) {
	funcName := idx.getFunctionName(fn)
	position := idx.fset.Position(fn.Pos())

	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.Functions[funcName] = &FunctionInfo{
		Name:        funcName,
		FilePath:    path,
		CalledBy:    make(map[string]struct{}),
		Calls:       make(map[string]struct{}),
		LineNumber:  position.Line,
		Node:        fn,
		DirectCalls: []string{},
		Package:     packageName,
		Type:        "function",
	}

	idx.Files[path] = append(idx.Files[path], idx.Functions[funcName])

	if ast.IsExported(fn.Name.Name) {
		idx.Packages[packageName][funcName] = struct{}{}
	}
}

// extractVariables extracts variable declarations from the AST.
func (idx *CodeIndex) extractVariables(genDecl *ast.GenDecl, path, packageName string) {
	for _, spec := range genDecl.Specs {
		if valueSpec, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range valueSpec.Names {
				position := idx.fset.Position(name.Pos())

				idx.mu.Lock()
				idx.Functions[name.Name] = &FunctionInfo{
					Name:       name.Name,
					FilePath:   path,
					CalledBy:   make(map[string]struct{}), // Initialize maps to prevent nil map assignments
					Calls:      make(map[string]struct{}),
					LineNumber: position.Line,
					Type:       "variable",
					Package:    packageName,
				}
				idx.Files[path] = append(idx.Files[path], idx.Functions[name.Name])
				idx.mu.Unlock()
			}
		}
	}
}

// getFunctionName returns the fully qualified function name (including receiver if any).
func (idx *CodeIndex) getFunctionName(fn *ast.FuncDecl) string {
	funcName := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recvType := ""
		switch t := fn.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if ident, ok := t.X.(*ast.Ident); ok {
				recvType = ident.Name
			}
		case *ast.Ident:
			recvType = t.Name
		}
		if recvType != "" {
			funcName = recvType + "." + funcName
		}
	}
	return funcName
}

// analyzeCallExpr processes the CallExpr AST node to gather call relationships between functions.
func (idx *CodeIndex) analyzeCallExpr(callExpr *ast.CallExpr, currentFunc *FunctionInfo) {
	if currentFunc == nil || currentFunc.Type != "function" {
		return
	}

	calledFunc, isMethodCall := idx.resolveCalledFunction(callExpr)

	if calledFunc != "" {
		// Record the call in DirectCalls and Calls map
		idx.mu.Lock()
		currentFunc.DirectCalls = append(currentFunc.DirectCalls, calledFunc)
		if !isMethodCall {
			currentFunc.Calls[calledFunc] = struct{}{}
			if called, exists := idx.Functions[calledFunc]; exists {
				called.CalledBy[currentFunc.Name] = struct{}{}
			}
		}
		idx.mu.Unlock()
	}

	// Recursively analyze arguments for nested calls
	for _, arg := range callExpr.Args {
		if nestedCall, ok := arg.(*ast.CallExpr); ok {
			idx.analyzeCallExpr(nestedCall, currentFunc)
		}
	}
}

// resolveCalledFunction determines the name of the called function and whether it's a method call.
func (idx *CodeIndex) resolveCalledFunction(callExpr *ast.CallExpr) (string, bool) {
	var calledFunc string
	isMethodCall := false

	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		// Direct function call
		calledFunc = fn.Name
	case *ast.SelectorExpr:
		// Method call or package function
		switch x := fn.X.(type) {
		case *ast.Ident:
			// Could be either package.Function or variable.Method
			if _, isPackage := idx.Packages[x.Name]; isPackage {
				calledFunc = x.Name + "." + fn.Sel.Name
			} else {
				calledFunc = fn.Sel.Name
				isMethodCall = true
			}
		}
	}

	return calledFunc, isMethodCall
}

// GetFunctionInfo retrieves information about a specific function or variable by name.
func (idx *CodeIndex) GetFunctionInfo(funcName string) (*FunctionInfo, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if info, exists := idx.Functions[funcName]; exists {
		return info, nil
	}
	return nil, fmt.Errorf("function or variable %s not found", funcName)
}

// GetFunctionsByFile retrieves all functions and variables defined in a specific file.
func (idx *CodeIndex) GetFunctionsByFile(filePath string) ([]*FunctionInfo, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if funcs, exists := idx.Files[filePath]; exists {
		return funcs, nil
	}
	return nil, fmt.Errorf("no functions found in file %s", filePath)
}

// GetRelatedFunctions retrieves functions related to a specific function through calls or being called by.
func (idx *CodeIndex) GetRelatedFunctions(funcName string) ([]*FunctionInfo, error) {
	info, err := idx.GetFunctionInfo(funcName)
	if err != nil {
		return nil, err
	}

	related := make(map[string]*FunctionInfo)

	// Functions that this function calls
	for called := range info.Calls {
		if calledInfo, exists := idx.Functions[called]; exists {
			related[called] = calledInfo
		}
	}

	// Functions that call this function
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

// PrintFunctionSource prints the source code of a function or variable by name.
func (idx *CodeIndex) PrintFunctionSource(funcName string) error {
	info, err := idx.GetFunctionInfo(funcName)
	if err != nil {
		return err
	}

	fmt.Printf("// %s: %s\n", strings.Title(info.Type), funcName)
	fmt.Printf("// Package: %s\n", info.Package)
	fmt.Printf("// File: %s\n", info.FilePath)
	fmt.Printf("// Line: %d\n\n", info.LineNumber)

	cfg := printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 4,
	}

	if err := cfg.Fprint(os.Stdout, idx.fset, info.Node); err != nil {
		return fmt.Errorf("failed to print source for %s: %v", funcName, err)
	}
	fmt.Println()

	return nil
}

// PrintCallTree prints the function call tree for a function starting at the provided depth.
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
		fmt.Printf("%s%s (unknown function)\n", strings.Repeat("  ", depth), funcName)
		return
	}

	indent := strings.Repeat("  ", depth)
	for _, calledFunc := range info.DirectCalls {
		if calledInfo, exists := idx.Functions[calledFunc]; exists {
			fmt.Printf("%s→ %s (internal, Line: %d)\n", indent, calledFunc, calledInfo.LineNumber)
			idx.PrintCallTree(calledFunc, depth+1, visited)
		} else {
			fmt.Printf("%s→ %s (external/library)\n", indent, calledFunc)
		}
	}
}

// GetFilesCallingFunction retrieves all file paths that invoke the specified function.
func (idx *CodeIndex) GetFilesCallingFunction(funcName string) ([]string, error) {
	info, err := idx.GetFunctionInfo(funcName)
	if err != nil {
		return nil, err
	}

	fileSet := make(map[string]struct{})
	for caller := range info.CalledBy {
		if callerInfo, exists := idx.Functions[caller]; exists {
			fileSet[callerInfo.FilePath] = struct{}{}
		}
	}

	files := make([]string, 0, len(fileSet))
	for file := range fileSet {
		files = append(files, file)
	}

	return files, nil
}

// GetFilesCalledByFunction retrieves all file paths that are invoked by the specified function.
func (idx *CodeIndex) GetFilesCalledByFunction(funcName string) ([]string, error) {
	info, err := idx.GetFunctionInfo(funcName)
	if err != nil {
		return nil, err
	}

	fileSet := make(map[string]struct{})
	for called := range info.Calls {
		if calledInfo, exists := idx.Functions[called]; exists {
			fileSet[calledInfo.FilePath] = struct{}{}
		}
	}

	files := make([]string, 0, len(fileSet))
	for file := range fileSet {
		files = append(files, file)
	}

	return files, nil
}
