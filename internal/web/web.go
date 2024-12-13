package web

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"os"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/go-shiori/go-readability"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/pterm/pterm"
	nethtml "golang.org/x/net/html"
)

var (
	unwantedURLs = []string{
		"web.archive.org",
		"www.youtube.com",
		"www.youtube.com/watch",
		"www.wired.com",
		"www.techcrunch.com",
		"www.wsj.com",
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

	resultURLs []string
)

// CheckRobotsTxt checks if the target website allows scraping by "et-bot".
func checkRobotsTxt(ctx context.Context, u string) bool {
	baseURL, err := url.Parse(u)
	if err != nil {
		log.Printf("Failed to parse baseURL: %v", err)
		return false
	}

	robotsUrl := url.URL{Scheme: baseURL.Scheme, Host: baseURL.Host, Path: "/robots.txt"}
	resp, err := http.Get(robotsUrl.String())
	if err != nil {
		log.Printf("Failed to fetch robots.txt for %s: %v", baseURL.String(), err)
		return false
	}
	defer resp.Body.Close()

	// Check if the status code is 200
	if resp.StatusCode != 200 {
		log.Printf("Failed to fetch robots.txt for %s: %v", baseURL.String(), err)

		// We assume its allowed if not found
		return true
	}

	// Parse the robots.txt content if needed
	// Print the URL and the content of the robots.txt
	log.Printf("URL: %s\n", robotsUrl.String())
	return true
}

// WebGetHandler fetches the content of a webpage, extracts the main content, and returns it as Markdown.
func WebGetHandler(address string) (string, error) {
	if !checkRobotsTxt(context.Background(), address) {
		return "", fmt.Errorf("scraping not allowed according to robots.txt for %s", address)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var docs string
	err := chromedp.Run(ctx,
		chromedp.Navigate(address),
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
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &docs),
	)

	if err != nil {
		return "", fmt.Errorf("error retrieving page %s: %w", address, err)
	}

	// Convert url to url.URL
	getUrl, err := url.Parse(address)
	if err != nil {
		return "", fmt.Errorf("error parsing URL %s: %w", address, err)
	}

	article, err := readability.FromReader(strings.NewReader(docs), getUrl)
	if err != nil {
		return "", fmt.Errorf("error parsing reader view for %s: %w", address, err)
	}

	// Convert to Markdown
	markdownContent, err := htmlToMarkdown(article.Content)
	if err != nil {
		return "", fmt.Errorf("error converting HTML to Markdown for %s: %w", address, err)
	}

	// Append the source URL to the fetched content
	result := fmt.Sprintf("Source: %s\n\n%s", address, markdownContent)

	return result, nil
}

// htmlToMarkdown converts HTML content to Markdown.
func htmlToMarkdown(htmlContent string) (string, error) {
	// Configure the Markdown parser
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)

	// Configure HTML renderer options
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	renderer := html.NewRenderer(html.RendererOptions{Flags: htmlFlags})

	md := markdown.ToHTML([]byte(htmlContent), p, renderer)

	return string(md), nil
}

func ExtractURLs(input string) []string {
	urlRegex := `http.*?://[^\s<>{}|\\^` + "`" + `"]+`
	re := regexp.MustCompile(urlRegex)

	matches := re.FindAllString(input, -1)

	var cleanedURLs []string
	for _, match := range matches {
		cleanedURL := cleanURL(match)
		cleanedURLs = append(cleanedURLs, cleanedURL)
	}

	return cleanedURLs
}

func RemoveUrl(input []string) []string {
	urlRegex := `http.*?://[^\s<>{}|\\^` + "`" + `"]+`
	re := regexp.MustCompile(urlRegex)

	for i, str := range input {
		matches := re.FindAllString(str, -1)
		for _, match := range matches {
			input[i] = strings.ReplaceAll(input[i], match, "")
		}
	}

	return input
}

func cleanURL(url string) string {
	illegalTrailingChars := []rune{'.', ',', ';', '!', '?'}

	for _, char := range illegalTrailingChars {
		if url[len(url)-1] == byte(char) {
			url = url[:len(url)-1]
		}
	}

	return url
}

