package deployment

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/google/uuid"
)

const (
	maxWebhookRedirects     = 3
	maxWebhookResponseBytes = 64 * 1024
)

var (
	ErrWebhookNotConfigured = errors.New("deployment webhook is not configured")
	ErrUnsafeWebhookTarget  = errors.New("unsafe webhook destination")
	ErrWebhookResponseLarge = errors.New("webhook response exceeds size limit")
	ErrWebhookRejected      = errors.New("webhook returned an unsuccessful response")
)

type WebhookPayload struct {
	DeploymentID   uuid.UUID `json:"deployment_id"`
	ApplicationID  uuid.UUID `json:"application_id"`
	Version        string    `json:"version"`
	CommitSHA      *string   `json:"commit_sha,omitempty"`
	ReleaseNotes   string    `json:"release_notes"`
	DeploymentType string    `json:"deployment_type"`
	TriggeredAt    time.Time `json:"triggered_at"`

	IncidentID          *uuid.UUID `json:"incident_id,omitempty"`
	RollbackFromVersion *string    `json:"rollback_from_version,omitempty"`
}

type WebhookResult struct {
	StatusCode   int
	LatencyMS    int64
	ResponseBody string
	CompletedAt  time.Time
}

type WebhookExecutor struct {
	cipher           SecretCipher
	client           *http.Client
	resolver         *net.Resolver
	timeout          time.Duration
	maxResponseBytes int64
	maxRedirects     int
}

func NewWebhookExecutor(
	cipher SecretCipher,
	timeout time.Duration,
) *WebhookExecutor {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	executor := &WebhookExecutor{
		cipher:           cipher,
		resolver:         net.DefaultResolver,
		timeout:          timeout,
		maxResponseBytes: maxWebhookResponseBytes,
		maxRedirects:     maxWebhookRedirects,
	}

	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            executor.safeDialContext,
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

	executor.client = &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(
			request *http.Request,
			previous []*http.Request,
		) error {
			if len(previous) >= executor.maxRedirects {
				return errors.New("webhook exceeded redirect limit")
			}

			return validateWebhookURL(request.URL)
		},
	}

	return executor
}

func (e *WebhookExecutor) Execute(
	ctx context.Context,
	encryptedURL *string,
	encryptedToken *string,
	payload WebhookPayload,
) (WebhookResult, error) {
	if encryptedURL == nil || strings.TrimSpace(*encryptedURL) == "" {
		return WebhookResult{}, ErrWebhookNotConfigured
	}

	webhookURL, err := e.cipher.Decrypt(*encryptedURL)
	if err != nil {
		return WebhookResult{}, fmt.Errorf(
			"decrypt deployment webhook URL: %w",
			err,
		)
	}

	parsedURL, err := url.ParseRequestURI(
		strings.TrimSpace(webhookURL),
	)
	if err != nil {
		return WebhookResult{}, errors.New(
			"stored deployment webhook URL is invalid",
		)
	}

	if err := validateWebhookURL(parsedURL); err != nil {
		return WebhookResult{}, err
	}

	var token string

	if encryptedToken != nil &&
		strings.TrimSpace(*encryptedToken) != "" {
		token, err = e.cipher.Decrypt(*encryptedToken)
		if err != nil {
			return WebhookResult{}, fmt.Errorf(
				"decrypt deployment webhook token: %w",
				err,
			)
		}
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return WebhookResult{}, fmt.Errorf(
			"encode deployment webhook payload: %w",
			err,
		)
	}

	requestContext, cancel := context.WithTimeout(
		ctx,
		e.timeout,
	)
	defer cancel()

	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		parsedURL.String(),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return WebhookResult{}, fmt.Errorf(
			"create deployment webhook request: %w",
			err,
		)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("User-Agent", "Nimbus-Deployment-Webhook/1.0")
	request.Header.Set("Cache-Control", "no-cache")

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	startedAt := time.Now()

	response, err := e.client.Do(request)
	if err != nil {
		return WebhookResult{}, classifyWebhookError(err)
	}
	defer response.Body.Close()

	body, err := readLimitedResponse(
		response.Body,
		e.maxResponseBytes,
	)
	if err != nil {
		return WebhookResult{}, err
	}

	result := WebhookResult{
		StatusCode:   response.StatusCode,
		LatencyMS:    time.Since(startedAt).Milliseconds(),
		ResponseBody: sanitizeResponseBody(body),
		CompletedAt:  time.Now().UTC(),
	}

	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return result, fmt.Errorf(
			"%w: HTTP %d",
			ErrWebhookRejected,
			response.StatusCode,
		)
	}

	return result, nil
}

