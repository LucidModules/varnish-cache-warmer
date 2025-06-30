package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	maxWorkers     = 3
	requestTimeout = 30 * time.Second
	maxRetries     = 3
	retryDelay     = 2 * time.Second
)

var urlsToWarm = []string{
	"/",
	"/customer/account",
	"/sofas.html",
}

type WarmResult struct {
	URL     string
	Success bool
	Error   error
	Cache   string
}

func main() {
	varnishBaseURL := os.Getenv("VARNISH_BASE_URL")
	if varnishBaseURL == "" {
		log.Fatal("VARNISH_BASE_URL environment variable is required")
	}

	log.Printf("Starting cache warming for %s", varnishBaseURL)

	if err := testConnectivity(varnishBaseURL); err != nil {
		log.Fatalf("Cannot connect to Varnish: %v", err)
	}
	log.Println("✓ Varnish is responding")

	results := warmURLsConcurrently(varnishBaseURL, urlsToWarm)

	successCount := 0
	for _, result := range results {
		if result.Success {
			log.Printf("✓ Warmed: %s (Cache: %s)", result.URL, result.Cache)
			successCount++
		} else {
			log.Printf("✗ Failed: %s - %v", result.URL, result.Error)
		}
	}

	log.Printf("\nCache warming completed: %d/%d URLs", successCount, len(urlsToWarm))

	if successCount < len(urlsToWarm) {
		log.Println("WARNING: Some URLs failed to warm")
		os.Exit(1)
	}

	log.Println("All URLs warmed successfully!")
}

func testConnectivity(baseURL string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func warmURLsConcurrently(baseURL string, urls []string) []WarmResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	jobs := make(chan string, len(urls))
	results := make(chan WarmResult, len(urls))

	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(ctx, baseURL, jobs, results, &wg)
	}

	for _, url := range urls {
		jobs <- url
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var allResults []WarmResult
	for result := range results {
		allResults = append(allResults, result)
	}

	return allResults
}

func worker(ctx context.Context, baseURL string, jobs <-chan string, results chan<- WarmResult, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{
		Timeout: requestTimeout,
	}

	for url := range jobs {
		select {
		case <-ctx.Done():
			results <- WarmResult{
				URL:     url,
				Success: false,
				Error:   ctx.Err(),
			}
			return
		default:
			result := warmURL(client, baseURL, url)
			results <- result
		}
	}
}

func warmURL(client *http.Client, baseURL, url string) WarmResult {
	fullURL := baseURL + url

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("GET", fullURL, nil)
		if err != nil {
			if attempt == maxRetries {
				return WarmResult{
					URL:     url,
					Success: false,
					Error:   fmt.Errorf("failed to create request: %w", err),
				}
			}
			time.Sleep(retryDelay)
			continue
		}

		req.Header.Set("User-Agent", "VarnishWarmer/1.0")
		req.Header.Set("X-Cache-Warm", "true")
		req.Header.Set("Cache-Control", "no-cache")

		resp, err := client.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return WarmResult{
					URL:     url,
					Success: false,
					Error:   fmt.Errorf("request failed after %d attempts: %w", maxRetries, err),
				}
			}
			log.Printf("⚠ Retry %d/%d for %s: %v", attempt, maxRetries, url, err)
			time.Sleep(retryDelay)
			continue
		}

		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			cacheStatus := resp.Header.Get("X-Varnish-Cache")
			if cacheStatus == "" {
				cacheStatus = "unknown"
			}
			return WarmResult{
				URL:     url,
				Success: true,
				Cache:   cacheStatus,
			}
		}

		if attempt == maxRetries {
			return WarmResult{
				URL:     url,
				Success: false,
				Error:   fmt.Errorf("HTTP %d after %d attempts", resp.StatusCode, maxRetries),
			}
		}

		log.Printf("⚠ Retry %d/%d for %s: HTTP %d", attempt, maxRetries, url, resp.StatusCode)
		time.Sleep(retryDelay)
	}

	return WarmResult{
		URL:     url,
		Success: false,
		Error:   fmt.Errorf("unexpected end of retry loop"),
	}
}