func SearchDDG(query string) []string {
	resultURLs = nil

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var nodes []*cdp.Node

	err := chromedp.Run(ctx,
		chromedp.Navigate(`https://lite.duckduckgo.com/lite/`),
		chromedp.WaitVisible(`input[name="q"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="q"]`, query+kb.Enter, chromedp.ByQuery),
		chromedp.Sleep(5*time.Second),
		chromedp.WaitVisible(`input[name="q"]`, chromedp.ByQuery),
		chromedp.Nodes(`a`, &nodes, chromedp.ByQueryAll),
	)
	if err != nil {
		log.Printf("Error during search: %v", err)
		return nil
	}

	err = chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			re, err := regexp.Compile(`^http[s]?://`)
			if err != nil {
				return err
			}

			uniqueUrls := make(map[string]bool)
			for _, n := range nodes {
				for _, attr := range n.Attributes {
					if re.MatchString(attr) && !strings.Contains(attr, "duckduckgo") {
						uniqueUrls[attr] = true
					}
				}
			}

			for u := range uniqueUrls {
				resultURLs = append(resultURLs, u)
			}

			return nil
		}),
	)

	if err != nil {
		log.Printf("Error processing results: %v", err)
		return nil
	}

	resultURLs = RemoveUnwantedURLs(resultURLs)

	// If resultURLs is contains cnn.com, replace the URL with https://lite.cnn.com
	for i, u := range resultURLs {
		if strings.Contains(u, "https://www.cnn.com") {
			resultURLs[i] = strings.Replace(u, "https://www.cnn.com", "https://lite.cnn.com", 1)
		}
	}

	pterm.Info.Println("Search results:", resultURLs)

	return resultURLs
}

func GetSearchResults(urls []string) string {
	var resultMarkdown string
	for _, url := range urls {
		res, err := WebGetHandler(url)
		if err != nil {
			pterm.Error.Printf("Error getting search result for URL %s: %v", url, err)
			continue
		}
		if res != "" {
			resultMarkdown += res + "\n\n" // Append to resultMarkdown with added newlines
		}
	}
	return resultMarkdown
}

func RemoveUnwantedURLs(urls []string) []string {
	var filteredURLs []string
	for _, u := range urls {
		pterm.Info.Printf("Checking URL: %s", u)

		unwanted := false
		for _, unwantedURL := range unwantedURLs {
			if strings.Contains(u, unwantedURL) {
				pterm.Info.Printf("URL %s contains unwanted URL %s", u, unwantedURL)
				unwanted = true
				break
			}
		}
		if !unwanted {
			filteredURLs = append(filteredURLs, u)
		}
	}

	pterm.Info.Printf("Filtered URLs: %v", filteredURLs)

	return filteredURLs
}

func GetPageScreen(chromeUrl string, pageAddress string) string {
	instanceUrl := chromeUrl

	allocatorCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), instanceUrl)
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

func RemoveUrls(input string) string {
	urlRegex := `http.*?://[^\s<>{}|\\^` + "`" + `"]+`
	re := regexp.MustCompile(urlRegex)

	matches := re.FindAllString(input, -1)

	for _, match := range matches {
		input = strings.ReplaceAll(input, match, "")
	}

	return input
}

func removeEmptyRows(input string) string {
	lines := strings.Split(input, "\n")
	var filteredLines []string
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" {
			filteredLines = append(filteredLines, trimmedLine)
		}
	}
	return strings.Join(filteredLines, "\n")
}

// postRequest sends a POST request to the given endpoint with a named parameter 'q' and returns the response body as a string.
func postRequest(endpoint string, queryParam string) (string, error) {
	// Create the form data
	formData := url.Values{}
	formData.Set("q", queryParam)

	// Convert form data to a byte buffer
	data := bytes.NewBufferString(formData.Encode())

	// Create a new POST request
	req, err := http.NewRequest("POST", endpoint, data)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set the appropriate headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the response body
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)

	return buf.String(), nil
}

// extractURLs parses the HTML response and extracts the URLs from the search results.
func extractURLs(htmlContent string) ([]string, error) {
	doc, err := nethtml.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var urls []string
	var f func(*nethtml.Node)
	f = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" && strings.Contains(attr.Val, "http") {
					urls = append(urls, attr.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return urls, nil
}

func GetSearXNGResults(endpoint string, query string) []string {
	htmlContent, err := postRequest(endpoint, query)
	if err != nil {
		pterm.Error.Printf("Error: %v\n", err)
		return nil
	}

	urls, err := extractURLs(htmlContent)
	if err != nil {
		pterm.Error.Printf("Error extracting URLs: %v\n", err)
		return nil
	}

	// Remove unwanted URLs
	urls = RemoveUnwantedURLs(urls)

	for i, u := range resultURLs {
		if strings.Contains(u, "https://www.cnn.com") {
			resultURLs[i] = strings.Replace(u, "https://www.cnn.com", "https://lite.cnn.com", 1)
		}
	}

	return urls
}
