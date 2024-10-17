// manifold/db.go
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteDB struct {
	db *gorm.DB
}

type ChatSession struct {
	ID        int64 `json:"id"`
	CreatedAt time.Time
	UpdatedAt time.Time
	ChatTurns []ChatTurn `json:"chat_turns"`
}

type ChatTurn struct {
	ID         int64 `json:"id"`
	SessionID  int64
	UserPrompt string
	Responses  []ChatResponse `json:"responses"`
}

type ChatResponse struct {
	ID        int64 `json:"id"`
	TurnID    int64
	Content   string
	Model     string // Identifier for the LLM model used
	Host      SystemInfo
	CreatedAt time.Time
}

type SystemInfo struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	CPUs   int    `json:"cpus"`
	Memory Memory `json:"memory"`
	GPUs   []GPU  `json:"gpus"`
}

type Memory struct {
	Total int64 `json:"total"`
}

type GPU struct {
	Model              string `json:"model"`
	TotalNumberOfCores string `json:"total_number_of_cores"`
	MetalSupport       string `json:"metal_support"`
}

type LanguageModels struct {
	ID                int64   `json:"id"`
	Name              string  `json:"name"`
	Path              string  `json:"path"`
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	TopK              int     `json:"top_k"`
	RepetitionPenalty float64 `json:"repetition_penalty"`
	Ctx               int     `json:"ctx"`
	ModelID           int64   `json:"model_id"` // Foreign key reference
}

// Model represents a language model, either gguf or mlx.
type LanguageModel struct {
	ID                int64   `gorm:"primaryKey" json:"id"`
	Name              string  `gorm:"uniqueIndex:idx_name_type" json:"name"`       // Model name
	Path              string  `gorm:"uniqueIndex:idx_name_type" json:"path"`       // Full path to the model file
	ModelType         string  `gorm:"uniqueIndex:idx_name_type" json:"model_type"` // "gguf" or "mlx"
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	TopK              int     `json:"top_k"`
	RepetitionPenalty float64 `json:"repetition_penalty"`
	Ctx               int     `json:"ctx"`
}

// TableName sets the table name for GORM.
func (Model) TableName() string {
	return "models"
}

type ImageModel struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Homepage   string `json:"homepage"`
	Prompt     string `json:"prompt"`
	Downloads  string `json:"downloads,omitempty"`
	Downloaded bool   `json:"downloaded"`
}

type SelectedModels struct {
	ID        int64  `json:"id"`
	ModelName string `json:"modelName"`
	ModelPath string `json:"modelPath"`
	Action    string `json:"action"`
}

type Chat struct {
	ID        int64  `json:"id"`
	Prompt    string `json:"prompt"`
	Response  string `json:"response"`
	ModelName string `json:"modelName"`
	Embedding []byte `json:"embedding"`
}

type URLTracking struct {
	ID  int64  `json:"id"`
	URL string `json:"url"`
}

type ToolMetadata struct {
	ID      uint   `gorm:"primaryKey"`
	Name    string `gorm:"index"`
	Enabled bool
	Params  []ToolParam `gorm:"foreignKey:ToolID"`
}

type ToolParam struct {
	ID         uint   `gorm:"primaryKey"`
	ToolID     uint   `gorm:"index"`
	ParamName  string `gorm:"index"`
	ParamValue string
}

// NewSQLiteDB initializes a new SQLiteDB with detailed logging.
func NewSQLiteDB(dataPath string) (*SQLiteDB, error) {
	dbPath := filepath.Join(dataPath, "eternaldata.db") // Ensure consistency here

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	// Configure GORM's logger for detailed output
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,  // Slow SQL threshold
			LogLevel:                  logger.Error, // Log level (Info for detailed logs)
			IgnoreRecordNotFoundError: true,         // Ignore ErrRecordNotFound error for logger
			Colorful:                  true,         // Enable color
		},
	)

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	return &SQLiteDB{db: db}, nil
}

// Enable SQLite extension loading
func (sqldb *SQLiteDB) EnableSQLiteExtensionLoading() error {
	db := sqldb.db

	// Enable foreign keys
	if err := db.Exec("PRAGMA foreign_keys = ON;").Error; err != nil {
		return fmt.Errorf("could not enable foreign keys: %w", err)
	}

	// Optionally enable extension loading, if needed
	if err := db.Exec("PRAGMA load_extension = 1;").Error; err != nil {
		return fmt.Errorf("could not enable extension loading: %w", err)
	}

	return nil
}

