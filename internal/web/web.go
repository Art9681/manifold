// internal/web/web.go

package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/go-shiori/go-readability"
)

// List of unwanted URLs to filter out from search results
var unwantedURLs = []string{
	"web.archive.org",
	"www.youtube.com",
	"www.youtube.com/watch",
	"www.wired.com",
	"www.techcrunch.com",
	"www.wsj.com",
	"www.cnn.com",
	"www.nytimes.com",
	"www.forbes.com",
	"www.businessinsider.com",
	"www.theverge.com",
	"www.thehill.com",
	"www.theatlantic.com",
	"www.foxnews.com",
	"www.theguardian.com",
	"www.nbcnews.com",
	"www.msn.com",
	"www.sciencedaily.com",
	"reuters.com",
	"bbc.com",
	"thenewstack.io",
	"abcnews.go.com",
	"apnews.com",
	"bloomberg.com",
	"polygon.com",
	"reddit.com",
	"indeed.com",
	"test.com",
	// Add more URLs to block from search results
}

var resultURLs []string

// ChromePool manages a pool of Chrome contexts for concurrent fetching.
type ChromePool struct {
	pool chan context.Context
	mu   sync.Mutex
}

// Get retrieves a Chrome context from the pool.
func (p *ChromePool) Get() context.Context {
	return <-p.pool
}

// Put returns a Chrome context back to the pool.
func (p *ChromePool) Put(ctx context.Context) {
	p.pool <- ctx
}

// Global ChromePool instance (adjust pool size as needed)
var chromePool *ChromePool

// NewChromePool creates a new ChromePool with the given size.
func NewChromePool(size int) (*ChromePool, error) {
	pool := make(chan context.Context, size)
	for i := 0; i < size; i++ {
		allocatorCtx, cancel := chromedp.NewExecAllocator(context.Background(), chromedp.Flag("headless", true))
		ctx, cancel := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Printf))
		pool <- ctx
		cancel()
	}
	return &ChromePool{pool: pool}, nil
}

func init() {
	var err error
	chromePool, err = NewChromePool(5) // Pool size of 5
	if err != nil {
		log.Fatalf("Failed to create Chrome pool: %v", err)
	}
}

// CheckRobotsTxt checks if the target website allows scraping by "et-bot".
func CheckRobotsTxt(ctx context.Context, u string) bool {
	baseURL, err := url.Parse(u)
	if err != nil {
		log.Printf("Failed to parse baseURL: %v", err)
		return false
	}

	robotsURL := url.URL{Scheme: baseURL.Scheme, Host: baseURL.Host, Path: "/robots.txt"}
	resp, err := http.Get(robotsURL.String())
	if err != nil {
		log.Printf("Failed to fetch robots.txt for %s: %v", baseURL.String(), err)
		return false
	}
	defer resp.Body.Close()

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		log.Printf("robots.txt not found for %s. Assuming scraping is allowed.", baseURL.String())
		return true
	}

	// TODO: Parse robots.txt content to check for disallowed paths for "et-bot"
	// Currently, we assume it's allowed
	log.Printf("robots.txt found for %s. Assuming scraping is allowed.", baseURL.String())
	return true
}

// WebGetHandler fetches and processes the content of a web page.
func WebGetHandler(parentCtx context.Context, address string) (string, error) {
	if !CheckRobotsTxt(parentCtx, address) {
		return "", errors.New("scraping not allowed according to robots.txt")
	}

	// Get a Chrome context from the pool
	ctx := chromePool.Get()
	defer chromePool.Put(ctx)

	// Create a child context with timeout
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var docs string
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			headers := map[string]interface{}{
				"User-Agent":      "et-bot", // Set user agent to et-bot
				"Referer":         "https://www.duckduckgo.com/",
				"Accept-Language": "en-US,en;q=0.9",
				"X-Forwarded-For": "203.0.113.195",
				"Accept-Encoding": "gzip, deflate, br",
				"Connection":      "keep-alive",
				"DNT":             "1",
			}
			return network.SetExtraHTTPHeaders(network.Headers(headers)).Do(ctx)
		}),
		chromedp.Navigate(address),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &docs, chromedp.ByQuery),
	)

	if err != nil {
		log.Println("Error retrieving page:", err)
		return "", err
	}

	// Parse the HTML content using readability
	getURL, err := url.Parse(address)
	if err != nil {
		log.Println("Error parsing URL:", err)
		return "", err
	}

	article, err := readability.FromReader(strings.NewReader(docs), getURL)
	if err != nil {
		log.Println("Error parsing reader view:", err)
		return "", err
	}

	// Use goquery to extract text
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(article.Content))
	if err != nil {
		log.Println("Error parsing document:", err)
		return "", err
	}

	text := doc.Find("body").Text()
	text = RemoveEmptyRows(text)

	return text, nil
}

