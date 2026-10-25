package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Result struct {
	Status  string
	Headers http.Header
	Body    []byte
	Err     error
}

func main() {
	var timeout int = 15
	flag.IntVar(&timeout, "t", 15, "Timeout in seconds")
	flag.IntVar(&timeout, "timeout", 15, "Timeout in seconds")

	flag.CommandLine.SetOutput(os.Stdout)
	flag.Parse()
	urls := flag.Args()

	var size int = len(urls)

	if size == 0 {
		fmt.Fprintln(os.Stderr, "Error: no URLs provided")
		os.Exit(1)
	}

	done := make(chan Result, size)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(timeout)*time.Second,
	)
	defer cancel()

	for i := 0; i < size; i++ {
		go fetch(urls[i], done, ctx)
	}

	for i := 0; i < size; i++ {
		select {
		case result := <-done:
			if result.Err == nil {
				result.print()
				return
			}

		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "Error: timeout")
			os.Exit(228)
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		fmt.Fprintln(os.Stderr, "Error: timeout")
		os.Exit(228)
	}

	fmt.Fprintln(os.Stderr, "Error: all requests failed")
	os.Exit(1)
}

func fetch(url string, done chan Result, ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		done <- Result{Err: err}
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		done <- Result{Err: err}
		return
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		done <- Result{Err: err}
		return
	}

	var result Result
	result.Status = resp.Proto + " " + resp.Status
	result.Headers = resp.Header
	result.Body = data

	done <- result
}

func (r *Result) print() {
	fmt.Println(r.Status)

	for key, values := range r.Headers {
		for _, value := range values {
			fmt.Printf("%s: %s\n", key, value)
		}
	}

	fmt.Println()
	fmt.Print(string(r.Body))
}