// Load the sqlite-vec extension
func (sqldb *SQLiteDB) LoadVecExtension() error {
	// Load the sqlite-vec extension
	db := sqldb.db

	// Use Exec().Error to get the error result
	if err := db.Exec("SELECT load_extension('libsqlitevec.dylib', 'sqlite3_vec_init');").Error; err != nil {
		return fmt.Errorf("could not load sqlite-vec extension: %w", err)
	}

	return nil
}

func (sqldb *SQLiteDB) AutoMigrate(models ...interface{}) error {
	for _, model := range models {
		if err := sqldb.db.AutoMigrate(model); err != nil {
			return fmt.Errorf("error migrating schema for %T: %v", model, err)
		}
	}
	return nil
}

func (sqldb *SQLiteDB) Create(record interface{}) error {
	return sqldb.db.Create(record).Error
}

func (sqldb *SQLiteDB) Find(out interface{}) error {
	return sqldb.db.Find(out).Error
}

func (sqldb *SQLiteDB) First(name string, out interface{}) error {
	return sqldb.db.Where("name = ?", name).First(out).Error
}

func (sqldb *SQLiteDB) UpdateByName(name string, updatedRecord interface{}) error {
	return sqldb.db.Model(updatedRecord).Where("name = ?", name).Updates(updatedRecord).Error
}

func (sqldb *SQLiteDB) UpdateDownloadedByName(name string, downloaded bool) (*LanguageModels, error) {
	var model LanguageModels
	if err := sqldb.db.Model(&model).Where("name = ?", name).Update("downloaded", downloaded).Error; err != nil {
		return nil, err
	}
	if err := sqldb.First(name, &model); err != nil {
		return nil, err
	}
	return &model, nil
}

func (sqldb *SQLiteDB) Delete(id uint, model interface{}) error {
	return sqldb.db.Delete(model, id).Error
}

func loadCompletionsRolesToDB(db *SQLiteDB, roles []CompletionsRole) error {
	for _, role := range roles {
		var existingRole CompletionsRole
		err := db.First(role.Name, &existingRole)

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := db.Create(&role); err != nil {
					return err
				}
			}
		} else {
			return err
		}
	}

	log.Println("Completions roles data loaded to database")

	return nil
}

func SetSelectedModel(db *gorm.DB, modelName string) error {
	// Clear any previously selected models
	if err := db.Exec("DELETE FROM selected_models").Error; err != nil {
		return err
	}

	// Retrieve the model path
	var model LanguageModel
	if err := db.Where("name = ?", modelName).First(&model).Error; err != nil {
		return err
	}

	// Insert the new selected model
	selectedModel := SelectedModels{
		ModelName: modelName,
		ModelPath: model.Path,
	}

	return db.Create(&selectedModel).Error
}

func GetSelectedModels(db *gorm.DB) (SelectedModels, error) {
	var selectedModels SelectedModels
	if err := db.First(&selectedModels).Error; err != nil {
		return selectedModels, err
	}
	return selectedModels, nil
}

func RemoveSelectedModel(db *gorm.DB, modelName string) error {
	return db.Where("modelName = ?", modelName).Delete(&SelectedModels{}).Error
}

func CreateChat(db *gorm.DB, prompt, response, model string) (Chat, error) {
	chat := Chat{Prompt: prompt, Response: response, ModelName: model}
	if err := db.Create(&chat).Error; err != nil {
		return chat, err
	}
	return chat, nil
}

func GetChats(db *gorm.DB) ([]Chat, error) {
	var chats []Chat
	if err := db.Find(&chats).Error; err != nil {
		return nil, err
	}
	return chats, nil
}

func GetChatByID(db *gorm.DB, id int64) (Chat, error) {
	var chat Chat
	if err := db.First(&chat, id).Error; err != nil {
		return chat, err
	}
	return chat, nil
}

func UpdateChat(db *gorm.DB, id int64, newPrompt, newResponse, newModel string) error {
	return db.Model(&Chat{}).Where("id = ?", id).Updates(Chat{Prompt: newPrompt, Response: newResponse, ModelName: newModel}).Error
}

func DeleteChat(db *gorm.DB, id int64) error {
	return db.Delete(&Chat{}, id).Error
}

func (sqldb *SQLiteDB) CreateURLTracking(url string) error {
	var existingURLTracking URLTracking

	err := sqldb.First(url, &existingURLTracking)
	if err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	urlTracking := URLTracking{URL: url}
	return sqldb.Create(&urlTracking)
}

