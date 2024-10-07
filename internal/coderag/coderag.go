package coderag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Config holds the configuration for the coderag package.
type Config struct {
	OpenAIAPIKey   string
	OpenAIEndpoint string
	OpenAIModel    string
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set")
	}

	endpoint := strings.TrimSpace(os.Getenv("OPENAI_API_ENDPOINT"))
	if endpoint == "" {
		// Adjust to your working API endpoint
		// endpoint = "https://api.openai.com/v1/chat/completions"
		endpoint = "http://localhost:32182/v1/chat/completions"
	}

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		// Example model identifier, make sure it matches your setup
		model = "gpt-4o-mini"
	}

	return &Config{
		OpenAIAPIKey:   apiKey,
		OpenAIEndpoint: endpoint,
		OpenAIModel:    model,
	}, nil
}

// FunctionInfo stores information about a function, method, or variable.
type FunctionInfo struct {
	Name       string   `json:"name"`
	FilePath   string   `json:"file_path"`
	Package    string   `json:"package"`
	Type       string   `json:"type"` // e.g., function, method, variable
	Parameters []string `json:"parameters,omitempty"`
	Returns    []string `json:"returns,omitempty"`
	Comments   string   `json:"comments,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Code       string   `json:"code,omitempty"`
	CalledBy   []string `json:"called_by,omitempty"`
	Calls      []string `json:"calls,omitempty"`
	LineNumber int      `json:"line_number"`
}

// RelationshipInfo encapsulates the relationships of a function.
type RelationshipInfo struct {
	FunctionName      string   `json:"function_name"`
	Comments          string   `json:"comments"`
	Code              string   `json:"code"`
	Summary           string   `json:"summary"`
	Calls             []string `json:"calls"`
	CalledBy          []string `json:"called_by"`
	CallsFilePaths    []string `json:"calls_file_paths"`
	CalledByFilePaths []string `json:"called_by_file_paths"`
	TotalCalls        int      `json:"total_calls"`
	TotalCalledBy     int      `json:"total_called_by"`
}

// VariableInfo stores information about a variable.
type VariableInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Scope      string `json:"scope,omitempty"` // e.g., function, package, global
	FilePath   string `json:"file_path"`
	LineNumber int    `json:"line_number"`
}

// DependencyGraph represents dependencies between functions.
type DependencyGraph struct {
	Nodes []string `json:"nodes"`
	Edges []struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"edges"`
}

// RefactoringOpportunity represents potential refactoring suggestions.
type RefactoringOpportunity struct {
	Description string `json:"description"`
	Location    string `json:"location"` // File and line number
	Severity    string `json:"severity"` // e.g., minor, major, critical
}

// Codebase encapsulates all extracted information.
type Codebase struct {
	Functions                map[string]*FunctionInfo `json:"functions"`
	Variables                map[string]*VariableInfo `json:"variables"`
	Files                    map[string][]string      `json:"files"`
	Packages                 map[string][]string      `json:"packages"`
	DependencyGraph          DependencyGraph          `json:"dependency_graph"`
	RefactoringOpportunities []RefactoringOpportunity `json:"refactoring_opportunities"`
}

// CodeIndex stores the indexed information about the codebase.
type CodeIndex struct {
	Functions                map[string]*FunctionInfo
	Variables                map[string]*VariableInfo
	Files                    map[string][]string
	Packages                 map[string][]string
	DependencyGraph          DependencyGraph
	RefactoringOpportunities []RefactoringOpportunity
	fset                     *token.FileSet
	mu                       sync.RWMutex
}

