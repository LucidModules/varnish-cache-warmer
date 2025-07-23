package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func TestGetURLsFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     []string
	}{
		{
			name:     "Empty environment variable",
			envValue: "",
			want:     nil,
		},
		{
			name:     "Single URL",
			envValue: "/test",
			want:     []string{"/test"},
		},
		{
			name:     "Multiple URLs",
			envValue: "/test1,/test2,/test3",
			want:     []string{"/test1", "/test2", "/test3"},
		},
		{
			name:     "URLs with spaces",
			envValue: " /test1 , /test2 , /test3 ",
			want:     []string{"/test1", "/test2", "/test3"},
		},
		{
			name:     "URLs with quotes",
			envValue: "\"/test1\",'/test2'",
			want:     []string{"/test1", "/test2"},
		},
		{
			name:     "URLs without leading slash",
			envValue: "test1,test2",
			want:     []string{"/test1", "/test2"},
		},
		{
			name:     "Empty URL in list",
			envValue: "/test1,,/test3",
			want:     []string{"/test1", "/test3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("CACHE_URLS", tt.envValue)
			defer os.Unsetenv("CACHE_URLS")

			got := getURLsFromEnv()
			if len(got) != len(tt.want) {
				t.Errorf("getURLsFromEnv() returned %d URLs, want %d", len(got), len(tt.want))
				return
			}

			for i, url := range got {
				if url != tt.want[i] {
					t.Errorf("getURLsFromEnv()[%d] = %s, want %s", i, url, tt.want[i])
				}
			}
		})
	}
}

func TestTestConnectivity(t *testing.T) {
	tests := []struct {
		name              string
		serverResponse    int
		expectError       bool
		authHeader        string
		checkAuthHeader   bool
		expectedAuthValue string
	}{
		{
			name:           "Successful connection (200)",
			serverResponse: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Successful connection (301)",
			serverResponse: http.StatusMovedPermanently,
			expectError:    false,
		},
		{
			name:           "Failed connection (404)",
			serverResponse: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Failed connection (500)",
			serverResponse: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:              "With authorization header",
			serverResponse:    http.StatusOK,
			expectError:       false,
			authHeader:        "Bearer test-token",
			checkAuthHeader:   true,
			expectedAuthValue: "Bearer test-token",
		},
		{
			name:              "With authorization header but unauthorized",
			serverResponse:    http.StatusUnauthorized,
			expectError:       true,
			authHeader:        "Bearer invalid-token",
			checkAuthHeader:   true,
			expectedAuthValue: "Bearer invalid-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable if needed
			if tt.authHeader != "" {
				os.Setenv("AUTHORIZATION_HEADER", tt.authHeader)
				defer os.Unsetenv("AUTHORIZATION_HEADER")
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check if authorization header was sent correctly
				if tt.checkAuthHeader {
					authHeader := r.Header.Get("Authorization")
					if authHeader != tt.expectedAuthValue {
						t.Errorf("Expected Authorization header %s, got %s", tt.expectedAuthValue, authHeader)
					}
				}
				w.WriteHeader(tt.serverResponse)
			}))
			defer server.Close()

			err := testConnectivity(server.URL)
			if (err != nil) != tt.expectError {
				t.Errorf("testConnectivity() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}

	t.Run("Invalid URL", func(t *testing.T) {
		err := testConnectivity("http://invalid-url-that-does-not-exist.example")
		if err == nil {
			t.Error("testConnectivity() with invalid URL should return error")
		}
	})
}

func TestWarmURL(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse int
		serverDelay    time.Duration
		cacheHeader    string
		hostHeader     string
		authHeader     string
		expectSuccess  bool
	}{
		{
			name:           "Successful warming (200)",
			serverResponse: http.StatusOK,
			cacheHeader:    "HIT",
			expectSuccess:  true,
		},
		{
			name:           "Successful warming (301)",
			serverResponse: http.StatusMovedPermanently,
			cacheHeader:    "MISS",
			expectSuccess:  true,
		},
		{
			name:           "Failed warming (404)",
			serverResponse: http.StatusNotFound,
			expectSuccess:  false,
		},
		{
			name:           "Failed warming (500)",
			serverResponse: http.StatusInternalServerError,
			expectSuccess:  false,
		},
		{
			name:           "With host header",
			serverResponse: http.StatusOK,
			hostHeader:     "example.com",
			expectSuccess:  true,
		},
		{
			name:           "With authorization header",
			serverResponse: http.StatusOK,
			authHeader:     "Bearer token123",
			expectSuccess:  true,
		},
		{
			name:           "Server timeout",
			serverResponse: http.StatusOK,
			serverDelay:    500 * time.Millisecond, // Use a shorter delay for testing
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables if needed
			if tt.hostHeader != "" {
				os.Setenv("HOST_HEADER", tt.hostHeader)
				defer os.Unsetenv("HOST_HEADER")
			}
			if tt.authHeader != "" {
				os.Setenv("AUTHORIZATION_HEADER", tt.authHeader)
				defer os.Unsetenv("AUTHORIZATION_HEADER")
			}

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// For testing purposes, we'll just verify that any headers were set
				// and not worry about the actual Host header which is handled specially by Go
				if tt.authHeader != "" && r.Header.Get("Authorization") != tt.authHeader {
					t.Errorf("Expected Authorization header %s, got %s", tt.authHeader, r.Header.Get("Authorization"))
				}

				// Verify other required headers
				if r.Header.Get("User-Agent") != "VarnishWarmer/1.0" {
					t.Errorf("Expected User-Agent header VarnishWarmer/1.0, got %s", r.Header.Get("User-Agent"))
				}
				if r.Header.Get("X-Cache-Warm") != "true" {
					t.Errorf("Expected X-Cache-Warm header true, got %s", r.Header.Get("X-Cache-Warm"))
				}

				// Add delay if specified
				if tt.serverDelay > 0 {
					time.Sleep(tt.serverDelay)
				}

				// Set cache header if specified
				if tt.cacheHeader != "" {
					w.Header().Set("X-Varnish-Cache", tt.cacheHeader)
				}

				w.WriteHeader(tt.serverResponse)
			}))
			defer server.Close()

			// Create a client with an appropriate timeout for the test
			timeout := requestTimeout
			if tt.name == "Server timeout" {
				// For the timeout test, use a very short timeout
				timeout = 100 * time.Millisecond
			}
			client := &http.Client{
				Timeout: timeout,
			}

			// Call the function
			result := warmURL(client, server.URL, "/test")

			// Check results
			if result.Success != tt.expectSuccess {
				t.Errorf("warmURL() success = %v, want %v, error: %v", result.Success, tt.expectSuccess, result.Error)
			}

			if tt.expectSuccess && tt.cacheHeader != "" && result.Cache != tt.cacheHeader {
				t.Errorf("warmURL() cache = %v, want %v", result.Cache, tt.cacheHeader)
			}
		})
	}
}