func (sqldb *SQLiteDB) ListURLTrackings() ([]URLTracking, error) {
	var urlTrackings []URLTracking
	err := sqldb.Find(&urlTrackings)
	return urlTrackings, err
}

func (sqldb *SQLiteDB) DeleteURLTracking(url string) error {
	return sqldb.db.Where("url = ?", url).Delete(&URLTracking{}).Error
}

// CreateToolMetadata adds a new tool to the database
func (sqldb *SQLiteDB) CreateToolMetadata(tool ToolMetadata) error {
	if err := sqldb.db.Create(&tool).Error; err != nil {
		return fmt.Errorf("failed to create tool metadata: %v", err)
	}
	return nil
}

// CreateToolParam adds a parameter to a tool
func (sqldb *SQLiteDB) CreateToolParam(toolID uint, paramName, paramValue string) error {
	// Ensure that the tool exists with the given toolID
	var tool ToolMetadata
	if err := sqldb.db.First(&tool, toolID).Error; err != nil {
		return fmt.Errorf("tool with ID %d not found: %w", toolID, err)
	}

	toolParam := ToolParam{
		ToolID:     toolID,
		ParamName:  paramName,
		ParamValue: paramValue,
	}

	// Insert the tool param
	if err := sqldb.db.Create(&toolParam).Error; err != nil {
		return fmt.Errorf("failed to create tool parameter: %w", err)
	}

	return nil
}

// GetToolMetadataByName retrieves a tool's metadata by its name.
func (sqldb *SQLiteDB) GetToolMetadataByName(name string) (*ToolMetadata, error) {
	var tool ToolMetadata
	err := sqldb.db.Preload("Params").Where("name = ?", name).First(&tool).Error
	if err != nil {
		return nil, fmt.Errorf("tool '%s' not found: %w", name, err)
	}
	return &tool, nil
}

