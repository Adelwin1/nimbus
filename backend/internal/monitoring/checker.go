package monitoring

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxRedirects     = 3
	defaultMaxResponseBytes = 64 * 1024
)

var (
	ErrUnsafeDestination = errors.New("unsafe health-check destination")
	ErrResponseTooLarge  = errors.New("health-check response exceeds size limit")
)

type CheckResult struct {
	StatusCode   *int
	LatencyMS    int64
	Healthy      bool
	Slow         bool
	ErrorMessage *string
	CheckedAt    time.Time
}

type Checker struct {
	client           *http.Client
	resolver         *net.Resolver
	timeout          time.Duration
	maxResponseBytes int64
	maxRedirects     int
}

func NewChecker(timeout time.Duration) *Checker {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	checker := &Checker{
		resolver:         net.DefaultResolver,
		timeout:          timeout,
		maxResponseBytes: defaultMaxResponseBytes,
		maxRedirects:     defaultMaxRedirects,
	}

	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            checker.safeDialContext,
		ForceAttemptHTTP2:      true,
		DisableCompression:     true,
		MaxIdleConns:           20,
		MaxIdleConnsPerHost:    2,
		MaxConnsPerHost:        4,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    timeout,
		ResponseHeaderTimeout:  timeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 64 * 1024,
	}

	checker.client = &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(
			request *http.Request,
			previous []*http.Request,
		) error {
			if len(previous) >= checker.maxRedirects {
				return errors.New("health check exceeded redirect limit")
			}

			return validateTargetURL(request.URL)
		},
	}

	return checker
}

func (c *Checker) Check(
	ctx context.Context,
	targetURL string,
	latencyThresholdMS int,
) CheckResult {
	startedAt := time.Now()

	result := CheckResult{
		CheckedAt: startedAt.UTC(),
	}

	parsedURL, err := url.ParseRequestURI(
		strings.TrimSpace(targetURL),
	)
	if err != nil {
		return failedResult(
			result,
			startedAt,
			"health URL is invalid",
		)
	}

	if err := validateTargetURL(parsedURL); err != nil {
		return failedResult(
			result,
			startedAt,
			err.Error(),
		)
	}

	requestContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodGet,
		parsedURL.String(),
		nil,
	)
	if err != nil {
		return failedResult(
			result,
			startedAt,
			"could not create health-check request",
		)
	}

	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("User-Agent", "Nimbus-Health-Checker/1.0")
	request.Header.Set("Cache-Control", "no-cache")

	response, err := c.client.Do(request)
	if err != nil {
		return failedResult(
			result,
			startedAt,
			sanitizeCheckError(err),
		)
	}
	defer response.Body.Close()

	statusCode := response.StatusCode
	result.StatusCode = &statusCode

	bytesRead, readErr := io.Copy(
		io.Discard,
		io.LimitReader(
			response.Body,
			c.maxResponseBytes+1,
		),
	)

	result.LatencyMS = time.Since(startedAt).Milliseconds()
	result.CheckedAt = time.Now().UTC()

	if readErr != nil {
		message := "could not read health-check response"
		result.ErrorMessage = &message
		return result
	}

	if bytesRead > c.maxResponseBytes {
		message := ErrResponseTooLarge.Error()
		result.ErrorMessage = &message
		return result
	}

	result.Healthy =
		response.StatusCode >= http.StatusOK &&
			response.StatusCode < http.StatusMultipleChoices

	result.Slow =
		result.Healthy &&
			latencyThresholdMS > 0 &&
			result.LatencyMS > int64(latencyThresholdMS)

	if !result.Healthy {
		message := fmt.Sprintf(
			"health endpoint returned HTTP %d",
			response.StatusCode,
		)
		result.ErrorMessage = &message
	}

	return result
}