func readLimitedResponse(
	reader io.Reader,
	limit int64,
) ([]byte, error) {
	limitedReader := io.LimitReader(reader, limit+1)

	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, errors.New(
			"could not read webhook response",
		)
	}

	if int64(len(body)) > limit {
		return nil, ErrWebhookResponseLarge
	}

	return body, nil
}

func sanitizeResponseBody(body []byte) string {
	value := strings.TrimSpace(string(body))

	if value == "" {
		return ""
	}

	if len(value) > 2000 {
		value = value[:2000]
	}

	return value
}

func classifyWebhookError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("deployment webhook timed out")

	case errors.Is(err, ErrUnsafeWebhookTarget):
		return ErrUnsafeWebhookTarget

	default:
		message := strings.ToLower(err.Error())

		if strings.Contains(message, "timeout") ||
			strings.Contains(message, "deadline exceeded") {
			return errors.New("deployment webhook timed out")
		}

		return errors.New("deployment webhook could not be reached")
	}
}

func (e *WebhookExecutor) safeDialContext(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf(
			"parse webhook destination address: %w",
			err,
		)
	}

	if err := validateWebhookHostname(host); err != nil {
		return nil, err
	}

	if parsedIP := net.ParseIP(host); parsedIP != nil {
		if !isSafeWebhookIP(parsedIP) {
			return nil, ErrUnsafeWebhookTarget
		}

		return e.dialResolvedAddress(
			ctx,
			network,
			parsedIP,
			port,
		)
	}

	addresses, err := e.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, errors.New(
			"deployment webhook host could not be resolved",
		)
	}

	if len(addresses) == 0 {
		return nil, errors.New(
			"deployment webhook host has no IP addresses",
		)
	}

	var lastError error

	for _, resolvedAddress := range addresses {
		if !isSafeWebhookIP(resolvedAddress.IP) {
			return nil, ErrUnsafeWebhookTarget
		}

		connection, dialErr := e.dialResolvedAddress(
			ctx,
			network,
			resolvedAddress.IP,
			port,
		)
		if dialErr == nil {
			return connection, nil
		}

		lastError = dialErr
	}

	if lastError != nil {
		return nil, lastError
	}

	return nil, errors.New(
		"deployment webhook could not be reached",
	)
}

func (e *WebhookExecutor) dialResolvedAddress(
	ctx context.Context,
	network string,
	ip net.IP,
	port string,
) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   e.timeout,
		KeepAlive: 30 * time.Second,
	}

	return dialer.DialContext(
		ctx,
		network,
		net.JoinHostPort(ip.String(), port),
	)
}

func validateWebhookURL(targetURL *url.URL) error {
	if targetURL == nil {
		return errors.New("deployment webhook URL is invalid")
	}

	if targetURL.Scheme != "http" &&
		targetURL.Scheme != "https" {
		return errors.New(
			"deployment webhook must use HTTP or HTTPS",
		)
	}

	if targetURL.Host == "" || targetURL.Hostname() == "" {
		return errors.New(
			"deployment webhook must include a host",
		)
	}

	if targetURL.User != nil {
		return errors.New(
			"deployment webhook must not contain embedded credentials",
		)
	}

	if targetURL.Fragment != "" {
		return errors.New(
			"deployment webhook must not contain a URL fragment",
		)
	}

	if err := validateWebhookHostname(
		targetURL.Hostname(),
	); err != nil {
		return err
	}

	if port := targetURL.Port(); port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil ||
			parsedPort < 1 ||
			parsedPort > 65535 {
			return errors.New(
				"deployment webhook contains an invalid port",
			)
		}
	}

	if parsedIP := net.ParseIP(targetURL.Hostname()); parsedIP != nil {
		if !isSafeWebhookIP(parsedIP) {
			return ErrUnsafeWebhookTarget
		}
	}

	return nil
}

func validateWebhookHostname(hostname string) error {
	normalized := strings.TrimSuffix(
		strings.ToLower(strings.TrimSpace(hostname)),
		".",
	)

	if normalized == "" {
		return errors.New(
			"deployment webhook must include a host",
		)
	}

	if normalized == "localhost" ||
		strings.HasSuffix(normalized, ".localhost") {
		return ErrUnsafeWebhookTarget
	}

	return nil
}

func isSafeWebhookIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}

	address = address.Unmap()

	for _, blockedPrefix := range blockedWebhookPrefixes {
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

var blockedWebhookPrefixes = []netip.Prefix{
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