// extractFunctionName parses the user prompt to extract the function name.
// It handles patterns like "function SaveChatTurn" or "SaveChatTurn function".
func extractFunctionName(prompt string) (string, error) {
	re := regexp.MustCompile(`(?i)(?:function|method)\s+([A-Za-z0-9_]+)|([A-Za-z0-9_]+)\s+(?:function|method)`)
	matches := re.FindStringSubmatch(prompt)
	if len(matches) < 2 {
		return "", fmt.Errorf("unable to extract function name from prompt")
	}
	if matches[1] != "" {
		return matches[1], nil
	}
	return matches[2], nil
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

// HandleUserPrompt processes a user prompt, matches it to a function, and returns its relationships along with code and comments.
func (idx *CodeIndex) HandleUserPrompt(prompt string) (*RelationshipInfo, error) {
	funcName, err := extractFunctionName(prompt)
	if err != nil {
		return nil, err
	}

	info, err := idx.GetFunctionInfo(funcName)
	if err != nil {
		return nil, err
	}

	calls := make([]string, 0, len(info.Calls))
	callsFilePaths := make([]string, 0, len(info.Calls))
	for _, called := range info.Calls {
		calls = append(calls, called)
		if calledInfo, exists := idx.Functions[called]; exists {
			callsFilePaths = append(callsFilePaths, calledInfo.FilePath)
		} else {
			callsFilePaths = append(callsFilePaths, "External/Unknown")
		}
	}

	calledBy := make([]string, 0, len(info.CalledBy))
	calledByFilePaths := make([]string, 0, len(info.CalledBy))
	for _, caller := range info.CalledBy {
		calledBy = append(calledBy, caller)
		if callerInfo, exists := idx.Functions[caller]; exists {
			calledByFilePaths = append(calledByFilePaths, callerInfo.FilePath)
		} else {
			calledByFilePaths = append(calledByFilePaths, "External/Unknown")
		}
	}

	relationship := &RelationshipInfo{
		FunctionName:      funcName,
		Comments:          info.Comments,
		Code:              info.Code,
		Summary:           info.Summary,
		Calls:             calls,
		CalledBy:          calledBy,
		CallsFilePaths:    callsFilePaths,
		CalledByFilePaths: calledByFilePaths,
		TotalCalls:        len(calls),
		TotalCalledBy:     len(calledBy),
	}

	return relationship, nil
}

// StartAPIServer starts an HTTP server for querying the codebase.
func (idx *CodeIndex) StartAPIServer(port int) {
	http.HandleFunc("/function", idx.handleFunctionQuery)
	//http.HandleFunc("/file", idx.handleFileQuery)
	//http.HandleFunc("/refactor", idx.handleRefactorQuery)
	//http.HandleFunc("/dependency", idx.handleDependencyQuery)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting API server at %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start API server: %v", err)
	}
}

