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
	Reader  io.ReadCloser
	Err     error
	ID      int
}

func main() {
	os.Exit(run())
}

func run() int {
	var timeout int
	flag.IntVar(&timeout, "t", 15, "Timeout in seconds")
	flag.IntVar(&timeout, "timeout", 15, "Timeout in seconds")
	flag.CommandLine.SetOutput(os.Stdout)
	flag.Parse()

	const maxTimeout = int64(1<<63-1) / int64(time.Second)
	if timeout <= 0 || int64(timeout) > maxTimeout {
		fmt.Fprintln(os.Stderr, "Error: invalid timeout")
		return 1
	}

	urls := flag.Args()
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no URLs provided")
		return 1
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(timeout)*time.Second,
	)
	defer cancel()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	done := make(chan Result)
	stop := make(chan struct{})
	cancels := make([]context.CancelFunc, len(urls))

	stopClosed := false
	defer func() {
		if !stopClosed {
			close(stop)
		}

		for _, cancelRequest := range cancels {
			if cancelRequest != nil {
				cancelRequest()
			}
		}
	}()

	for i, url := range urls {
		requestCtx, requestCancel := context.WithCancel(ctx)
		cancels[i] = requestCancel

		go fetch(i, url, client, done, stop, requestCtx)
	}

	for range urls {
		select {
		case result := <-done:
			if result.Err != nil {
				continue
			}

			if ctx.Err() == context.DeadlineExceeded {
				result.Reader.Close()
				fmt.Fprintln(os.Stderr, "Error: timeout")
				return 228
			}

			close(stop)
			stopClosed = true

			for i, cancelRequest := range cancels {
				if i != result.ID {
					cancelRequest()
				}
			}

			err := result.print()

			cancels[result.ID]()

			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					fmt.Fprintln(os.Stderr, "Error: timeout")
					return 228
				}

				fmt.Fprintln(os.Stderr, "Error reading body:", err)
				return 1
			}

			return 0

		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "Error: timeout")
			return 228
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		fmt.Fprintln(os.Stderr, "Error: timeout")
		return 228
	}

	fmt.Fprintln(os.Stderr, "Error: all requests failed")
	return 1
}

func fetch(
	id int,
	url string,
	client *http.Client,
	done chan<- Result,
	stop <-chan struct{},
	ctx context.Context,
) {

	send := func(result Result) {
		select {
		case done <- result:

		case <-stop:
			if result.Reader != nil {
				result.Reader.Close()
			}
		}
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		url,
		nil,
	)
	if err != nil {
		send(Result{ID: id, Err: err})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}

		send(Result{ID: id, Err: err})
		return
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()

		send(Result{
			ID:  id,
			Err: fmt.Errorf("HTTP error: %s", resp.Status),
		})
		return
	}

	send(Result{
		ID:      id,
		Status:  resp.Proto + " " + resp.Status,
		Headers: resp.Header,
		Reader:  resp.Body,
	})
}

func (r *Result) print() error {
	defer r.Reader.Close()

	if _, err := fmt.Fprintln(os.Stdout, r.Status); err != nil {
		return err
	}

	for key, values := range r.Headers {
		for _, value := range values {
			if _, err := fmt.Fprintf(
				os.Stdout,
				"%s: %s\n",
				key,
				value,
			); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(os.Stdout); err != nil {
		return err
	}

	_, err := io.Copy(os.Stdout, r.Reader)
	return err
}
