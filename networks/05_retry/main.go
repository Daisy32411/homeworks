package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	URL            string
	Method         string
	MaxAttempts    int
	IdempotencyKey string
}

func main() {
	config, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(runRetries(config))
}

func parseArgs(args []string) (Config, error) {
	config := Config{
		Method:      "GET",
		MaxAttempts: 5,
	}

	if len(args) == 0 {
		return Config{}, fmt.Errorf("missing URL")
	}

	config.URL = args[0]

	parsedURL, err := url.Parse(config.URL)
	if err != nil || parsedURL.Scheme != "http" || parsedURL.Host == "" {
		return Config{}, fmt.Errorf("invalid URL: %s", config.URL)
	}

	for i := 1; i < len(args); i++ {
		if i+1 >= len(args) {
			return Config{}, fmt.Errorf("missing value for %s", args[i])
		}

		switch args[i] {
		case "--method":
			config.Method = args[i+1]

		case "--max-attempts":
			attempts, err := strconv.Atoi(args[i+1])
			if err != nil || attempts <= 0 {
				return Config{}, fmt.Errorf("invalid max-attempts")
			}
			config.MaxAttempts = attempts

		case "--idempotency-key":
			config.IdempotencyKey = args[i+1]

		default:
			return Config{}, fmt.Errorf("unknown flag: %s", args[i])
		}

		i++
	}

	return config, nil
}

func runRetries(config Config) int {
	transport := &http.Transport{
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		status, retryAfter, err := doRequest(client, config)

		if err != nil {
			fmt.Printf("attempt %d error %s\n", attempt, err)
		} else {
			fmt.Printf("attempt %d status %d\n", attempt, status)
		}

		if err == nil && status >= 200 && status <= 399 {
			fmt.Printf("result success attempts %d\n", attempt)
			return 0
		}

		if attempt == config.MaxAttempts ||
			!canRetryMethod(config.Method, config.IdempotencyKey) ||
			(err == nil && !canRetryStatus(status)) {

			fmt.Printf("result failure attempts %d\n", attempt)
			return 1
		}

		delay := retryDelay(attempt+1, retryAfter)

		fmt.Printf("sleep_ms %d\n", delay.Milliseconds())
		time.Sleep(delay)
	}

	return 1
}

func doRequest(client *http.Client, config Config) (int, string, error) {
	req, err := http.NewRequest(config.Method, config.URL, nil)
	if err != nil {
		return 0, "", err
	}

	if config.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", config.IdempotencyKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, "", err
	}

	return resp.StatusCode, resp.Header.Get("Retry-After"), nil
}

func canRetryMethod(method string, key string) bool {
	switch method {
	case "GET", "HEAD", "PUT", "DELETE", "OPTIONS", "TRACE":
		return true
	case "POST":
		return key != ""
	default:
		return false
	}
}

func canRetryStatus(status int) bool {
	switch status {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func retryDelay(nextAttempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		seconds, err := strconv.ParseInt(strings.TrimSpace(retryAfter), 10, 64)
		if err == nil && seconds >= 0 &&
			seconds <= (1<<63-1)/int64(time.Second) {
			return time.Duration(seconds) * time.Second
		}
	}

	maxDelay := 200

	for i := 1; i < nextAttempt; i++ {
		maxDelay *= 2

		if maxDelay >= 2000 {
			maxDelay = 2000
			break
		}
	}

	delay := rand.Intn(maxDelay + 1)

	return time.Duration(delay) * time.Millisecond
}
