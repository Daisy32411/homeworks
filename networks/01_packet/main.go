package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	data, err := readInput()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	decodedData, err := decodeHex(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	etherType, err := parseEthernet(decodedData)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if etherType == 0x0800 {
		err = parseIPv4(decodedData[14:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
            os.Exit(1)
		}
	}
}

func printMac(frame []byte, dir string, l, r int) {
	fmt.Printf("eth.%s ", dir)

	for i := l; i <= r; i++ {
		fmt.Printf("%02x", frame[i])
		
		if i != r {
			fmt.Printf(":")
		}
	}

	fmt.Println()
}

func parseEthernet(frame []byte) (uint16, error) {
	if len(frame) < 14 {
		return 0, fmt.Errorf("Ethernet frame too short")
	}

	printMac(frame, "dst", 0, 5)
	printMac(frame, "src", 6, 11)

	etherType := binary.BigEndian.Uint16(frame[12:14])

	fmt.Printf("eth.ethertype 0x%04x\n", etherType)

	return etherType, nil
}

func parseIPv4(packet []byte) error {
	if len(packet) < 20 {
		return fmt.Errorf("IPv4 packet too short")
	}

	version := packet[0] >> 4
	ihl := packet[0] & 0x0F

	byteIHL := int(ihl) * 4
	if byteIHL < 20 || byteIHL > len(packet) {
		return fmt.Errorf("invalid IPv4 header length: %d", byteIHL)
	}

	totalLength := binary.BigEndian.Uint16(packet[2:4])
	identification := binary.BigEndian.Uint16(packet[4:6])
	flagsAndOffset := binary.BigEndian.Uint16(packet[6:8])

	df := flagsAndOffset & 0x4000 != 0
	mf := flagsAndOffset & 0x2000 != 0

	fragmentOffset := (flagsAndOffset & 0x1FFF) * 8
	

	flagsText := ""
	
	if !df && !mf {
		flagsText = "none"
	} else if df && !mf {
		flagsText = "DF"
	} else if !df && mf {
		flagsText = "MF"
	} else {
		flagsText = "DF,MF"
	}

	ttl := int(packet[8])
	protocol := int(packet[9])

	checkSumValid := validIPv4Checksum(packet[:byteIHL])

	sourceIp := binary.BigEndian.Uint32(packet[12:16])
	destinationIp := binary.BigEndian.Uint32(packet[16:20])

	fmt.Printf("ip.version %d\n", version)
	fmt.Printf("ip.ihl_bytes %d\n", byteIHL)
	fmt.Printf("ip.total_length %d\n", totalLength)
	fmt.Printf("ip.id 0x%04x\n", identification)
	fmt.Printf("ip.flags %s\n", flagsText)
	fmt.Printf("ip.frag_offset %d\n", fragmentOffset)
	fmt.Printf("ip.ttl %d\n", ttl)
	fmt.Printf("ip.protocol %d\n", protocol)
	fmt.Printf("ip.checksum_valid %t\n", checkSumValid)
	fmt.Printf("ip.src %d.%d.%d.%d\n", (sourceIp >> 24) & 0xFF, 
									 (sourceIp >> 16) & 0xFF, 
									 (sourceIp >> 8) & 0xFF, 
									 (sourceIp & 0xFF))
	fmt.Printf("ip.dst %d.%d.%d.%d\n", (destinationIp >> 24) & 0xFF, 
									 (destinationIp >> 16) & 0xFF, 
									 (destinationIp >> 8) & 0xFF, 
									 (destinationIp & 0xFF))
	
	if int(totalLength) < byteIHL || int(totalLength) > len(packet) {
		return fmt.Errorf("invalid IPv4 total length: %d", totalLength)
	}

	if protocol == 6 && fragmentOffset == 0 {
		err := parseTCP(packet[byteIHL:int(totalLength)])
		if err != nil {
			return err
		}
	}

	if protocol == 17 && fragmentOffset == 0 {
		err := parseUDP(packet[byteIHL:int(totalLength)])
		if err != nil {
			return err
		}
	}

	if (protocol != 6 && protocol != 17) || fragmentOffset != 0 {
    	payloadLength := int(totalLength) - byteIHL

    	fmt.Printf("payload.length %d\n", payloadLength)
	}

	return nil
}

func parseUDP(segment []byte) error {
	if len(segment) < 8 {
		return fmt.Errorf("UDP segment too short: %d bytes", len(segment))
	}

	srcPort := binary.BigEndian.Uint16(segment[0:2])
	dstPort := binary.BigEndian.Uint16(segment[2:4])
	udpLength := int(binary.BigEndian.Uint16(segment[4:6]))

	if udpLength < 8 || udpLength > len(segment) {
		return fmt.Errorf("invalid UDP length: %d", udpLength)
	}

	payloadLength := udpLength - 8

	fmt.Printf("udp.src_port %d\n", srcPort)
	fmt.Printf("udp.dst_port %d\n", dstPort)
	fmt.Printf("udp.length %d\n", udpLength)
	fmt.Printf("payload.length %d\n", payloadLength)

	return nil
}

func parseTCP(segment []byte) error {
	if len(segment) < 20 {
		return fmt.Errorf("TCP segment too short: %d bytes", len(segment))
	}

	srcPort := binary.BigEndian.Uint16(segment[0:2])
	dstPort := binary.BigEndian.Uint16(segment[2:4])

	seq := binary.BigEndian.Uint32(segment[4:8])
	ack := binary.BigEndian.Uint32(segment[8:12])

	dataOffset := segment[12] >> 4
	headerLength := int(dataOffset) * 4

	if headerLength < 20 || headerLength > len(segment) {
		return fmt.Errorf("invalid TCP header length: %d", headerLength)
	}

	flags := segment[13]

	var flagsList []string
	if flags & 0x01 != 0 {
		flagsList = append(flagsList, "FIN")
	}
	if flags & 0x02 != 0 {
		flagsList = append(flagsList, "SYN")
	}
	if flags & 0x04 != 0 {
		flagsList = append(flagsList, "RST")
	}
	if flags & 0x08 != 0 {
		flagsList = append(flagsList, "PSH")
	}
	if flags & 0x10 != 0 {
		flagsList = append(flagsList, "ACK")
	}
	if flags & 0x20 != 0 {
		flagsList = append(flagsList, "URG")
	}
	if len(flagsList) == 0 {
		flagsList = append(flagsList, "none")
	}
	result := strings.Join(flagsList, ",")

	window := binary.BigEndian.Uint16(segment[14:16])

	payloadLength := len(segment) - headerLength

	fmt.Printf("tcp.src_port %d\n", srcPort)
	fmt.Printf("tcp.dst_port %d\n", dstPort)
	fmt.Printf("tcp.seq %d\n", seq)
	fmt.Printf("tcp.ack %d\n", ack)
	fmt.Printf("tcp.data_offset_bytes %d\n", headerLength)
	fmt.Printf("tcp.flags %s\n", result)
	fmt.Printf("tcp.window %d\n", window)
	fmt.Printf("payload.length %d\n", payloadLength)

	return nil
}

func validIPv4Checksum(header []byte) bool {
	var sum uint32

	for i := 0; i < len(header); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(header[i:i + 2]))
	}

	for (sum > 0xFFFF) {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}

	return sum == 0xFFFF
}

func decodeHex(data []byte) ([]byte, error) {
	var str string = string(data)

	str = strings.Join(strings.Fields(str), "")
	str = strings.ReplaceAll(str, ":", "")

	decodedData, err := hex.DecodeString(str)
	if err != nil {
		return nil, err
	}

	return decodedData, nil
}

func readInput() ([]byte, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}

	return data, nil
}