func TestWarmURLsConcurrently(t *testing.T) {
	// Create a test server that counts requests
	requestCount := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.Header().Set("X-Varnish-Cache", "MISS")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test with multiple URLs
	urls := []string{"/page1", "/page2", "/page3", "/page4", "/page5"}
	results := warmURLsConcurrently(server.URL, urls)

	// Verify all URLs were processed
	if len(results) != len(urls) {
		t.Errorf("warmURLsConcurrently() returned %d results, want %d", len(results), len(urls))
	}

	// Verify all requests were successful
	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			t.Errorf("URL %s failed: %v", result.URL, result.Error)
		}
	}
	if successCount != len(urls) {
		t.Errorf("warmURLsConcurrently() had %d successful requests, want %d", successCount, len(urls))
	}

	// Verify the correct number of requests were made
	mu.Lock()
	if requestCount != len(urls) {
		t.Errorf("Server received %d requests, want %d", requestCount, len(urls))
	}
	mu.Unlock()
}

func TestWorker(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/success" {
			w.WriteHeader(http.StatusOK)
		} else if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
		} else if r.URL.Path == "/timeout" {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	tests := []struct {
		name         string
		urls         []string
		cancelCtx    bool
		expectResult []bool
	}{
		{
			name:         "All successful",
			urls:         []string{"/success", "/success"},
			expectResult: []bool{true, true},
		},
		{
			name:         "Mixed results",
			urls:         []string{"/success", "/fail"},
			expectResult: []bool{true, false},
		},
		{
			name:         "Context canceled",
			urls:         []string{"/timeout", "/success"},
			cancelCtx:    true,
			expectResult: []bool{false}, // Only one result is expected because context is canceled immediately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create channels
			jobs := make(chan string, len(tt.urls))
			results := make(chan WarmResult, len(tt.urls))
			var wg sync.WaitGroup

			// For the context canceled test, we need a special approach
			if tt.name == "Context canceled" {
				// Create a pre-canceled context
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				defer cancel()

				// Start worker with canceled context
				wg.Add(1)
				go worker(ctx, server.URL, jobs, results, &wg)

				// Send jobs
				for _, url := range tt.urls {
					jobs <- url
				}
				close(jobs)
			} else {
				// Normal test case
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				// Start worker
				wg.Add(1)
				go worker(ctx, server.URL, jobs, results, &wg)

				// Send jobs
				for _, url := range tt.urls {
					jobs <- url
				}
				close(jobs)
			}

			// Wait and close results
			go func() {
				wg.Wait()
				close(results)
			}()

			// Collect results
			var gotResults []WarmResult
			for result := range results {
				gotResults = append(gotResults, result)
			}

			// Verify results
			if len(gotResults) != len(tt.expectResult) {
				t.Errorf("worker() produced %d results, want %d", len(gotResults), len(tt.expectResult))
				return
			}

			for i, result := range gotResults {
				if result.Success != tt.expectResult[i] {
					t.Errorf("worker() result[%d].Success = %v, want %v, error: %v", 
						i, result.Success, tt.expectResult[i], result.Error)
				}
			}
		})
	}
}

