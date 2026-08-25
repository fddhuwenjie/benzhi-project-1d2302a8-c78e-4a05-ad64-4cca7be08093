package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

func resolveAddress(flagAddress string) (string, error) {
	addr := strings.TrimSpace(flagAddress)
	if addr == "" {
		if rawPort := strings.TrimSpace(os.Getenv("PORT")); rawPort != "" {
			port, err := strconv.Atoi(rawPort)
			if err != nil || port < 1 || port > 65535 {
				return "", fmt.Errorf("PORT 必须是 1 至 65535 的端口号")
			}
			addr = net.JoinHostPort("127.0.0.1", rawPort)
		} else {
			addr = defaultAddress
		}
	}
	host, rawPort, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("监听地址必须为 host:port: %w", err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("监听端口无效")
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("监听地址必须使用回环主机")
		}
	}
	return addr, nil
}
