package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// isPublicIP refuses loopback, private, link-local and similar ranges so
// the app can only ever talk to public websites.
func isPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil && (cgnat.Contains(v4) || v4[0] == 0) {
		return false
	}
	return true
}

func proxyHosts() map[string]bool {
	hosts := map[string]bool{}
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if v := os.Getenv(k); v != "" {
			if u, err := url.Parse(v); err == nil && u.Hostname() != "" {
				hosts[u.Hostname()] = true
			}
		}
	}
	return hosts
}

var errPrivateAddress = errors.New("refusing to connect to a non-public address")

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	allowed := proxyHosts()
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if allowed[host] {
				return dialer.DialContext(ctx, network, addr)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var lastErr error = errPrivateAddress
			for _, ip := range ips {
				if !isPublicIP(ip.IP) {
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 25 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 6 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return errors.New("store redirected to an insecure address")
			}
			return nil
		},
	}
}

// validStoreDomain cleans a pasted website address down to a hostname
// and rejects anything that isn't a public domain name.
func validStoreDomain(input string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(input))
	if s == "" {
		return "", errors.New("enter the brand's website, like glossier.com")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return "", errors.New("that doesn't look like a website address")
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	if net.ParseIP(host) != nil {
		return "", errors.New("use the brand's website name, not an IP address")
	}
	if !strings.Contains(host, ".") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".localhost") || host == "localhost" || len(host) > 253 {
		return "", errors.New("that isn't a public website")
	}
	for _, r := range host {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
			return "", errors.New("that website address has unusual characters")
		}
	}
	if u.Port() != "" && u.Port() != "443" {
		return "", errors.New("use the brand's normal website address")
	}
	return host, nil
}