// Helper methods for the API server
func (idx *CodeIndex) handleFunctionQuery(w http.ResponseWriter, r *http.Request) {
	funcName := r.URL.Query().Get("name")
	if funcName == "" {
		http.Error(w, "Missing 'name' query parameter", http.StatusBadRequest)
		return
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	fn, exists := idx.Functions[funcName]
	if !exists {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(fn)
}

// NewCodeIndex creates a new instance of CodeIndex.
func NewCodeIndex() *CodeIndex {
	return &CodeIndex{
		Functions:                make(map[string]*FunctionInfo),
		Variables:                make(map[string]*VariableInfo),
		Files:                    make(map[string][]string),
		Packages:                 make(map[string][]string),
		DependencyGraph:          DependencyGraph{},
		RefactoringOpportunities: []RefactoringOpportunity{},
		fset:                     token.NewFileSet(),
	}
}

// Implement the extractVariables method to process variable declarations.
func (idx *CodeIndex) extractVariables(genDecl *ast.GenDecl, path, packageName string) {
	for _, spec := range genDecl.Specs {
		if valueSpec, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range valueSpec.Names {
				position := idx.fset.Position(name.Pos())
				var varType string
				if valueSpec.Type != nil {
					varType = idx.exprToString(valueSpec.Type)
				} else {
					varType = "unknown"
				}

				idx.mu.Lock()
				idx.Variables[name.Name] = &VariableInfo{
					Name:       name.Name,
					Type:       varType,
					Scope:      "package", // Assuming package-level scope
					FilePath:   path,
					LineNumber: position.Line,
				}
				idx.Files[path] = append(idx.Files[path], name.Name)
				idx.mu.Unlock()
			}
		}
	}
}

// Implement the getFunctionName method to get the function name, including receiver if any.
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

// Update analyzeCallExpr to remove or use the isMethodCall variable if necessary.
func (idx *CodeIndex) analyzeCallExpr(callExpr *ast.CallExpr, currentFunc *FunctionInfo) {
	if currentFunc == nil || currentFunc.Type != "function" {
		return
	}

	calledFunc, _ := idx.resolveCalledFunction(callExpr) // Removed unused isMethodCall variable

	if calledFunc != "" {
		idx.mu.Lock()
		currentFunc.Calls = append(currentFunc.Calls, calledFunc)
		if calledInfo, exists := idx.Functions[calledFunc]; exists {
			calledInfo.CalledBy = append(calledInfo.CalledBy, currentFunc.Name)
		}
		idx.mu.Unlock()
	}

	for _, arg := range callExpr.Args {
		if nestedCall, ok := arg.(*ast.CallExpr); ok {
			idx.analyzeCallExpr(nestedCall, currentFunc)
		}
	}
}

// IndexRepository walks through the repository and indexes all Go files for function relationships.
func (idx *CodeIndex) IndexRepository(repoPath string, cfg *Config) error {
	if err := filepath.Walk(repoPath, idx.indexDeclarations); err != nil {
		return err
	}

	if err := filepath.Walk(repoPath, idx.indexCallRelationships); err != nil {
		return err
	}

	if err := idx.GenerateSummaries(cfg); err != nil {
		return err
	}

	idx.AnalyzeCodeSmells(100)

	if err := idx.SerializeToJSON("codebase.json"); err != nil {
		return err
	}

	return nil
}

// indexDeclarations processes each file to extract function and variable declarations.
func (idx *CodeIndex) indexDeclarations(path string, info os.FileInfo, err error) error {
	if err != nil || !strings.HasSuffix(path, ".go") || strings.Contains(path, "vendor/") {
		return err
	}

	file, err := parser.ParseFile(idx.fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %v", path, err)
	}

	packageName := file.Name.Name
	idx.mu.Lock()
	if _, exists := idx.Packages[packageName]; !exists {
		idx.Packages[packageName] = []string{}
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

// extractFunction extracts function/method declarations from the AST.
func (idx *CodeIndex) extractFunction(fn *ast.FuncDecl, path, packageName string) {
	funcName := idx.getFunctionName(fn)
	position := idx.fset.Position(fn.Pos())

	comments := ""
	if fn.Doc != nil {
		comments = strings.TrimSpace(fn.Doc.Text())
	}

	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	err := cfg.Fprint(&buf, idx.fset, fn)
	code := ""
	if err == nil {
		code = buf.String()
	} else {
		code = fmt.Sprintf("Error extracting code: %v", err)
	}

	parameters := idx.extractParameters(fn)
	returns := idx.extractReturns(fn)

	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.Functions[funcName] = &FunctionInfo{
		Name:       funcName,
		FilePath:   path,
		Package:    packageName,
		CalledBy:   []string{},
		Calls:      []string{},
		LineNumber: position.Line,
		Type:       "function",
		Comments:   comments,
		Code:       code,
		Parameters: parameters,
		Returns:    returns,
		Summary:    "",
	}

	idx.Files[path] = append(idx.Files[path], funcName)
	if ast.IsExported(fn.Name.Name) {
		idx.Packages[packageName] = append(idx.Packages[packageName], funcName)
	}
}

// extractParameters and extractReturns helper functions
func (idx *CodeIndex) extractParameters(fn *ast.FuncDecl) []string {
	var params []string
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			paramType := idx.exprToString(param.Type)
			for _, name := range param.Names {
				params = append(params, fmt.Sprintf("%s %s", name.Name, paramType))
			}
		}
	}
	return params
}

func (idx *CodeIndex) extractReturns(fn *ast.FuncDecl) []string {
	var returns []string
	if fn.Type.Results != nil {
		for _, result := range fn.Type.Results.List {
			returnType := idx.exprToString(result.Type)
			returns = append(returns, returnType)
		}
	}
	return returns
}

// exprToString converts ast.Expr to string.
func (idx *CodeIndex) exprToString(expr ast.Expr) string {
	var buf bytes.Buffer
	printer.Fprint(&buf, token.NewFileSet(), expr)
	return buf.String()
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

// resolveCalledFunction determines the name of the called function and whether it's a method call.
func (idx *CodeIndex) resolveCalledFunction(callExpr *ast.CallExpr) (string, bool) {
	var calledFunc string
	isMethodCall := false

	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		calledFunc = fn.Name
	case *ast.SelectorExpr:
		switch x := fn.X.(type) {
		case *ast.Ident:
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

// SummarizeCode sends the code to OpenAI API and returns the summary.
func (idx *CodeIndex) SummarizeCode(code string, cfg *Config) (string, error) {
	// Construct the messages structure according to the chat completion format
	messages := []map[string]string{
		{"role": "system", "content": "You are a helpful AI assistant that responds in well structured markdown format. Do not repeat your instructions. Do not deviate from the topic."},
		{"role": "user", "content": fmt.Sprintf("Provide a concise summary for the following Go function:\n\n%s", code)},
	}

	// Define the request payload using the updated structure
	payload := map[string]interface{}{
		//"model":       cfg.OpenAIModel,
		"messages":    messages,
		"temperature": 0.3,
		//"max_tokens":  150,
		"stream": false,
	}

	// Print the payload for debugging purposes
	fmt.Println("Payload:", payload)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", cfg.OpenAIEndpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	//req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.OpenAIAPIKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Request URL: %s", cfg.OpenAIEndpoint)
		log.Printf("Request Payload: %s", string(payloadBytes))
		return "", fmt.Errorf("failed to send request to OpenAI: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OpenAI response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error: %s", string(bodyBytes))
	}

	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal OpenAI response: %v", err)
	}

	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("no summary returned by OpenAI")
	}

	// Return the content from the first choice
	return strings.TrimSpace(openAIResp.Choices[0].Message.Content), nil
}

