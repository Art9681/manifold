// Package main provides the initialization and setup functions for the application.
package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/blevesearch/bleve/v2"
)

// embedFS embeds the necessary files for the application.
//
//go:embed public/*
var embedFS embed.FS

// initializeApplication initializes the application with the given configuration.
func initializeApplication(config *Config) (*SQLiteDB, *bleve.Index, error) {
	createDataDirectory(config.DataPath)
	initializeServer(config.DataPath)
	db, err := initializeDatabase(config.DataPath)
	if err != nil {
		return nil, nil, err
	}

	searchIndex, err := initializeSearchIndex(config.DataPath)
	if err != nil {
		return nil, nil, err
	}

	return db, searchIndex, nil
}

// createDataDirectory creates the data directory and removes the temporary directory if it exists.
func createDataDirectory(dataPath string) error {
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return fmt.Errorf("error creating data directory: %w", err)
	}

	tmpDir := filepath.Join(dataPath, "web", "public", "tmp")
	if err := os.RemoveAll(tmpDir); err != nil {
		return err
	}

	return nil
}

// initializeServer initializes the server by setting up necessary directories and files.
func initializeServer(dataPath string) error {
	if _, err := initServer(dataPath); err != nil {
		return err
	}

	log.Println("Server initialized")

	return nil
}

// initializeDatabase initializes the SQLite database and performs auto-migration.
func initializeDatabase(dataPath string) (*SQLiteDB, error) {
	dbPath := filepath.Join(dataPath, "database.sqlite")
	dbExists := fileExists(dbPath)

	db, err := NewSQLiteDB(dataPath)
	if err != nil {
		return nil, err
	}

	if !dbExists {
		err = db.AutoMigrate(
			&ModelParams{},
			&ImageModel{},
			&SelectedModels{},
			&Chat{},
			&URLTracking{},
		)
		if err != nil {
			return nil, err
		}
		log.Println("Database created and migrated")
	} else {
		log.Println("Existing database found")
	}

	return db, nil
}

// initializeSearchIndex initializes the search index using Bleve.
func initializeSearchIndex(dataPath string) (*bleve.Index, error) {
	searchDB := filepath.Join(dataPath, "search.bleve")

	if _, err := os.Stat(searchDB); os.IsNotExist(err) {
		mapping := bleve.NewIndexMapping()
		searchIndex, err := bleve.New(searchDB, mapping)
		if err != nil {
			return nil, err
		}

		return &searchIndex, nil
	} else {
		searchIndex, err := bleve.Open(searchDB)
		if err != nil {
			return nil, err
		}

		return &searchIndex, nil
	}
}

// downloadDefaultImageModel downloads the default image model based on the configuration.
// func downloadDefaultImageModel(config *AppConfig) {
// 	if err := DownloadDefaultImageModel(config); err != nil {
// 		return fmt.Errorf("Failed to download default image model:", err)
// 	}
// }

// initServer initializes the server by setting up necessary directories and files.
func initServer(configPath string) (string, error) {
	if err := setupDirectory(configPath, "web", "public"); err != nil {
		return "", err
	}

	if err := setupDirectory(configPath, "gguf", "pkg/llm/local/bin"); err != nil {
		return "", err
	}

	// if err := setupComfyUI(configPath); err != nil {
	// 	return "", err
	// }

	// if err := setupComfyUIEssentials(configPath); err != nil {
	// 	return "", err
	// }

	// if err := setupImpactPack(configPath); err != nil {
	// 	return "", err
	// }

	// if err := setupComfyUImtb(configPath); err != nil {
	// 	return "", err
	// }

	// Needs work, finish in future commit
	// if err := setupKolors(configPath); err != nil {
	// 	return "", err
	// }

	return configPath, nil
}

// setupDirectory creates a directory and copies files into it.
func setupDirectory(configPath, dirName, srcDir string) error {
	dirPath := filepath.Join(configPath, dirName)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}

	if err := copyFiles(embedFS, srcDir, dirPath); err != nil {
		return fmt.Errorf("failed to copy files to %s: %w", dirPath, err)
	}

	return setExecutablePermissions(dirPath)
}

// installPythonRequirements installs Python requirements from a requirements.txt file.
func installPythonRequirements(reqPath string) error {
	cmd := exec.Command("pip3", "install", "-r", reqPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// setExecutablePermissions sets executable permissions on all files in a directory.
func setExecutablePermissions(dirPath string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		return os.Chmod(path, 0755)
	})
}

func copyFiles(fsys embed.FS, srcDir, destDir string) error {
	fileEntries, err := fsys.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range fileEntries {
		srcPath := filepath.Join(srcDir, entry.Name())
		destPath := filepath.Join(destDir, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			if err := copyFiles(fsys, srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(fsys, srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(fsys embed.FS, srcPath, destPath string) error {
	fileData, err := fsys.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, fileData, 0644)
}