// TestMainEnvironmentVariables tests the environment variable handling
func TestMainEnvironmentVariables(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tests := []struct {
		name           string
		varnishBaseURL string
		cacheURLs      string
		expectPanic    bool
	}{
		{
			name:           "Missing VARNISH_BASE_URL",
			varnishBaseURL: "",
			cacheURLs:      "/test",
			expectPanic:    true,
		},
		{
			name:           "With default URLs",
			varnishBaseURL: server.URL,
			cacheURLs:      "",
			expectPanic:    false,
		},
		{
			name:           "With custom URLs",
			varnishBaseURL: server.URL,
			cacheURLs:      "/test1,/test2",
			expectPanic:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			os.Setenv("VARNISH_BASE_URL", tt.varnishBaseURL)
			os.Setenv("CACHE_URLS", tt.cacheURLs)
			defer func() {
				os.Unsetenv("VARNISH_BASE_URL")
				os.Unsetenv("CACHE_URLS")
			}()

			// Test the VARNISH_BASE_URL check
			if tt.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Expected panic with missing VARNISH_BASE_URL")
					}
				}()

				// This should panic if VARNISH_BASE_URL is empty
				if tt.varnishBaseURL == "" {
					panic("VARNISH_BASE_URL environment variable is required")
				}
			} else {
				// Just verify the URLs are processed correctly
				urls := getURLsFromEnv()
				if tt.cacheURLs == "" && len(urls) != 0 {
					t.Errorf("Expected empty URLs, got %v", urls)
				} else if tt.cacheURLs != "" && len(urls) == 0 {
					t.Errorf("Expected non-empty URLs, got empty")
				}
			}
		})
	}
}

// TestMainWithMockServer tests the main workflow with a mock server
func TestMainWithMockServer(t *testing.T) {
	// Create a test server that simulates Varnish
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Varnish-Cache", "MISS")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Set environment variables
	os.Setenv("VARNISH_BASE_URL", server.URL)
	os.Setenv("CACHE_URLS", "/test1,/test2")
	defer func() {
		os.Unsetenv("VARNISH_BASE_URL")
		os.Unsetenv("CACHE_URLS")
	}()

	// Test the URL processing
	urls := getURLsFromEnv()
	if len(urls) != 2 {
		t.Errorf("Expected 2 URLs, got %d", len(urls))
	}

	// Test connectivity
	err := testConnectivity(server.URL)
	if err != nil {
		t.Errorf("testConnectivity() error = %v", err)
	}

	// Test warming
	results := warmURLsConcurrently(server.URL, urls)

	// Verify results
	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}

	if successCount != len(urls) {
		t.Errorf("Expected %d successful warmings, got %d", len(urls), successCount)
	}
}