// GenerateSummaries generates summaries for all functions using OpenAI API one at a time.
func (idx *CodeIndex) GenerateSummaries(cfg *Config) error {
	for _, fn := range idx.Functions {
		if fn.Type != "function" && fn.Type != "method" {
			continue
		}

		// Retry mechanism for each function in case of failure
		success := false
		for attempt := 1; attempt <= 3; attempt++ {
			log.Printf("Summarizing function %s (attempt %d)...", fn.Name, attempt)
			summary, err := idx.SummarizeCode(fn.Code, cfg)
			if err != nil {
				log.Printf("Failed to summarize function %s: %v", fn.Name, err)
				time.Sleep(2 * time.Second) // Backoff before retrying
			} else {
				fn.Summary = summary
				log.Printf("Successfully summarized function %s.", fn.Name)
				success = true
				break
			}
		}

		// If after retries it still fails, mark the summary as unavailable
		if !success {
			fn.Summary = "Summary not available."
		}
	}
	return nil
}

// SerializeToJSON serializes the CodeIndex into a JSON file.
func (idx *CodeIndex) SerializeToJSON(outputPath string) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	codebase := Codebase{
		Functions:                make(map[string]*FunctionInfo),
		Variables:                make(map[string]*VariableInfo),
		Files:                    make(map[string][]string),
		Packages:                 make(map[string][]string),
		DependencyGraph:          idx.DependencyGraph,
		RefactoringOpportunities: idx.RefactoringOpportunities,
	}

	for name, fn := range idx.Functions {
		codebase.Functions[name] = fn
	}

	for name, varInfo := range idx.Variables {
		codebase.Variables[name] = varInfo
	}

	for file, funcs := range idx.Files {
		codebase.Files[file] = funcs
	}

	for pkg, funcs := range idx.Packages {
		codebase.Packages[pkg] = funcs
	}

	data, err := json.MarshalIndent(codebase, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outputPath, data, 0644)
}

// AnalyzeCodeSmells detects functions that exceed a specified number of lines.
func (idx *CodeIndex) AnalyzeCodeSmells(maxLines int) {
	for _, fn := range idx.Functions {
		if fn.Type != "function" && fn.Type != "method" {
			continue
		}
		lineCount := strings.Count(fn.Code, "\n")
		if lineCount > maxLines {
			opportunity := RefactoringOpportunity{
				Description: fmt.Sprintf("Function '%s' is too long (%d lines). Consider breaking it into smaller functions.", fn.Name, lineCount),
				Location:    fmt.Sprintf("%s:%d", fn.FilePath, fn.LineNumber),
				Severity:    "major",
			}
			idx.RefactoringOpportunities = append(idx.RefactoringOpportunities, opportunity)
		}
	}
}
