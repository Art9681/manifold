package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// FunctionInfo stores information about a function
type FunctionInfo struct {
	Name        string
	FilePath    string
	CalledBy    map[string]struct{}
	Calls       map[string]struct{}
	LineNumber  int
	Node        *ast.FuncDecl
	DirectCalls []string
	Package     string
}

// CodeIndex stores the indexed information about the codebase
type CodeIndex struct {
	Functions map[string]*FunctionInfo
	fset      *token.FileSet
	Packages  map[string]map[string]struct{}
}

// NewCodeIndex creates a new instance of CodeIndex
func NewCodeIndex() *CodeIndex {
	return &CodeIndex{
		Functions: make(map[string]*FunctionInfo),
		fset:      token.NewFileSet(),
		Packages:  make(map[string]map[string]struct{}),
	}
}

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
		case *ast.CallExpr:
			// Handle chained calls like a.B().C()
			calledFunc = fn.Sel.Name
			isMethodCall = true
		case *ast.SelectorExpr:
			// Handle nested selectors like a.b.Method()
			calledFunc = fn.Sel.Name
			isMethodCall = true
		}
	}

	if calledFunc != "" {
		// Record the call in both DirectCalls and Calls map
		currentFunc.DirectCalls = append(currentFunc.DirectCalls, calledFunc)
		if !isMethodCall {
			currentFunc.Calls[calledFunc] = struct{}{}
			if called, exists := idx.Functions[calledFunc]; exists {
				called.CalledBy[currentFunc.Name] = struct{}{}
			}
		}
	}

	// Recursively analyze arguments for nested calls
	for _, arg := range callExpr.Args {
		if nested, ok := arg.(*ast.CallExpr); ok {
			idx.analyzeCallExpr(nested, currentFunc)
		}
	}
}

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
			if fn, ok := n.(*ast.FuncDecl); ok {
				funcName := fn.Name.Name
				fullName := funcName
				if fn.Recv != nil {
					if len(fn.Recv.List) > 0 {
						switch t := fn.Recv.List[0].Type.(type) {
						case *ast.StarExpr:
							if ident, ok := t.X.(*ast.Ident); ok {
								fullName = ident.Name + "." + funcName
							}
						case *ast.Ident:
							fullName = t.Name + "." + funcName
						}
					}
				}

				position := idx.fset.Position(fn.Pos())
				idx.Functions[fullName] = &FunctionInfo{
					Name:        fullName,
					FilePath:    path,
					CalledBy:    make(map[string]struct{}),
					Calls:       make(map[string]struct{}),
					LineNumber:  position.Line,
					Node:        fn,
					DirectCalls: make([]string, 0),
					Package:     packageName,
				}

				if ast.IsExported(funcName) {
					idx.Packages[packageName][fullName] = struct{}{}
				}
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
				funcName := x.Name.Name
				if x.Recv != nil {
					if len(x.Recv.List) > 0 {
						switch t := x.Recv.List[0].Type.(type) {
						case *ast.StarExpr:
							if ident, ok := t.X.(*ast.Ident); ok {
								funcName = ident.Name + "." + funcName
							}
						case *ast.Ident:
							funcName = t.Name + "." + funcName
						}
					}
				}
				var exists bool
				currentFunc, exists = idx.Functions[funcName]
				if !exists {
					currentFunc = nil
				}

			case *ast.CallExpr:
				idx.analyzeCallExpr(x, currentFunc)
			}
			return true
		})

		return nil
	})
}

func (idx *CodeIndex) GetFunctionInfo(funcName string) (*FunctionInfo, error) {
	if info, exists := idx.Functions[funcName]; exists {
		return info, nil
	}
	return nil, fmt.Errorf("function %s not found", funcName)
}

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

	// Add functions from the same package that share common calls
	for _, otherFunc := range idx.Functions {
		if otherFunc.Name != info.Name && otherFunc.Package == info.Package {
			// Check for common function calls
			for called := range info.Calls {
				if _, exists := otherFunc.Calls[called]; exists {
					related[otherFunc.Name] = otherFunc
					break
				}
			}
			// Check for common callers
			for caller := range info.CalledBy {
				if _, exists := otherFunc.CalledBy[caller]; exists {
					related[otherFunc.Name] = otherFunc
					break
				}
			}
		}
	}

	result := make([]*FunctionInfo, 0, len(related))
	for _, info := range related {
		result = append(result, info)
	}

	return result, nil
}

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
			fmt.Printf("%s→ %s (internal function, Line: %d)\n",
				indent,
				calledFunc,
				calledInfo.LineNumber)
			idx.PrintCallTree(calledFunc, depth+1, visited)
		} else {
			// Check if it's a method call
			if strings.Contains(calledFunc, ".") {
				fmt.Printf("%s→ %s (method call)\n", indent, calledFunc)
			} else {
				fmt.Printf("%s→ %s (external/library call)\n", indent, calledFunc)
			}
		}
	}
}

func main() {
	repoPath := flag.String("path", ".", "Path to the Git repository")
	queryFunc := flag.String("func", "", "Function name to query (optional)")
	showSource := flag.Bool("source", false, "Show function source code when querying a function")
	showCalls := flag.Bool("calls", false, "Show function call tree when querying a function")
	flag.Parse()

	idx := NewCodeIndex()
	if err := idx.IndexRepository(*repoPath); err != nil {
		log.Fatalf("Failed to index repository: %v", err)
	}

	if *queryFunc != "" {
		info, err := idx.GetFunctionInfo(*queryFunc)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Printf("Function: %s\nPackage: %s\nFile: %s\nLine: %d\n\n",
			info.Name, info.Package, info.FilePath, info.LineNumber)

		if *showSource {
			if err := idx.PrintFunctionSource(*queryFunc); err != nil {
				log.Fatalf("Error printing source: %v", err)
			}
			fmt.Println()
		}

		if *showCalls {
			fmt.Println("Call tree:")
			fmt.Printf("● %s\n", *queryFunc)
			idx.PrintCallTree(*queryFunc, 1, nil)
			fmt.Println()
		}

		related, err := idx.GetRelatedFunctions(*queryFunc)
		if err != nil {
			log.Fatalf("Error getting related functions: %v", err)
		}

		if len(related) > 0 {
			fmt.Println("Related functions:")
			for _, rel := range related {
				fmt.Printf("- %s (Package: %s, File: %s, Line: %d)\n",
					rel.Name, rel.Package, rel.FilePath, rel.LineNumber)
			}
		} else {
			fmt.Println("No related functions found.")
		}
	} else {
		fmt.Println("All functions in the repository:")
		for _, info := range idx.Functions {
			fmt.Printf("- %s (Package: %s, File: %s, Line: %d)\n",
				info.Name, info.Package, info.FilePath, info.LineNumber)
		}
	}
}
