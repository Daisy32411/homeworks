package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type DNSAnswer struct {
	Type  string
	Value string
	TTL   uint32
}

type CachedResponse struct {
	Answers   []DNSAnswer
	ExpiresAt time.Time
}

func main() {
	server, port, err := parseServerArgs(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	err = readQueries(os.Stdin, server, port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseServerArgs(args []string) (string, int, error) {
	if len(args) != 3 {
		return "", 0, fmt.Errorf("usage: run.sh <server> <port>")
	}

	server := args[1]
	ip, err := netip.ParseAddr(server)
	if err != nil {
		return "", 0, fmt.Errorf("invalid DNS server IP: %q", server)
	}

	server = ip.String()

	port, err := strconv.Atoi(args[2])
	if err != nil {
		return "", 0, err
	}

	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port: %d", port)
	}

	return server, port, nil
}

func readQueries(in io.Reader, server string, port int) error {
	scanner := bufio.NewScanner(in)

	cache := make(map[string]CachedResponse)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		err := handleQuery(line, server, port, cache)
		if err != nil {
			return err
		}
	}

	return scanner.Err()
}

func handleQuery(line string, server string, port int, cache map[string]CachedResponse) error {
	parts := strings.Fields(line)
	if len(parts) != 2 {
		return fmt.Errorf("invalid query: %q", line)
	}

	name := parts[0]
	recordType := parts[1]

	fmt.Printf("query %s %s\n", name, recordType)

	code, err := dnsTypeCode(recordType)
	if err != nil {
		return err
	}

	key := strings.ToLower(strings.TrimSuffix(name, ".")) + "|" + recordType

	if cached, ok := cache[key]; ok {
		if time.Now().Before(cached.ExpiresAt) {
			printDNSResult("NOERROR", cached.Answers)
			return nil
		}

		delete(cache, key)
	}

	query, err := buildDNSQuery(name, code, 0x1234)
	if err != nil {
		return err
	}

	response, err := sendDNSQuery(server, port, query, 0x1234)
	if err != nil {
		if os.IsTimeout(err) {
			fmt.Println("status TIMEOUT")
			fmt.Println("end")
		}

		return err
	}

	status, q, a, err := parseDNSHeader(response)
	if err != nil {
		return err
	}

	answers, minTTL, err := parseDNSAnswers(response, q, a)
	if err != nil {
		return err
	}

	if status == "NOERROR" && a > 0 && minTTL > 0 {
		cache[key] = CachedResponse{
			Answers:   answers,
			ExpiresAt: time.Now().Add(time.Duration(minTTL) * time.Second),
		}
	}

	printDNSResult(status, answers)

	return nil
}

func sendDNSQuery(server string, port int, packet []byte, id uint16) ([]byte, error) {
	strPort := strconv.Itoa(port)

	host := net.JoinHostPort(server, strPort)

	connection, err := net.Dial("udp", host)
	if err != nil {
		return nil, err
	}

	defer connection.Close()

	currTime := time.Now().Add(5 * time.Second)

	err = connection.SetDeadline(currTime)
	if err != nil {
		return nil, err
	}

	n, err := connection.Write(packet)
	if err != nil {
		return nil, err
	}

	if n != len(packet) {
		return nil, fmt.Errorf("incomplete UDP write: sent %d of %d bytes", n, len(packet))
	}

	response := make([]byte, 65535)

	for {
		n, err = connection.Read(response)
		if err != nil {
			return nil, err
		}

		if n < 12 {
			continue
		}

		respId := binary.BigEndian.Uint16(response[0:2])
		if respId != id {
			continue
		}

		flags := binary.BigEndian.Uint16(response[2:4])
		if flags&0x8000 == 0 {
			continue
		}

		return response[:n], nil
	}
}

func dnsTypeCode(recordType string) (uint16, error) {
	switch recordType {
	case "A":
		return 1, nil
	case "NS":
		return 2, nil
	case "CNAME":
		return 5, nil
	case "MX":
		return 15, nil
	case "TXT":
		return 16, nil
	case "AAAA":
		return 28, nil
	}

	return 0, fmt.Errorf("invalid type code: %q", recordType)
}

func encodeDomain(name string) ([]byte, error) {
	if name == "." {
		return []byte{0}, nil
	}

	name = strings.TrimSuffix(name, ".")

	parts := strings.Split(name, ".")

	result := make([]byte, 0)

	for _, label := range parts {
		if len(label) == 0 || len(label) > 63 {
			return nil, fmt.Errorf("invalid domain label %q: length must be 1..63 bytes", label)
		}

		result = append(result, byte(len(label)))
		result = append(result, []byte(label)...)
	}

	result = append(result, 0)

	if len(result) > 255 {
		return nil, fmt.Errorf("encoded domain name too long: %d bytes", len(result))
	}

	return result, nil
}

func buildDNSQuery(name string, qtype uint16, id uint16) ([]byte, error) {
	header := make([]byte, 12)

	binary.BigEndian.PutUint16(header[0:2], id)
	binary.BigEndian.PutUint16(header[2:4], 0x0100)
	binary.BigEndian.PutUint16(header[4:6], 1)

	dom, err := encodeDomain(name)
	if err != nil {
		return nil, err
	}

	header = append(header, dom...)

	newByte := make([]byte, 4)

	binary.BigEndian.PutUint16(newByte[0:2], qtype)
	binary.BigEndian.PutUint16(newByte[2:4], 1)

	header = append(header, newByte...)

	return header, nil
}

func dnsStatus(rcode uint16) string {
	switch rcode {
	case 0:
		return "NOERROR"
	case 1:
		return "FORMERR"
	case 2:
		return "SERVFAIL"
	case 3:
		return "NXDOMAIN"
	case 5:
		return "REFUSED"
	default:
		return fmt.Sprintf("RCODE%d", rcode)
	}
}

func parseDNSHeader(response []byte) (string, int, int, error) {
	if len(response) < 12 {
		return "", 0, 0, fmt.Errorf("DNS response too short: %d bytes", len(response))
	}

	flags := binary.BigEndian.Uint16(response[2:4])
	rCode := flags & 0x000F

	status := dnsStatus(rCode)

	qdCount := int(binary.BigEndian.Uint16(response[4:6]))
	anCount := int(binary.BigEndian.Uint16(response[6:8]))

	return status, qdCount, anCount, nil
}

func readDomain(data []byte, offset int) (string, int, error) {
	parts := make([]string, 0)

	nextOffset := offset
	jumped := false

	for steps := 0; steps < len(data); steps++ {
		if offset < 0 || offset >= len(data) {
			return "", 0, fmt.Errorf("DNS name offset out of bounds: %d", offset)
		}

		b := data[offset]

		if b == 0 {
			if !jumped {
				nextOffset = offset + 1
			}

			if len(parts) == 0 {
				return ".", nextOffset, nil
			}

			return strings.Join(parts, ".") + ".", nextOffset, nil
		}

		if b&0xC0 == 0xC0 {
			if offset+1 >= len(data) {
				return "", 0, fmt.Errorf("incomplete DNS compression pointer")
			}

			pointer := int(b&0x3F)<<8 | int(data[offset+1])

			if !jumped {
				nextOffset = offset + 2
			}

			jumped = true
			offset = pointer

			continue
		}

		if b&0xC0 != 0 {
			return "", 0, fmt.Errorf("invalid DNS label length byte: %d", b)
		}

		labelLength := int(b)

		start := offset + 1
		end := start + labelLength

		if end > len(data) {
			return "", 0, fmt.Errorf("DNS label exceeds packet length")
		}

		label := string(data[start:end])
		parts = append(parts, label)

		offset = end

		if !jumped {
			nextOffset = offset
		}
	}

	return "", 0, fmt.Errorf("DNS name compression pointer loop detected")
}

func parseDNSAnswers(data []byte, questions int, answerCount int) ([]DNSAnswer, uint32, error) {
	if len(data) < 12 {
		return nil, 0, fmt.Errorf("DNS response too short")
	}

	offset := 12
	answers := make([]DNSAnswer, 0, answerCount)

	var minTTL uint32

	for i := 0; i < questions; i++ {
		_, nextOffset, err := readDomain(data, offset)
		if err != nil {
			return nil, 0, err
		}

		offset = nextOffset

		if offset+4 > len(data) {
			return nil, 0, fmt.Errorf("incomplete DNS question")
		}

		offset += 4
	}

	for i := 0; i < answerCount; i++ {

		_, nextOffset, err := readDomain(data, offset)
		if err != nil {
			return nil, 0, err
		}

		offset = nextOffset

		if offset+10 > len(data) {
			return nil, 0, fmt.Errorf("incomplete DNS answer header")
		}

		recordType := binary.BigEndian.Uint16(data[offset : offset+2])
		recordClass := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		ttl := binary.BigEndian.Uint32(data[offset+4 : offset+8])
		rdlength := int(binary.BigEndian.Uint16(data[offset+8 : offset+10]))

		dataStart := offset + 10
		dataEnd := dataStart + rdlength

		if dataEnd > len(data) {
			return nil, 0, fmt.Errorf("DNS answer data exceeds packet length")
		}

		if i == 0 || ttl < minTTL {
			minTTL = ttl
		}

		if recordClass == 1 {
			typeName, value, supported, err := parseDNSValue(
				data,
				recordType,
				dataStart,
				dataEnd,
			)
			if err != nil {
				return nil, 0, err
			}

			if supported {
				answers = append(answers, DNSAnswer{
					Type:  typeName,
					Value: value,
					TTL:   ttl,
				})
			}
		}

		offset = dataEnd
	}

	return answers, minTTL, nil
}

func parseDNSValue(data []byte, recordType uint16, start int, end int) (string, string, bool, error) {
	switch recordType {

	case 1:
		if end-start != 4 {
			return "", "", false, fmt.Errorf("invalid A record length")
		}

		ip := fmt.Sprintf(
			"%d.%d.%d.%d",
			data[start],
			data[start+1],
			data[start+2],
			data[start+3],
		)

		return "A", ip, true, nil

	case 28:
		if end-start != 16 {
			return "", "", false, fmt.Errorf("invalid AAAA record length")
		}

		var addr [16]byte
		copy(addr[:], data[start:end])

		return "AAAA", netip.AddrFrom16(addr).String(), true, nil

	case 5, 2:
		name, nextOffset, err := readDomain(data, start)
		if err != nil {
			return "", "", false, err
		}

		if nextOffset != end {
			return "", "", false, fmt.Errorf("invalid DNS name record length")
		}

		if recordType == 5 {
			return "CNAME", name, true, nil
		}

		return "NS", name, true, nil

	case 15:
		if end-start < 3 {
			return "", "", false, fmt.Errorf("invalid MX record length")
		}

		priority := binary.BigEndian.Uint16(data[start : start+2])

		name, nextOffset, err := readDomain(data, start+2)
		if err != nil {
			return "", "", false, err
		}

		if nextOffset != end {
			return "", "", false, fmt.Errorf("invalid MX domain length")
		}

		value := fmt.Sprintf("%d %s", priority, name)

		return "MX", value, true, nil

	case 16:
		var text strings.Builder

		offset := start

		for offset < end {
			length := int(data[offset])
			offset++

			if offset+length > end {
				return "", "", false, fmt.Errorf("invalid TXT record length")
			}

			text.Write(data[offset : offset+length])

			offset += length
		}

		return "TXT", text.String(), true, nil

	default:
		return "", "", false, nil
	}
}

func printDNSResult(status string, answers []DNSAnswer) {
	fmt.Printf("status %s\n", status)

	for _, answer := range answers {
		fmt.Printf("answer %s %s %d\n",
			answer.Type,
			answer.Value,
			answer.TTL,
		)
	}

	fmt.Println("end")
}