// UpdateToolMetadataByName updates the 'enabled' status of a tool by its name.
func (sqldb *SQLiteDB) UpdateToolMetadataByName(name string, enabled bool) error {

	log.Printf("Updating tool '%s' status to %v", name, enabled)

	result := sqldb.db.Model(&ToolMetadata{}).Where("name = ?", name).Update("enabled", enabled)
	if result.Error != nil {
		return fmt.Errorf("failed to update tool '%s' status: %w", name, result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

func (sqldb *SQLiteDB) UpdateToolParam(toolID uint, paramName, paramValue string) error {
	return sqldb.db.Model(&ToolParam{}).
		Where("tool_id = ? AND param_name = ?", toolID, paramName).
		Update("param_value", paramValue).Error
}

func loadToolsToDB(db *SQLiteDB, tools []ToolConfig) error {
	for _, toolConfig := range tools {
		var existingTool ToolMetadata
		err := db.db.Preload("Params").Where("name = ?", toolConfig.Name).First(&existingTool).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			toolMetadata := ToolMetadata{Name: toolConfig.Name, Enabled: toolConfig.Parameters["enabled"].(bool)}
			if err := db.CreateToolMetadata(toolMetadata); err != nil {
				return err
			}

			// Make sure toolMetadata has the new tool ID
			err = db.db.Where("name = ?", toolConfig.Name).First(&existingTool).Error
			if err != nil {
				return fmt.Errorf("failed to retrieve newly created tool: %v", err)
			}
		} else if err != nil {
			return err
		}

		// Insert or update tool parameters
		for paramName, paramValue := range toolConfig.Parameters {
			if err := db.CreateToolParam(existingTool.ID, paramName, fmt.Sprintf("%v", paramValue)); err != nil {
				return err
			}
		}
	}
	log.Println("Tools metadata loaded to database")
	return nil
}

// stopWords is a map of stop words that are common but carry little meaning in searches.
var stopWords = map[string]bool{
	"the":  true,
	"is":   true,
	"of":   true,
	"and":  true,
	"a":    true,
	"an":   true,
	"in":   true,
	"to":   true,
	"for":  true,
	"with": true,
	"on":   true,
	"at":   true,
	"by":   true,
	"from": true,
	"that": true,
	"this": true,
}

// sanitizeFTSQuery cleans the query string for FTS5 by escaping and removing problematic characters.
func sanitizeFTSQuery(query string) string {
	// Replace single quotes with two single quotes for SQLite escaping
	sanitized := strings.ReplaceAll(query, "'", "''")

	// Remove unwanted punctuation except for '"' and '*'
	re := regexp.MustCompile(`[^\w\s*"*]`)
	sanitized = re.ReplaceAllString(sanitized, "")

	// Trim leading and trailing whitespace
	sanitized = strings.TrimSpace(sanitized)

	// Split the query into terms
	terms := strings.Fields(sanitized)
	var filteredTerms []string

	// Remove stop words
	for _, term := range terms {
		if !stopWords[strings.ToLower(term)] {
			// Optionally, you can handle phrases or boolean operators here
			filteredTerms = append(filteredTerms, term)
		}
	}

	// Rejoin the filtered terms
	return strings.Join(filteredTerms, " ")
}

func (sqldb *SQLiteDB) RetrieveTopNDocuments(ctx context.Context, query string, topN int) ([]string, error) {

	log.Printf("Retrieving top %d documents for query: %s", topN, query)

	// Step 1: Initial query sanitization and execution
	sanitizedQuery := sanitizeFTSQuery(query)
	results, err := sqldb.executeFTSQuery(sanitizedQuery, topN)
	ragInstructions := "Use the previous information to answer the following message if it is relevant to answer or complete the following message:\n"

	// If results are found, return them
	if err == nil && len(results) > topN {
		// Append instructions for the RAG model
		results = append(results, ragInstructions)
		return results, nil
	}

	// Step 2: Generalize by removing stop words or less important terms
	terms := strings.Fields(sanitizedQuery)
	for len(terms) > 1 {
		terms = removeLessImportantTerms(terms)
		generalizedQuery := strings.Join(terms, " ")
		results, err = sqldb.executeFTSQuery(generalizedQuery, topN)
		if err == nil && len(results) > topN {
			// Append instructions for the RAG model
			results = append(results, ragInstructions)
			return results, nil
		}
	}

	// Step 3: Apply prefix matching for partial matches
	terms = strings.Fields(sanitizedQuery)
	for i, term := range terms {
		terms[i] = fmt.Sprintf("%s*", term)
	}
	prefixQuery := strings.Join(terms, " ")
	results, err = sqldb.executeFTSQuery(prefixQuery, topN)
	if err == nil && len(results) > topN {
		// Append instructions for the RAG model
		results = append(results, ragInstructions)
		return results, nil
	}

	// Step 4: Optionally apply fuzzy matching using Levenshtein distance
	// (You can implement this if needed based on your previous logic.)

	// Step 5: Wildcard matching for the most general case
	terms = strings.Fields(sanitizedQuery)
	finalQuery := strings.Join(terms, " OR ")
	results, err = sqldb.executeFTSQuery(finalQuery, topN)
	if err == nil && len(results) > 0 {
		// Append instructions for the RAG model
		results = append(results, ragInstructions)
		return results, nil
	}

	// If no results are found at all, return an error or empty list
	return nil, fmt.Errorf("no documents found after generalizing query")
}

// Helper function to remove less important terms (like stop words)
func removeLessImportantTerms(terms []string) []string {
	if len(terms) > 1 {
		return terms[:len(terms)-1] // Remove one term at a time (simplified strategy)
	}
	return terms
}

// Helper function to execute FTS5 query with given query string and return results
func (sqldb *SQLiteDB) executeFTSQuery(ftsQuery string, topN int) ([]string, error) {
	var results []struct {
		Prompt     string
		Response   string
		Similarity float64
	}

	// Log the query for debugging purposes
	log.Printf("Executing FTS5 Query: %s", ftsQuery)

	// Execute the FTS5 query with match syntax
	err := sqldb.db.Raw(`
		SELECT prompt, response 
		FROM chat_fts 
		WHERE chat_fts MATCH ? 
		ORDER BY bm25(chat_fts, 1.5, 1.0) DESC
		LIMIT ?;
	`, ftsQuery, topN).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	// Format results
	var formattedResults []string
	for _, row := range results {
		formatted := fmt.Sprintf("%s\n%s\n", row.Prompt, row.Response)
		formattedResults = append(formattedResults, formatted)
	}

	return formattedResults, nil
}

func embeddingToBlob(embedding []float64) []byte {
	buf := make([]byte, len(embedding)*8) // Each float64 is 8 bytes
	for i, v := range embedding {
		binary.LittleEndian.PutUint64(buf[i*8:(i+1)*8], math.Float64bits(v))
	}
	return buf
}

// blobToEmbedding converts a byte slice (BLOB) back into a slice of float64 values.
func blobToEmbedding(blob []byte) []float64 {
	embedding := make([]float64, len(blob)/8) // Each float64 is 8 bytes
	for i := 0; i < len(embedding); i++ {
		embedding[i] = math.Float64frombits(binary.LittleEndian.Uint64(blob[i*8:]))
	}
	return embedding
}

// ScanGGUFModels scans the "models-gguf" directory and returns a list of models.
func ScanGGUFModels(modelsDir string) ([]LanguageModel, error) {
	var ggufModels []LanguageModel

	ggufPath := filepath.Join(modelsDir, "models-gguf")
	entries, err := os.ReadDir(ggufPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read models-gguf directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			modelName := entry.Name()
			modelDir := filepath.Join(ggufPath, modelName)

			files, err := ioutil.ReadDir(modelDir)
			if err != nil {
				log.Printf("Failed to read directory %s: %v", modelDir, err)
				continue
			}

			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".gguf") {
					fullPath := filepath.Join(modelDir, file.Name())
					ggufModels = append(ggufModels, LanguageModel{
						Name:              modelName,
						Path:              fullPath,
						ModelType:         "gguf",
						Temperature:       0.5,
						TopP:              0.9,
						TopK:              50,
						RepetitionPenalty: 1.1,
						Ctx:               4096,
					})
					break // Only first gguf file per model
				}
			}
		}
	}

	return ggufModels, nil
}

