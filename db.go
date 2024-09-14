// manifold/db.go
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

type ModelParams struct {
	ID                int64   `json:"id"`
	Name              string  `json:"name"`
	Homepage          string  `json:"homepage"`
	Downloads         string  `json:"downloads"` // Changed from []string to string
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	TopK              int     `json:"top_k"`
	RepetitionPenalty float64 `json:"repetition_penalty"`
	Prompt            string  `json:"prompt"`
	Ctx               int     `json:"ctx"`
	Downloaded        bool    `json:"downloaded"`
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
	Action    string `json:"action"`
}

type Chat struct {
	ID        int64  `json:"id"`
	Prompt    string `json:"prompt"`
	Response  string `json:"response"`
	ModelName string `json:"modelName"`
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

func NewSQLiteDB(dataPath string) (*SQLiteDB, error) {
	dbPath := filepath.Join(dataPath, "eternaldata.db")

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	return &SQLiteDB{db: db}, nil
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

func (sqldb *SQLiteDB) UpdateDownloadedByName(name string, downloaded bool) (*ModelParams, error) {
	var model ModelParams
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

func loadModelDataToDB(db *SQLiteDB, models []ModelParams) error {
	for _, model := range models {
		var existingModel ModelParams
		err := db.First(model.Name, &existingModel)

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := db.Create(&model); err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			if !areModelParamsEqual(existingModel, model) {
				if err := db.UpdateByName(model.Name, &model); err != nil {
					return err
				}
			}
		}
	}

	log.Println("Model data loaded to database")

	return nil
}

func areModelParamsEqual(a, b ModelParams) bool {
	if a.Name != b.Name {
		return false
	}
	if a.Homepage != b.Homepage {
		return false
	}
	if a.Temperature != b.Temperature {
		return false
	}
	if a.TopP != b.TopP {
		return false
	}
	if a.TopK != b.TopK {
		return false
	}
	if a.RepetitionPenalty != b.RepetitionPenalty {
		return false
	}
	if a.Prompt != b.Prompt {
		return false
	}
	if a.Ctx != b.Ctx {
		return false
	}
	if a.Downloads != b.Downloads {
		return false
	}
	if a.Downloaded != b.Downloaded {
		return false
	}

	return true
}

func LoadImageModelDataToDB(db *SQLiteDB, models []ImageModel) error {
	for _, model := range models {
		var existingModel ImageModel
		err := db.First(model.Name, &existingModel)

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := db.Create(&model); err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			if err := db.UpdateByName(model.Name, &model); err != nil {
				return err
			}
		}
	}

	return nil
}

func AddSelectedModel(db *gorm.DB, modelName string) error {
	if err := db.Exec("DELETE FROM selected_models").Error; err != nil {
		return err
	}

	selectedModel := SelectedModels{
		ModelName: modelName,
	}

	return db.Create(&selectedModel).Error
}

func RemoveSelectedModel(db *gorm.DB, modelName string) error {
	return db.Where("model_name = ?", modelName).Delete(&SelectedModels{}).Error
}

func GetSelectedModels(db *gorm.DB) ([]SelectedModels, error) {
	var selectedModels []SelectedModels
	if err := db.Find(&selectedModels).Error; err != nil {
		return nil, err
	}
	return selectedModels, nil
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

func checkDownloadedModels(db *SQLiteDB, config *Config) (*Config, error) {
	for i := range config.LanguageModels {
		model := &config.LanguageModels[i]

		modelPath := filepath.Join(config.DataPath, "models", model.Name)
		if _, err := os.Stat(modelPath); err == nil {
			modelParams, err := db.UpdateDownloadedByName(model.Name, true)
			if err != nil {
				return nil, fmt.Errorf("failed to update downloaded state for model %s: %w", model.Name, err)
			}

			model.Name = modelParams.Name
			model.Homepage = modelParams.Homepage
			model.Downloads = modelParams.Downloads
			model.Temperature = modelParams.Temperature
			model.TopP = modelParams.TopP
			model.TopK = modelParams.TopK
			model.RepetitionPenalty = modelParams.RepetitionPenalty
			model.Prompt = modelParams.Prompt
			model.Ctx = modelParams.Ctx
			model.Downloaded = modelParams.Downloaded
		}
	}

	return config, nil
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
	toolParam := ToolParam{
		ToolID:     toolID,
		ParamName:  paramName,
		ParamValue: paramValue,
	}
	return sqldb.db.Create(&toolParam).Error
}

func (sqldb *SQLiteDB) GetToolMetadataByName(name string) (*ToolMetadata, error) {
	var tool ToolMetadata
	err := sqldb.db.Preload("Params").Where("name = ?", name).First(&tool).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find tool %s: %v", name, err)
	}
	return &tool, nil
}

func (sqldb *SQLiteDB) UpdateToolMetadataByName(name string, enabled bool) error {
	return sqldb.db.Model(&ToolMetadata{}).Where("name = ?", name).Update("enabled", enabled).Error
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
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				toolMetadata := ToolMetadata{Name: toolConfig.Name, Enabled: toolConfig.Parameters["enabled"].(bool)}
				if err := db.CreateToolMetadata(toolMetadata); err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			err = db.UpdateToolMetadataByName(toolConfig.Name, toolConfig.Parameters["enabled"].(bool))
			if err != nil {
				return err
			}
		}

		for paramName, paramValue := range toolConfig.Parameters {
			var existingParam ToolParam
			err := db.db.Where("tool_id = ? AND param_name = ?", existingTool.ID, paramName).First(&existingParam).Error
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if err := db.CreateToolParam(existingTool.ID, paramName, fmt.Sprintf("%v", paramValue)); err != nil {
						return err
					}
				} else {
					return err
				}
			} else {
				if err := db.UpdateToolParam(existingTool.ID, paramName, fmt.Sprintf("%v", paramValue)); err != nil {
					return err
				}
			}
		}
	}
	log.Println("Tools metadata loaded to database")
	return nil
}
