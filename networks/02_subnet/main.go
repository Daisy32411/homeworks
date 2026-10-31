package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: subnet|route ...")
		os.Exit(1)
	}

	var err error

	if os.Args[1] == "subnet" && len(os.Args) == 3 {
		err = subnet(os.Args[2])
	} else if os.Args[1] == "route" && len(os.Args) == 4 {
		err = route(os.Args[2], os.Args[3])
	} else {
		fmt.Fprintln(os.Stderr, "invalid mode or number of arguments")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func subnet(ip string) error {
	parts := strings.Split(ip, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR format: %s", ip)
	}

	prefix, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}

	if prefix < 0 || prefix > 32 {
		return fmt.Errorf("invalid prefix: %d", prefix)
	}

	ipUint, err := parseIPv4Uint(parts[0])
	if err != nil {
		return err
	}

	mask := ^uint32(0) << (32 - prefix)

	network := ipUint & mask

	broadcast := network | ^mask

	first := network + 1
	last := broadcast - 1
	host := uint64(1)<<(32-prefix) - 2

	if prefix == 32 {
		first = network
		last = network
		host = 1
	} else if prefix == 31 {
		first = network
		last = network + 1
		host = 2
	}

	fmt.Printf("network %s\n", formatIPv4(network))
	if prefix == 31 || prefix == 32 {
		fmt.Printf("broadcast none\n")
	} else {
		fmt.Printf("broadcast %s\n", formatIPv4(broadcast))
	}
	fmt.Printf("netmask %s\n", formatIPv4(mask))
	fmt.Printf("prefix %d\n", prefix)
	fmt.Printf("first %s\n", formatIPv4(first))
	fmt.Printf("last %s\n", formatIPv4(last))
	fmt.Printf("hosts %d\n", host)

	return nil
}

func parseIPv4Uint(ip string) (uint32, error) {
	ipParts := strings.Split(ip, ".")
	if len(ipParts) != 4 {
		return 0, fmt.Errorf("invalid IPv4 address: %s", ip)
	}

	var ipUint uint32

	for i := 0; i < 4; i++ {
		num, err := strconv.Atoi(ipParts[i])
		if err != nil {
			return 0, err
		}
		if num < 0 || num > 255 {
			return 0, fmt.Errorf("IPv4 octet out of range: %d", num)
		}

		ipUint |= uint32(num) << (8 * (3 - i))
	}

	return ipUint, nil
}

func formatIPv4(ip uint32) string {
	ip1 := ip >> 24 & 0xFF
	ip2 := ip >> 16 & 0xFF
	ip3 := ip >> 8 & 0xFF
	ip4 := ip & 0xFF

	return fmt.Sprintf("%d.%d.%d.%d", ip1, ip2, ip3, ip4)
}

func route(path string, ip string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	parts := strings.Split(string(data), "\n")

	ipUint, err := parseIPv4Uint(ip)
	if err != nil {
		return err
	}

	bestPrefix := -1
	bestInterface := ""

	for _, str := range parts {
		str = strings.TrimSpace(str)

		if str == "" || strings.HasPrefix(str, "#") {
			continue
		}

		fields := strings.Fields(str)
		if len(fields) != 2 {
			return fmt.Errorf("invalid route entry: %s", str)
		}

		cidr := strings.Split(fields[0], "/")
		if len(cidr) != 2 {
			return fmt.Errorf("invalid CIDR: %s", fields[0])
		}

		networkUint, err := parseIPv4Uint(cidr[0])
		if err != nil {
			return err
		}

		prefix, err := strconv.Atoi(cidr[1])
		if err != nil {
			return err
		}

		if prefix < 0 || prefix > 32 {
			return fmt.Errorf("invalid prefix: %d", prefix)
		}

		mask := ^uint32(0) << (32 - prefix)

		if (ipUint & mask) == (networkUint & mask) {
			if prefix > bestPrefix {
				bestPrefix = prefix
				bestInterface = fields[1]
			}
		}
	}

	if bestPrefix == -1 {
		fmt.Println("unreachable true")
		return fmt.Errorf("no route to %s", ip)
	}

	fmt.Printf("via %s\n", bestInterface)
	fmt.Printf("prefix %d\n", bestPrefix)

	return nil
}
