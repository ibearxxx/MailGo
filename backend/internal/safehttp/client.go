// Package safehttp provides HTTP clients for requests whose destination may be
// influenced by a user. It rejects local, private, link-local, multicast, and
// otherwise non-global addresses both before the request and when dialing.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var errUnsafeDestination = errors.New("destination is not a public HTTP endpoint")

// NewClient returns a direct HTTP client protected against DNS rebinding and
// redirects to private network destinations.
func NewClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := publicIPs(ctx, host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			return nil, lastErr
		},
		ForceAttemptHTTP2: true,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return ValidateURL(req.Context(), req.URL)
		},
	}
}

// ValidateURL permits only HTTP(S) URLs with no credentials and a public host.
func ValidateURL(ctx context.Context, target *url.URL) error {
	if target == nil ||
		(target.Scheme != "http" && target.Scheme != "https") ||
		target.Hostname() == "" ||
		target.User != nil {
		return errUnsafeDestination
	}
	_, err := publicIPs(ctx, target.Hostname())
	return err
}

// ValidateHostname ensures host is syntactically valid and resolves exclusively
// to public addresses. Rejecting mixed answers prevents DNS-based bypasses.
func ValidateHostname(ctx context.Context, host string) error {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" || strings.ContainsAny(host, `/\@`) {
		return errUnsafeDestination
	}
	_, err := publicIPs(ctx, host)
	return err
}

func publicIPs(ctx context.Context, host string) ([]net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		if err == nil {
			err = errUnsafeDestination
		}
		return nil, err
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("%w: %s", errUnsafeDestination, ip)
		}
	}
	return ips, nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil &&
		ip.IsGlobalUnicast() &&
		!ip.IsPrivate() &&
		!ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsUnspecified()
}
