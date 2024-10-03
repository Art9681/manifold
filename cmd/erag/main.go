package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

// Passage structure to store passage entries (No custom ID now)
type Passage struct {
	Passage string `json:"Passage"` // Match JSON field from Parquet data
}

// SQLiteDB structure to hold GORM DB object
type SQLiteDB struct {
	db *gorm.DB
}

// Initialize the SQLite database and perform auto-migration
func initializeDatabase(dataPath string) (*SQLiteDB, error) {
	dbPath := filepath.Join(dataPath, "eternaldata.db")
	dbExists := fileExists(dbPath)

	// Initialize the database connection
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // Enable detailed logs
	})
	if err != nil {
		return nil, err
	}

	// If the database doesn't exist, auto-migrate and create the FTS5 table
	if !dbExists {
		// Create the FTS5 virtual table for full-text search
		err = db.Exec(`
            CREATE VIRTUAL TABLE IF NOT EXISTS passage_fts USING fts5(
                passage
            );
        `).Error
		if err != nil {
			return nil, fmt.Errorf("failed to create FTS5 table: %v", err)
		}

		log.Println("Database created and FTS5 table created")
	} else {
		log.Println("Existing database found")
	}

	return &SQLiteDB{db: db}, nil
}

// Check if a file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// InsertPassage inserts a passage entry into the FTS5 table
func (sqldb *SQLiteDB) InsertPassage(ctx context.Context, passage Passage) error {
	// Insert the passage into the FTS5 table (SQLite automatically assigns rowid)
	err := sqldb.db.Exec(`
        INSERT INTO passage_fts(passage) 
        VALUES(?)
    `, passage.Passage).Error
	if err != nil {
		return fmt.Errorf("failed to insert into FTS5: %v", err)
	}

	return nil
}

// LoadParquetData loads data from the Parquet file, converts it to JSON, and inserts it into the FTS5 table
func (sqldb *SQLiteDB) LoadParquetData(ctx context.Context, parquetFilePath string) error {
	// Open the Parquet file
	fr, err := local.NewLocalFileReader(parquetFilePath)
	if err != nil {
		return fmt.Errorf("failed to open parquet file: %v", err)
	}
	defer fr.Close()

	// Create a new Parquet reader
	pr, err := reader.NewParquetReader(fr, nil, 4)
	if err != nil {
		return fmt.Errorf("can't create parquet reader: %v", err)
	}
	defer pr.ReadStop()

	// Get the number of rows in the Parquet file
	numRows := int(pr.GetNumRows())

	// Read all rows into a slice
	rows, err := pr.ReadByNumber(numRows)
	if err != nil {
		return fmt.Errorf("can't read parquet file: %v", err)
	}

	// Convert the rows to JSON
	jsonBs, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("failed to convert parquet data to JSON: %v", err)
	}

	// Print the JSON for debugging
	log.Println(string(jsonBs))

	// Unmarshal JSON data into Passage structs
	var passages []Passage
	err = json.Unmarshal(jsonBs, &passages)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON data: %v", err)
	}

	// Insert each passage into the FTS5 table
	for _, passage := range passages {
		err := sqldb.InsertPassage(ctx, passage)
		if err != nil {
			return fmt.Errorf("failed to insert passage: %v", err)
		}
	}

	log.Println("Parquet data inserted successfully")
	return nil
}

func main() {
	// Define the data path where the SQLite database will be stored
	dataPath := "/Users/arturoaquino/.manifold/datasets"

	// Define the path to the Parquet file
	parquetFilePath := "/Users/arturoaquino/.manifold/datasets/part.0.parquet"

	// Initialize the SQLite database
	db, err := initializeDatabase(dataPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Load and insert data from the Parquet file into the SQLite database
	ctx := context.Background()
	err = db.LoadParquetData(ctx, parquetFilePath)
	if err != nil {
		log.Fatalf("Failed to load data from Parquet file: %v", err)
	}

	log.Println("Database setup and Parquet data insertion complete")
}
