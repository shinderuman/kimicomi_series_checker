package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/slack-go/slack"
	"golang.org/x/net/html"
)

type SeriesData struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type StoredData struct {
	Series []SeriesData `json:"series"`
}

type Config struct {
	S3BucketName  string `json:"S3BucketName"`
	S3ObjectKey   string `json:"S3ObjectKey"`
	S3Region      string `json:"S3Region"`
	SlackBotToken string `json:"SlackBotToken"`
	SlackChannel  string `json:"SlackChannel"`
}

var appConfig Config

var days = []string{"月", "火", "水", "木", "金", "土", "日", "その他"}

func main() {
	if err := initConfig(); err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	if isLambda() {
		lambda.Start(handler)
	} else {
		if _, err := handler(context.Background()); err != nil {
			log.Fatalf("Error: %v", err)
		}
	}
}

func initConfig() error {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &appConfig); err != nil {
		return err
	}

	return nil
}

func isLambda() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

func handler(ctx context.Context) (string, error) {
	result, err := processSeriesCheck(ctx)
	if err != nil {
		alertToSlack(err)
		return "", err
	}
	return result, nil
}

func processSeriesCheck(_ context.Context) (string, error) {
	log.Println("Starting kimicomi series checker")

	cfg, err := initAWSConfig()
	if err != nil {
		return "", fmt.Errorf("failed to init AWS config: %w", err)
	}

	currentSeries, err := fetchAllSeries()
	if err != nil {
		return "", fmt.Errorf("failed to fetch series: %w", err)
	}

	previousSeries, err := loadPreviousData(cfg)
	if err != nil {
		log.Printf("Warning: failed to load previous data: %v", err)
		previousSeries = []SeriesData{}
	}

	added, removed := compareSeries(previousSeries, currentSeries)

	if len(added) > 0 || len(removed) > 0 {
		err = postToSlack(buildSlackMessage(added, removed))
		if err != nil {
			return "", fmt.Errorf("failed to notify Slack: %w", err)
		}
	} else {
		log.Println("No changes detected")
	}

	err = saveCurrentData(cfg, currentSeries)
	if err != nil {
		return "", fmt.Errorf("failed to save current data: %w", err)
	}

	log.Println("Completed successfully")
	return "Processing complete", nil
}

func initAWSConfig() (aws.Config, error) {
	return config.LoadDefaultConfig(context.Background(), config.WithRegion(appConfig.S3Region))
}

func fetchAllSeries() ([]SeriesData, error) {
	seriesMap := make(map[string]SeriesData)

	for _, day := range days {
		series, err := fetchSeriesForDay(day)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch series for %s: %w", day, err)
		}

		log.Printf("Fetched %d series for %s", len(series), day)

		for _, s := range series {
			seriesMap[s.ID] = s
		}
	}

	result := make([]SeriesData, 0, len(seriesMap))
	for _, s := range seriesMap {
		result = append(result, s)
	}

	log.Printf("Total unique series: %d", len(result))

	return result, nil
}

func fetchSeriesForDay(day string) ([]SeriesData, error) {
	baseURL := "https://kimicomi.com/category/manga"
	params := url.Values{}
	params.Add("type", "連載中")
	params.Add("day", day)
	fullURL := baseURL + "?" + params.Encode()

	resp, err := httpGet(fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return extractSeriesFromHTML(string(body))
}

func httpGet(url string) (*http.Response, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL %s: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code %d for URL %s", resp.StatusCode, url)
	}

	return resp, nil
}

func extractSeriesFromHTML(htmlContent string) ([]SeriesData, error) {
	seriesMap := make(map[string]SeriesData)

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			// Check if this is a series link
			href := ""
			for _, attr := range n.Attr {
				if attr.Key == "href" && strings.HasPrefix(attr.Val, "https://kimicomi.com/series/") {
					href = attr.Val
					break
				}
			}

			if href != "" {
				// Extract series ID from URL
				id := strings.TrimPrefix(href, "https://kimicomi.com/series/")
				if id == "" {
					// continue traversal
				} else {
					// Look for title in the child elements
					title := findTitleInNode(n)
					if title != "" {
						seriesMap[id] = SeriesData{
							ID:    id,
							URL:   href,
							Title: title,
						}
					}
				}
			}
		}

		// Traverse child nodes
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)

	result := make([]SeriesData, 0, len(seriesMap))
	for _, s := range seriesMap {
		result = append(result, s)
	}

	return result, nil
}

func findTitleInNode(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "div" {
		for _, attr := range n.Attr {
			if attr.Key == "class" && attr.Val == "title-text" {
				// Get the text content of this div
				return getTextContent(n)
			}
		}
	}

	// Search recursively in child nodes
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := findTitleInNode(c); title != "" {
			return title
		}
	}

	return ""
}

func getTextContent(n *html.Node) string {
	var text strings.Builder
	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.TextNode {
			text.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(n)
	return strings.TrimSpace(text.String())
}

func compareSeries(previous, current []SeriesData) (added, removed []SeriesData) {
	prevMap := make(map[string]SeriesData)
	for _, s := range previous {
		prevMap[s.ID] = s
	}

	currMap := make(map[string]SeriesData)
	for _, s := range current {
		currMap[s.ID] = s
	}

	for id, s := range currMap {
		if _, exists := prevMap[id]; !exists {
			added = append(added, s)
		}
	}

	for id, s := range prevMap {
		if _, exists := currMap[id]; !exists {
			removed = append(removed, s)
		}
	}

	return added, removed
}

func loadPreviousData(cfg aws.Config) ([]SeriesData, error) {
	client := s3.NewFromConfig(cfg)

	result, err := client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(appConfig.S3BucketName),
		Key:    aws.String(appConfig.S3ObjectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object body: %w", err)
	}

	var stored StoredData
	err = json.Unmarshal(body, &stored)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return stored.Series, nil
}

func saveCurrentData(cfg aws.Config, series []SeriesData) error {
	stored := StoredData{
		Series: series,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	_, err = client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(appConfig.S3BucketName),
		Key:    aws.String(appConfig.S3ObjectKey),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("failed to put object to S3: %w", err)
	}

	return nil
}

func buildSlackMessage(added, removed []SeriesData) string {
	var msg strings.Builder

	msg.WriteString("キミコミ連載情報の変更を検出しました\n\n")

	if len(added) > 0 {
		msg.WriteString("*【新規連載】*\n")
		for _, s := range added {
			msg.WriteString(fmt.Sprintf("* <%s|%s>\n", s.URL, s.Title))
		}
		msg.WriteString("\n")
	}

	if len(removed) > 0 {
		msg.WriteString("*【削除された連載】*\n")
		for _, s := range removed {
			msg.WriteString(fmt.Sprintf("* <%s|%s>\n", s.URL, s.Title))
		}
	}

	return msg.String()
}

func alertToSlack(err error) {
	message := fmt.Sprintf("キミコミチェッカーエラー\n```%v```", err)
	if postErr := postToSlack(message); postErr != nil {
		log.Printf("Failed to send error to Slack: %v", postErr)
	}
}

func postToSlack(message string) error {
	api := slack.New(appConfig.SlackBotToken)

	_, _, err := api.PostMessage(
		appConfig.SlackChannel,
		slack.MsgOptionText(message, false),
	)
	return err
}
