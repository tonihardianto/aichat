package tooling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	WebhookURL  string          `json:"-"`
}

func ValidatePublicURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return errors.New("URL tidak valid")
	}
	if isPrivateHost(u.Hostname()) {
		return errors.New("URL lokal atau private tidak diizinkan")
	}
	return nil
}

func ValidateWebhookURL(raw string) error {
	if err := ValidatePublicURL(raw); err != nil {
		return err
	}
	if !strings.HasPrefix(raw, "https://") {
		return errors.New("webhook harus memakai HTTPS")
	}
	return nil
}

func isPrivateHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}
	return false
}

func publicTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(address)
			if err != nil || isPrivateHost(host) {
				return nil, errors.New("target private tidak diizinkan")
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
					return nil, errors.New("target private tidak diizinkan")
				}
			}
			return dialer.DialContext(ctx, network, address)
		},
	}
}

func fetch(ctx context.Context, rawURL, method string, body io.Reader) (string, error) {
	if err := ValidatePublicURL(rawURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: publicTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxResponseBytes {
		return "", errors.New("respons tool melebihi 1 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("target mengembalikan status %d", resp.StatusCode)
	}
	return string(data), nil
}

func WebFetch(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &in); err != nil || in.URL == "" {
		return "", errors.New("parameter url wajib diisi")
	}
	body, err := fetch(ctx, in.URL, http.MethodGet, nil)
	if err != nil {
		return "", err
	}
	return cleanText(body), nil
}

func Webhook(ctx context.Context, rawURL string, args json.RawMessage) (string, error) {
	if err := ValidateWebhookURL(rawURL); err != nil {
		return "", err
	}
	return fetchJSON(ctx, rawURL, args)
}

func fetchJSON(ctx context.Context, rawURL string, args json.RawMessage) (string, error) {
	if err := ValidateWebhookURL(rawURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(string(args)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, Transport: publicTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxResponseBytes {
		return "", errors.New("respons tool melebihi 1 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("target mengembalikan status %d", resp.StatusCode)
	}
	return string(data), nil
}
