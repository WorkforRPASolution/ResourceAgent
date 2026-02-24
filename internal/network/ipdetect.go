package network

import (
	"fmt"
	"net"
	"regexp"
)

// IPInfo holds detected IP address information.
type IPInfo struct {
	IPAddr      string   // External IP (first non-loopback IPv4)
	IPAddrLocal string   // Internal IP matching pattern, or "_" if no pattern
	AllIPs      []string // All detected non-loopback IPv4 addresses
}

// DetectIPs discovers network interfaces and classifies IPs.
// privateIPPattern is a regex to identify the local/internal IP.
// overrideIP, if non-empty, is used as IPAddr instead of auto-detection.
// If privateIPPattern is empty, IPAddrLocal is set to "_" (Redis key convention).
func DetectIPs(privateIPPattern string, overrideIP string) (*IPInfo, error) {
	// 1. Validate regex pattern if non-empty
	var re *regexp.Regexp
	if privateIPPattern != "" {
		var err error
		re, err = regexp.Compile(privateIPPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid private IP pattern %q: %w", privateIPPattern, err)
		}
	}

	// 2. Enumerate all non-loopback IPv4 addresses
	allIPs, err := detectIPv4Addresses()
	if err != nil {
		return nil, fmt.Errorf("failed to detect IPs: %w", err)
	}

	info := &IPInfo{
		AllIPs:      allIPs,
		IPAddrLocal: "_", // default if no pattern or no match
	}

	// 3. Determine IPAddr
	if overrideIP != "" {
		info.IPAddr = overrideIP
	} else if len(allIPs) > 0 {
		info.IPAddr = allIPs[0]
	}

	// 4. Find local IP matching pattern
	if re != nil {
		for _, ip := range allIPs {
			if re.MatchString(ip) {
				info.IPAddrLocal = ip
				// If no override, use a non-matching IP as external IP
				if overrideIP == "" {
					for _, candidate := range allIPs {
						if candidate != ip {
							info.IPAddr = candidate
							break
						}
					}
				}
				break
			}
		}
	}

	return info, nil
}

// detectIPv4Addresses returns all non-loopback IPv4 addresses.
func detectIPv4Addresses() ([]string, error) {
	var ips []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Only IPv4, non-loopback
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}

			ips = append(ips, ip.String())
		}
	}

	return ips, nil
}
