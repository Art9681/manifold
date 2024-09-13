// db_test.go
package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSQLiteDB(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "testdata", "database.sqlite")

	db, err := NewSQLiteDB(tempDir)
	require.NoError(t, err, "NewSQLiteDB should not return an error")
	assert.FileExists(t, dbPath, "Database file should be created")
	assert.NotNil(t, db, "SQLiteDB instance should not be nil")
}

func TestAutoMigrate(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewSQLiteDB(tempDir)
	require.NoError(t, err, "NewSQLiteDB should not return an error")

	err = db.AutoMigrate(&ModelParams{}, &ImageModel{}, &SelectedModels{}, &Chat{}, &URLTracking{})
	require.NoError(t, err, "AutoMigrate should not return an error")
}

func TestCreateAndFind(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewSQLiteDB(tempDir)
	require.NoError(t, err, "NewSQLiteDB should not return an error")

	err = db.AutoMigrate(&ModelParams{})
	require.NoError(t, err, "AutoMigrate should not return an error")

	model := ModelParams{
		Name:              "test-model",
		Homepage:          "https://example.com",
		Downloads:         "https://example.com/download",
		Temperature:       0.5,
		TopP:              0.8,
		TopK:              50,
		RepetitionPenalty: 1.2,
		Prompt:            "Test prompt",
		Ctx:               1500,
		Downloaded:        false,
	}

	// Test Create
	err = db.Create(&model)
	require.NoError(t, err, "Create should not return an error")

	// Test Find
	var foundModels []ModelParams
	err = db.Find(&foundModels)
	require.NoError(t, err, "Find should not return an error")
	assert.Len(t, foundModels, 1, "Should find one model")
	assert.Equal(t, "test-model", foundModels[0].Name, "Model name should match")
}

func TestLoadModelDataToDB(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewSQLiteDB(tempDir)
	require.NoError(t, err, "NewSQLiteDB should not return an error")

	err = db.AutoMigrate(&ModelParams{})
	require.NoError(t, err, "AutoMigrate should not return an error")

	models := []ModelParams{
		{
			Name:              "model1",
			Homepage:          "https://example.com/model1",
			Downloads:         "https://download.example.com/model1",
			Temperature:       0.6,
			TopP:              0.85,
			TopK:              90,
			RepetitionPenalty: 1.1,
			Prompt:            "Prompt1",
			Ctx:               2000,
			Downloaded:        true,
		},
		{
			Name:              "model2",
			Homepage:          "https://example.com/model2",
			Downloads:         "https://download.example.com/model2",
			Temperature:       0.7,
			TopP:              0.9,
			TopK:              95,
			RepetitionPenalty: 1.2,
			Prompt:            "Prompt2",
			Ctx:               2500,
			Downloaded:        false,
		},
	}

	err = loadModelDataToDB(db, models)
	require.NoError(t, err, "loadModelDataToDB should not return an error")

	// Verify models are loaded
	var loadedModels []ModelParams
	err = db.Find(&loadedModels)
	require.NoError(t, err, "Find should not return an error")
	assert.Len(t, loadedModels, 2, "Should load two models")

	// Verify first model
	assert.Equal(t, "model1", loadedModels[0].Name)
	assert.Equal(t, 0.6, loadedModels[0].Temperature)
	assert.True(t, loadedModels[0].Downloaded)

	// Verify second model
	assert.Equal(t, "model2", loadedModels[1].Name)
	assert.Equal(t, 0.7, loadedModels[1].Temperature)
	assert.False(t, loadedModels[1].Downloaded)
}

func TestLoadCompletionsRolesToDB(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewSQLiteDB(tempDir)
	require.NoError(t, err, "NewSQLiteDB should not return an error")

	err = db.AutoMigrate(&CompletionsRole{})
	require.NoError(t, err, "AutoMigrate should not return an error")

	roles := []CompletionsRole{
		{
			Name:         "role1",
			Instructions: "Instructions for role1",
		},
		{
			Name:         "role2",
			Instructions: "Instructions for role2",
		},
	}

	err = loadCompletionsRolesToDB(db, roles)
	require.NoError(t, err, "loadCompletionsRolesToDB should not return an error")

	// Verify roles are loaded
	var loadedRoles []CompletionsRole
	err = db.Find(&loadedRoles)
	require.NoError(t, err, "Find should not return an error")
	assert.Len(t, loadedRoles, 2, "Should load two roles")

	assert.Equal(t, "role1", loadedRoles[0].Name)
	assert.Equal(t, "Instructions for role1", loadedRoles[0].Instructions)

	assert.Equal(t, "role2", loadedRoles[1].Name)
	assert.Equal(t, "Instructions for role2", loadedRoles[1].Instructions)
}
