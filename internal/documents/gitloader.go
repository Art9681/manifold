package documents

import (
	"fmt"
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	golangssh "golang.org/x/crypto/ssh"
)

type GitLoader struct {
	RepoPath           string
	CloneURL           string
	Branch             string
	PrivateKeyPath     string
	FileFilter         func(string) bool
	InsecureSkipVerify bool
	DocumentManager    *DocumentManager
	IndexManager       *IndexManager
}

func NewGitLoader(repoPath, cloneURL, branch, privateKeyPath string, fileFilter func(string) bool, insecureSkipVerify bool, dm *DocumentManager, im *IndexManager) *GitLoader {
	return &GitLoader{
		RepoPath:           repoPath,
		CloneURL:           cloneURL,
		Branch:             branch,
		PrivateKeyPath:     privateKeyPath,
		FileFilter:         fileFilter,
		InsecureSkipVerify: insecureSkipVerify,
		DocumentManager:    dm,
		IndexManager:       im,
	}
}

// Load loads the documents from the Git repository specified by the GitLoader.
func (gl *GitLoader) Load() error {
	var err error

	// Clone or open the repository
	if _, err = os.Stat(gl.RepoPath); os.IsNotExist(err) && gl.CloneURL != "" {
		var auth *gitssh.PublicKeys
		// Only set up SSH authentication if PrivateKeyPath is provided
		if gl.PrivateKeyPath != "" {
			sshKey, _ := os.ReadFile(gl.PrivateKeyPath)
			signer, _ := golangssh.ParsePrivateKey(sshKey)
			auth = &gitssh.PublicKeys{User: "git", Signer: signer}
			if gl.InsecureSkipVerify {
				auth.HostKeyCallback = golangssh.InsecureIgnoreHostKey()
			}
		}

		// Clone the repository; omit the Auth field if auth is nil
		cloneOptions := &gogit.CloneOptions{
			URL: gl.CloneURL,
		}
		if auth != nil {
			cloneOptions.Auth = auth
		}

		_, err = gogit.PlainClone(gl.RepoPath, false, cloneOptions)
		if err != nil {
			return err
		}
	} else {
		_, err = gogit.PlainOpen(gl.RepoPath)
		if err != nil {
			return err
		}
	}

	// Walk through the files in the repository and ingest them into DocumentManager
	err = filepath.Walk(gl.RepoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Filter out non-text files based on file extension
		if !isTextFile(info.Name()) {
			return nil
		}

		if gl.FileFilter != nil && !gl.FileFilter(path) {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		textContent := string(content)
		relFilePath, _ := filepath.Rel(gl.RepoPath, path)
		fileType := filepath.Ext(info.Name())

		// Construct metadata
		metadata := map[string]string{
			"source":    relFilePath,
			"file_path": relFilePath,
			"file_name": info.Name(),
			"file_type": fileType,
		}

		// Use DocumentManager's method to determine the language
		language, err := getLanguageFromMetadata(metadata)
		if err == nil {
			metadata["language"] = string(language)
		}

		// Create Document and ingest it into DocumentManager
		doc := Document{PageContent: textContent, Metadata: metadata}
		gl.DocumentManager.IngestDocument(doc)

		// Index the full document content before splitting
		docID := metadata["file_path"]
		if err := gl.IndexManager.IndexFullDocument(docID, textContent, relFilePath); err != nil {
			return fmt.Errorf("failed to index full document %s: %w", docID, err)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error reading files: %s\n", err)
		return err
	}

	return nil
}

// validTextFileExtensions holds the set of file extensions that are considered text files.
var validTextFileExtensions = map[string]bool{
	".txt":  true,
	".md":   true,
	".go":   true,
	".py":   true,
	".js":   true,
	".ts":   true,
	".html": true,
	".json": true,
	".css":  true,
}

// isTextFile checks if a file is likely to be a text file based on its extension.
func isTextFile(filename string) bool {
	ext := filepath.Ext(filename)
	return validTextFileExtensions[ext]
}