// ExtractURLs extracts URLs from a given input string using regex.
func ExtractURLs(input string) []string {
	urlRegex := `http[s]?://[^\s<>{}|\\^` + "`" + `"]+`
	re := regexp.MustCompile(urlRegex)

	matches := re.FindAllString(input, -1)

	var cleanedURLs []string
	for _, match := range matches {
		cleanedURL := CleanURL(match)
		cleanedURLs = append(cleanedURLs, cleanedURL)
	}

	return cleanedURLs
}

// CleanURL removes illegal trailing characters from a URL.
func CleanURL(urlStr string) string {
	illegalTrailingChars := []rune{'.', ',', ';', '!', '?'}

	for len(urlStr) > 0 {
		trimmed := false
		for _, char := range illegalTrailingChars {
			if urlStr[len(urlStr)-1] == byte(char) {
				urlStr = urlStr[:len(urlStr)-1]
				trimmed = true
			}
		}
		if !trimmed {
			break
		}
	}

	return urlStr
}

// RemoveUnwantedURLs filters out URLs that match unwanted patterns.
func RemoveUnwantedURLs(urls []string) []string {
	var filteredURLs []string
	for _, u := range urls {
		log.Printf("Checking URL: %s", u)

		unwanted := false
		for _, unwantedURL := range unwantedURLs {
			if strings.Contains(u, unwantedURL) {
				log.Printf("URL %s contains unwanted URL %s", u, unwantedURL)
				unwanted = true
				break
			}
		}
		if !unwanted {
			filteredURLs = append(filteredURLs, u)
		}
	}

	log.Printf("Filtered URLs: %v", filteredURLs)

	return filteredURLs
}

// GetSearXNGResults performs a search and retrieves the result URLs.
func GetSearXNGResults(endpoint string, query string) []string {
	htmlContent, err := PostRequest(endpoint, query)
	if err != nil {
		log.Printf("Error performing request: %v\n", err)
		return nil
	}

	urls := ExtractURLs(htmlContent)

	// Remove unwanted URLs
	urls = RemoveUnwantedURLs(urls)

	return urls
}

// PostRequest sends a POST request to the given endpoint with a named parameter 'q' and returns the response body as a string.
// PostRequest sends a POST request to the given endpoint with a named parameter 'q' and returns the response body as a string.
func PostRequest(endpoint string, queryParam string) (string, error) {
	// Create the form data
	formData := url.Values{}
	formData.Set("q", queryParam)

	// Convert form data to a byte buffer
	data := strings.NewReader(formData.Encode())

	// Create a new POST request
	req, err := http.NewRequest("POST", endpoint, data)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set the appropriate headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Perform the request
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the response body using bytes.Buffer
	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return buf.String(), nil
}

// GetSearchResults fetches content from a list of URLs.
func GetSearchResults(urls []string) string {
	var resultHTML strings.Builder

	for _, url := range urls {
		res, err := WebGetHandler(context.Background(), url)
		if err != nil {
			log.Printf("Error getting search result: %v", err)
			continue
		}

		if res != "" {
			resultHTML.WriteString(res)
			resultHTML.WriteString("\n")
		}
	}

	return resultHTML.String()
}

// RemoveEmptyRows removes empty lines from the input string.
func RemoveEmptyRows(input string) string {
	lines := strings.Split(input, "\n")
	var filteredLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filteredLines = append(filteredLines, line)
		}
	}

	return strings.Join(filteredLines, "\n")
}

// GetPageScreen captures a screenshot of the given page.
func GetPageScreen(chromeURL string, pageAddress string) string {
	allocatorCtx, cancel := chromedp.NewExecAllocator(context.Background(), chromedp.Flag("headless", true))
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(pageAddress),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		log.Fatal(err)
	}

	u, err := url.Parse(pageAddress)
	if err != nil {
		log.Fatal(err)
	}

	t := time.Now()
	filename := u.Hostname() + "-" + t.Format("20060102150405") + ".png"

	err = os.WriteFile(filename, buf, 0644)
	if err != nil {
		log.Fatal(err)
	}

	return filename
}
