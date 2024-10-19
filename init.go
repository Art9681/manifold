// manifold/init.go

package main

import (
	"embed"
	"fmt"
	"io"
	"log"
	"manifold/internal/documents"
	"os"
	"os/exec"
	"path/filepath"
	//"github.com/blevesearch/bleve/v2"
)

// embedFS embeds the necessary files for the application.
//
//go:embed public/*
var embedFS embed.FS

// initializeApplication initializes the application with the given configuration.
func initializeApplication(config *Config) (*SQLiteDB, error) {
	createDataDirectory(config.DataPath)
	initializeServer(config.DataPath)
	db, err := initializeDatabase(config)
	if err != nil {
		return nil, err
	}

	// Initialize IndexManager
	indexPath := filepath.Join(config.DataPath, "searchindex")
	indexManager, err = documents.NewIndexManager(indexPath)
	if err != nil {
		fmt.Println("Failed to initialize index:", err)
		return nil, err
	}

	// Initialize the DocumentManager with chunk size, overlap size, and IndexManager
	docManager = documents.NewDocumentManager(2048, 0, indexManager)

	// searchIndex, err = initializeSearchIndex(config.DataPath)
	// if err != nil {
	// 	return nil, err
	// }

	// Print the number of documents in the search index
	// count, err := searchIndex.DocCount()
	// if err != nil {
	// 	return nil, err
	// }

	// fmt.Printf("Search index contains %d documents\n", count)

	// Initialize the edata database
	// err = edata.InitDB(filepath.Join(config.DataPath, "edata.db"))
	// if err != nil {
	// 	return nil, nil, fmt.Errorf("failed to initialize edata database: %w", err)
	// }

	return db, nil
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
func initializeDatabase(config *Config) (*SQLiteDB, error) {
	dbPath := filepath.Join(config.DataPath, "eternaldata.db") // Changed from "database.sqlite" to "eternaldata.db"
	dbExists := fileExists(dbPath)

	db, err := NewSQLiteDB(config.DataPath)
	if err != nil {
		return nil, err
	}

	// Enable SQLite extension loading if necessary
	if err := db.EnableSQLiteExtensionLoading(); err != nil {
		log.Fatalf("Failed to enable SQLite extensions: %v", err)
	}

	// Load and execute the sqlite-vec extension if needed
	// if err := db.LoadVecExtension(); err != nil {
	// 	log.Fatalf("Failed to load sqlite-vec extension: %v", err)
	// }

	if !dbExists {
		// Perform AutoMigrate for all relevant models
		err = db.AutoMigrate(
			&Chat{},
			&ToolMetadata{},
			&ToolParam{},
			&CompletionsRole{},
			&LanguageModel{},
			&SelectedModels{},
			&URLTracking{},
		)
		if err != nil {
			log.Fatal(err)
		}

		// Scan models directories
		ggufModels, err := ScanGGUFModels(config.DataPath)
		if err != nil {
			log.Fatalf("Failed to scan gguf models: %v", err)
		}

		mlxModels, err := ScanMLXModels(config.DataPath)
		if err != nil {
			log.Fatalf("Failed to scan mlx models: %v", err)
		}

		// Combine all models
		allModels := append(ggufModels, mlxModels...)

		config.LanguageModels = allModels

		// Synchronize models with the database
		if err := db.SyncModels(allModels); err != nil {
			log.Fatalf("Failed to synchronize models: %v", err)
		}

		// Load tools data into the database
		if err := loadToolsToDB(db, config.Tools); err != nil {
			log.Fatal(err)
		}

		// Load completions roles into the database
		if err := loadCompletionsRolesToDB(db, config.Roles); err != nil {
			log.Fatal(err)
		}

		// Create FTS5 table for full-text search on 'Prompt' and 'Response'
		err = db.db.Exec(`
			CREATE VIRTUAL TABLE IF NOT EXISTS chat_fts USING fts5(
				prompt,
				response,
				modelName,
				tokenize = "porter"
			);
        `).Error
		if err != nil {
			return nil, fmt.Errorf("failed to create FTS5 table: %v", err)
		}

		log.Println("Database created, migrated, and FTS5 table created")
	} else {
		log.Println("Existing database found")
	}

	return db, nil
}

// isDirEmpty checks whether a directory is empty
func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Read only one entry from the directory
	_, err = f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
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