// ScanMLXModels scans the "models-mlx" directory and returns a list of models.
func ScanMLXModels(modelsDir string) ([]LanguageModel, error) {
	var mlxModels []LanguageModel

	mlxPath := filepath.Join(modelsDir, "models-mlx")
	entries, err := os.ReadDir(mlxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read models-mlx directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			modelName := entry.Name()
			modelDir := filepath.Join(mlxPath, modelName)

			files, err := os.ReadDir(modelDir)
			if err != nil {
				log.Printf("Failed to read directory %s: %v", modelDir, err)
				continue
			}

			var safetensorsPath string
			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".safetensors") {
					fullPath := filepath.Join(modelDir, file.Name())
					safetensorsPath = fullPath
					break // Only first safetensors file per model
				}
			}

			if safetensorsPath != "" {
				mlxModels = append(mlxModels, LanguageModel{
					Name:              modelName,
					Path:              safetensorsPath,
					ModelType:         "mlx",
					Temperature:       0.5,
					TopP:              0.9,
					TopK:              50,
					RepetitionPenalty: 1.1,
					Ctx:               4096,
				})
			}
		}
	}

	return mlxModels, nil
}

// SyncModels synchronizes the models in the filesystem with the database.
func (sqldb *SQLiteDB) SyncModels(models []LanguageModel) error {
	for _, model := range models {
		var existing LanguageModel
		err := sqldb.db.Where("name = ? AND model_type = ?", model.Name, model.ModelType).First(&existing).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Insert new model
			if err := sqldb.db.Create(&model).Error; err != nil {
				log.Printf("Failed to insert model %s (%s): %v", model.Name, model.ModelType, err)
				continue
			}
			log.Printf("Inserted new model: %s (%s)", model.Name, model.ModelType)
		} else if err != nil {
			log.Printf("Error querying model %s (%s): %v", model.Name, model.ModelType, err)
			continue
		} else {
			// Model already exists, optionally update fields if needed
			// For now, do nothing
		}
	}

	// Remove models from DB that no longer exist in the filesystem
	var dbModels []LanguageModel
	if err := sqldb.db.Find(&dbModels).Error; err != nil {
		return fmt.Errorf("failed to retrieve models from DB: %v", err)
	}

	for _, dbModel := range dbModels {
		if !fileExists(dbModel.Path) {
			if err := sqldb.db.Delete(&dbModel).Error; err != nil {
				log.Printf("Failed to delete model %s (%s): %v", dbModel.Name, dbModel.ModelType, err)
				continue
			}
			log.Printf("Deleted missing model: %s (%s)", dbModel.Name, dbModel.ModelType)
		}
	}

	return nil
}

func GetModelsByBackend(db *gorm.DB, backend string) ([]LanguageModel, error) {
	var models []LanguageModel
	err := db.Where("model_type = ?", backend).Find(&models).Error
	if err != nil {
		return nil, err
	}
	return models, nil
}

func (sqldb *SQLiteDB) GetModels() ([]LanguageModel, error) {
	var models []LanguageModel
	if err := sqldb.db.Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

func (sqldb *SQLiteDB) GetToolsMetadata() ([]ToolMetadata, error) {
	var tools []ToolMetadata
	if err := sqldb.db.Preload("Params").Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}
