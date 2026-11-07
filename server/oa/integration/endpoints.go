package integration

import (
	"net"
	"net/url"
	"strings"
)

func localHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	switch strings.ToLower(host) {
	case "localhost", "host.docker.internal", "wecom-mock", "oss-mock":
		return true
	default:
		return false
	}
}

func endpoint(raw string, allowPath bool) (*url.URL, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || strings.ContainsAny(raw, "\r\n\t#") ||
		(!allowPath && parsed.Path != "" && parsed.Path != "/") {
		return nil, false
	}
	return parsed, true
}

func ValidateOrigin(raw string, production bool) bool {
	parsed, ok := endpoint(raw, false)
	return ok && (parsed.Scheme == "https" || (!production && localHost(parsed.Hostname())))
}
