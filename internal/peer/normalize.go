package peer

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"agentpal/internal/constants"
)

func Normalize(input string, defaultPort int) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("peer IP is required")
	}
	if defaultPort == 0 {
		defaultPort = constants.DefaultPort
	}
	if !strings.Contains(input, "://") {
		input = "http://" + input
	}
	parsed, err := url.Parse(input)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" {
		return "", errors.New("only http peer URLs are supported")
	}
	host := parsed.Hostname()
	if host == "" {
		return "", errors.New("peer host is required")
	}
	port := parsed.Port()
	if port == "" {
		port = strconv.Itoa(defaultPort)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", errors.New("peer port is invalid")
	}
	return "http://" + net.JoinHostPort(host, port), nil
}
