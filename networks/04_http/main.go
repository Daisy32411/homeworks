package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type HTTPHeader struct {
	Name  string
	Value string
}

type HTTPMessage struct {
	Type    string
	Method  string
	Target  string
	Version string
	Status  string
	Reason  string
	Headers []HTTPHeader
	Body    []byte
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	msg, err := parseHTTPMessage(data)
	if err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}

	printHTTPMessage(msg)
}

func parseStartLine(line string) (HTTPMessage, error) {
	var msg HTTPMessage

	if strings.HasPrefix(line, "HTTP/") {
		parts := strings.SplitN(line, " ", 3)

		if len(parts) < 2 {
			return msg, fmt.Errorf("bad_start_line")
		}
		if parts[0] != "HTTP/1.1" {
			return msg, fmt.Errorf("bad_start_line")
		}
		if len(parts[1]) != 3 || parts[1][0] < '0' || parts[1][0] > '9' ||
			parts[1][1] < '0' || parts[1][1] > '9' ||
			parts[1][2] < '0' || parts[1][2] > '9' {
			return msg, fmt.Errorf("bad_start_line")
		}

		msg.Type = "response"
		msg.Version = parts[0]
		msg.Status = parts[1]
		if len(parts) > 2 {
			msg.Reason = parts[2]
		}
	} else {
		parts := strings.Split(line, " ")

		if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
			return msg, fmt.Errorf("bad_start_line")
		}

		if parts[2] != "HTTP/1.1" {
			return msg, fmt.Errorf("bad_start_line")
		}

		for i := 0; i < len(parts[0]); i++ {
			if !validCh(parts[0][i]) {
				return msg, fmt.Errorf("bad_start_line")
			}
		}

		for i := 0; i < len(parts[1]); i++ {
			if parts[1][i] < 0x21 || parts[1][i] > 0x7E {
				return msg, fmt.Errorf("bad_start_line")
			}
		}

		msg.Type = "request"
		msg.Method = parts[0]
		msg.Target = parts[1]
		msg.Version = parts[2]
	}

	return msg, nil
}

func parseHeaders(data []byte) ([]HTTPHeader, int, error) {
	headers := make([]HTTPHeader, 0)
	offset := 0

	for {
		idx := bytes.Index(data[offset:], []byte("\r\n"))
		if idx == -1 {
			return nil, 0, fmt.Errorf("bad_header")
		}

		str := data[offset : offset+idx]

		offset += idx + 2

		if len(str) == 0 {
			return headers, offset, nil
		}

		idxColon := bytes.IndexByte(str, ':')
		if idxColon == -1 {
			return nil, 0, fmt.Errorf("bad_header")
		}

		name := string(str[:idxColon])
		value := string(str[idxColon+1:])

		if len(name) == 0 {
			return nil, 0, fmt.Errorf("bad_header")
		}

		name = strings.ToLower(name)

		for i := 0; i < len(name); i++ {
			if !validCh(name[i]) {
				return nil, 0, fmt.Errorf("bad_header")
			}
		}

		value = strings.Trim(value, " \t")

		for i := 0; i < len(value); i++ {
			ch := value[i]

			if (ch < 0x20 && ch != '\t') || ch == 0x7F {
				return nil, 0, fmt.Errorf("bad_header")
			}
		}

		if name == "content-length" {
			for i := 0; i < len(value); i++ {
				if value[i] < '0' || value[i] > '9' {
					return nil, 0, fmt.Errorf("bad_header")
				}
			}

			_, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return nil, 0, fmt.Errorf("bad_header")
			}
		}

		headers = append(headers, HTTPHeader{name, value})
	}
}

func parseHTTPMessage(data []byte) (HTTPMessage, error) {
	idx := bytes.Index(data, []byte("\r\n"))

	if idx == -1 {
		return HTTPMessage{}, fmt.Errorf("bad_start_line")
	}

	httpMessage, err := parseStartLine(string(data[:idx]))
	if err != nil {
		return HTTPMessage{}, err
	}

	rest := data[idx+2:]

	headers, offset, err := parseHeaders(rest)
	if err != nil {
		return HTTPMessage{}, err
	}

	httpMessage.Headers = headers

	body, err := parseBody(rest[offset:], headers)
	if err != nil {
		return HTTPMessage{}, err
	}

	httpMessage.Body = body

	return httpMessage, nil
}

func printHTTPMessage(msg HTTPMessage) {
	fmt.Printf("type %s\n", msg.Type)
	if msg.Type == "request" {
		fmt.Printf("method %s\n", msg.Method)
		fmt.Printf("target %s\n", msg.Target)
	}

	fmt.Printf("version %s\n", msg.Version)

	if msg.Type == "response" {
		fmt.Printf("status %s\n", msg.Status)
		fmt.Printf("reason %s\n", msg.Reason)
	}

	for i := 0; i < len(msg.Headers); i++ {
		fmt.Printf("header %s %s\n", msg.Headers[i].Name, msg.Headers[i].Value)
	}

	fmt.Printf("body.length %d\n", len(msg.Body))

	printable := false
	if len(msg.Body) > 0 {
		printable = true
	}

	for i := 0; i < len(msg.Body); i++ {
		if msg.Body[i] < 0x20 || msg.Body[i] > 0x7E {
			printable = false
		}
	}

	if printable {
		fmt.Printf("body.text %s\n", string(msg.Body))
	}
}

func parseBody(data []byte, headers []HTTPHeader) ([]byte, error) {
	for _, header := range headers {
		if header.Name == "transfer-encoding" &&
			strings.EqualFold(header.Value, "chunked") {
			return decodeChunked(data)
		}
	}

	for i := 0; i < len(headers); i++ {
		if headers[i].Name == "content-length" {
			length, err := strconv.ParseUint(headers[i].Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("bad_header")
			}

			if length > uint64(len(data)) {
				return nil, fmt.Errorf("incomplete_body")
			}

			return data[:int(length)], nil
		}
	}

	return nil, nil
}

func decodeChunked(data []byte) ([]byte, error) {
	body := make([]byte, 0)
	offset := 0

	for {
		idx := bytes.Index(data[offset:], []byte("\r\n"))
		if idx == -1 {
			return nil, fmt.Errorf("bad_chunk")
		}

		line := string(data[offset : offset+idx])

		text := strings.SplitN(line, ";", 2)[0]

		offset += idx + 2

		size, err := strconv.ParseUint(text, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("bad_chunk")
		}

		if size == 0 {
			for {
				idx := bytes.Index(data[offset:], []byte("\r\n"))
				if idx == -1 {
					return nil, fmt.Errorf("bad_chunk")
				}

				offset += idx + 2

				if idx == 0 {
					return body, nil
				}
			}
		}

		if size > uint64(len(data)-offset) {
			return nil, fmt.Errorf("bad_chunk")
		}

		end := offset + int(size)

		if len(data)-end < 2 || data[end] != '\r' || data[end+1] != '\n' {
			return nil, fmt.Errorf("bad_chunk")
		}

		body = append(body, data[offset:end]...)

		offset = end + 2
	}
}

func validCh(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		strings.ContainsAny(string(ch), "!#$%&'*+-.^_`|~")
}
