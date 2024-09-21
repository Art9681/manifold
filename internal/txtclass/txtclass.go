package txtclass

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// ClassificationResult represents the structure of the classification output.
type ClassificationResult struct {
	Sequence string           `json:"sequence"`
	Labels   []string         `json:"labels"`
	Scores   []float64        `json:"scores"`
	Ranked   []LabelWithScore `json:"ranked_labels,omitempty"`
}

// LabelWithScore pairs a label with its corresponding score.
type LabelWithScore struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

// Classify performs zero-shot classification by invoking a Python script.
// It takes the text to classify and a slice of candidate labels.
// Returns the classification result or an error.
func Classify(sequence string, candidateLabels []string) (*ClassificationResult, error) {
	// Define the Python script
	pythonScript := `
import sys
import json
from transformers import pipeline

def main():
    try:
        # Read input from stdin
        input_data = sys.stdin.read()
        data = json.loads(input_data)
        sequence = data['sequence']
        candidate_labels = data['labels']

        # Initialize the zero-shot classification pipeline
        classifier = pipeline(
            "zero-shot-classification",
            model="knowledgator/comprehend_it-base",
            device=-1  # Use GPU if available; set to -1 for CPU
        )

        # Perform zero-shot classification
        result = classifier(sequence, candidate_labels)

        # Output the result as JSON
        print(json.dumps(result))
    except Exception as e:
        # Print the error message to stderr
        print(str(e), file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
`

	// Prepare the input JSON
	input := map[string]interface{}{
		"sequence": sequence,
		"labels":   candidateLabels,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input JSON: %w", err)
	}

	// Execute the Python script
	cmd := exec.Command("python3", "-c", pythonScript)
	cmd.Stdin = bytes.NewBuffer(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("python script error: %v, stderr: %s", err, stderr.String())
	}

	// Parse the output JSON
	var result ClassificationResult
	err = json.Unmarshal(stdout.Bytes(), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal output JSON: %w, output: %s", err, stdout.String())
	}

	// Optionally, rank the labels based on scores
	if len(result.Labels) != len(result.Scores) {
		return nil, errors.New("mismatch between number of labels and scores")
	}

	for i, label := range result.Labels {
		result.Ranked = append(result.Ranked, LabelWithScore{
			Label: label,
			Score: result.Scores[i],
		})
	}

	// Sort the Ranked slice by score in descending order
	for i := 0; i < len(result.Ranked)-1; i++ {
		for j := 0; j < len(result.Ranked)-i-1; j++ {
			if result.Ranked[j].Score < result.Ranked[j+1].Score {
				result.Ranked[j], result.Ranked[j+1] = result.Ranked[j+1], result.Ranked[j]
			}
		}
	}

	return &result, nil
}