func (c *Checker) safeDialContext(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse destination address: %w", err)
	}

	if err := validateHostname(host); err != nil {
		return nil, err
	}

	if parsedIP := net.ParseIP(host); parsedIP != nil {
		if !isSafePublicIP(parsedIP) {
			return nil, ErrUnsafeDestination
		}

		dialer := &net.Dialer{
			Timeout:   c.timeout,
			KeepAlive: 30 * time.Second,
		}

		return dialer.DialContext(
			ctx,
			network,
			net.JoinHostPort(parsedIP.String(), port),
		)
	}

	addresses, err := c.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve health-check host: %w", err)
	}

	if len(addresses) == 0 {
		return nil, errors.New("health-check host has no IP addresses")
	}

	dialer := &net.Dialer{
		Timeout:   c.timeout,
		KeepAlive: 30 * time.Second,
	}

	var lastError error

	for _, resolvedAddress := range addresses {
		if !isSafePublicIP(resolvedAddress.IP) {
			return nil, ErrUnsafeDestination
		}

		connection, dialErr := dialer.DialContext(
			ctx,
			network,
			net.JoinHostPort(
				resolvedAddress.IP.String(),
				port,
			),
		)
		if dialErr == nil {
			return connection, nil
		}

		lastError = dialErr
	}

	if lastError != nil {
		return nil, lastError
	}

	return nil, errors.New("could not connect to health-check host")
}

func validateTargetURL(targetURL *url.URL) error {
	if targetURL == nil {
		return errors.New("health URL is invalid")
	}

	if targetURL.Scheme != "http" &&
		targetURL.Scheme != "https" {
		return errors.New("health URL must use HTTP or HTTPS")
	}

	if targetURL.Host == "" || targetURL.Hostname() == "" {
		return errors.New("health URL must include a host")
	}

	if targetURL.User != nil {
		return errors.New(
			"health URL must not contain embedded credentials",
		)
	}

	if targetURL.Fragment != "" {
		return errors.New(
			"health URL must not contain a fragment",
		)
	}

	if err := validateHostname(targetURL.Hostname()); err != nil {
		return err
	}

	port := targetURL.Port()
	if port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return errors.New("health URL contains an invalid port")
		}
	}

	if parsedIP := net.ParseIP(targetURL.Hostname()); parsedIP != nil {
		if !isSafePublicIP(parsedIP) {
			return ErrUnsafeDestination
		}
	}

	return nil
}

func validateHostname(hostname string) error {
	normalized := strings.TrimSuffix(
		strings.ToLower(strings.TrimSpace(hostname)),
		".",
	)

	if normalized == "" {
		return errors.New("health URL must include a host")
	}

	if normalized == "localhost" ||
		strings.HasSuffix(normalized, ".localhost") {
		return ErrUnsafeDestination
	}

	return nil
}

func isSafePublicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}

	address = address.Unmap()

	for _, blockedPrefix := range blockedNetworkPrefixes {
		if blockedPrefix.Contains(address) {
			return false
		}
	}

	return address.IsValid() &&
		!address.IsUnspecified() &&
		!address.IsLoopback() &&
		!address.IsPrivate() &&
		!address.IsLinkLocalUnicast() &&
		!address.IsMulticast()
}

func failedResult(
	result CheckResult,
	startedAt time.Time,
	message string,
) CheckResult {
	result.LatencyMS = time.Since(startedAt).Milliseconds()
	result.CheckedAt = time.Now().UTC()
	result.Healthy = false
	result.Slow = false

	sanitized := strings.TrimSpace(message)
	if len(sanitized) > 1000 {
		sanitized = sanitized[:1000]
	}

	result.ErrorMessage = &sanitized

	return result
}

func sanitizeCheckError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "health check timed out"

	case errors.Is(err, ErrUnsafeDestination):
		return ErrUnsafeDestination.Error()

	default:
		message := strings.ToLower(err.Error())

		if strings.Contains(message, "timeout") ||
			strings.Contains(message, "deadline exceeded") {
			return "health check timed out"
		}

		return "health endpoint could not be reached"
	}
}

var blockedNetworkPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),

	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
	netip.MustParsePrefix("2001:db8::/32"),
}
