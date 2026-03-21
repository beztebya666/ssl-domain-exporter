package checker

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type HTTPResult struct {
	StatusCode     int
	RedirectsHTTPS bool
	HSTSEnabled    bool
	HSTSMaxAge     string
	ResponseTimeMs int64
	FinalURL       string
	Error          string
}

func CheckHTTP(domain string, port int, timeout time.Duration, customCAPEM string, rc *ResolveContext) *HTTPResult {
	result := &HTTPResult{}

	roots, rootsErr := loadRootPool(customCAPEM)
	if rootsErr != nil {
		result.Error = rootsErr.Error()
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots},
	}

	// Use custom DNS resolver for HTTP connections if available
	if rc != nil && len(rc.Servers) > 0 {
		transport.DialContext = rc.CustomDialer(timeout)
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	httpURL, httpsURL := httpCheckURLs(domain, port)

	start := time.Now()
	resp, err := client.Get(httpURL)
	result.ResponseTimeMs = time.Since(start).Milliseconds()

	if err != nil {
		start2 := time.Now()
		resp2, err2 := client.Get(httpsURL)
		result.ResponseTimeMs = time.Since(start2).Milliseconds()
		if err2 != nil {
			result.Error = combineErrors(result.Error, fmt.Sprintf("http: %v; https: %v", err, err2))
			return result
		}
		defer resp2.Body.Close()
		fillHTTPResult(result, resp2)
		return result
	}
	defer resp.Body.Close()

	fillHTTPResult(result, resp)
	return result
}

func httpCheckURLs(domain string, port int) (httpURL string, httpsURL string) {
	if port <= 0 {
		port = 443
	}
	httpURL = "http://" + domain
	httpsURL = "https://" + domain
	if port != 443 {
		httpsURL = fmt.Sprintf("https://%s:%d", domain, port)
	}
	if port != 80 && port != 443 {
		httpURL = fmt.Sprintf("http://%s:%d", domain, port)
	}
	return httpURL, httpsURL
}

func fillHTTPResult(result *HTTPResult, resp *http.Response) {
	result.StatusCode = resp.StatusCode
	result.FinalURL = resp.Request.URL.String()
	result.RedirectsHTTPS = strings.HasPrefix(strings.ToLower(result.FinalURL), "https://")
	result.HSTSEnabled, result.HSTSMaxAge = parseHSTS(resp.Header.Get("Strict-Transport-Security"))
}

func combineErrors(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}

func parseHSTS(header string) (bool, string) {
	if header == "" {
		return false, ""
	}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "max-age=") {
			return true, strings.TrimPrefix(strings.ToLower(part), "max-age=")
		}
	}
	return true, ""
}
