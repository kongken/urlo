package clicks

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

// HashIP returns sha256(ip+salt) hex truncated to 16 chars, suitable for
// uniqueness-without-identifiability. Empty ip returns "".
func HashIP(ip, salt string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(ip + "|" + salt))
	return hex.EncodeToString(sum[:8])
}

// ReferrerHost extracts the host portion of a Referer header. Returns
// "" if the header is empty or unparseable.
func ReferrerHost(referer string) string {
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	return strings.ToLower(host)
}

// FirstLang returns the first language tag from an Accept-Language
// header (e.g. "en-US,en;q=0.9" -> "en-US").
func FirstLang(acceptLanguage string) string {
	if acceptLanguage == "" {
		return ""
	}
	first, _, _ := strings.Cut(acceptLanguage, ",")
	first, _, _ = strings.Cut(first, ";")
	return strings.TrimSpace(first)
}

// ParseUA performs a tiny, dependency-free user-agent classification.
// Returns (browser, os, device, isBot). Good enough for "rough breakdown
// + bot flag"; swap in a real parser later if needed.
func ParseUA(ua string) (browser, os, device string, isBot bool) {
	if ua == "" {
		return "", "", "other", false
	}
	low := strings.ToLower(ua)

	for _, sig := range botSignatures {
		if strings.Contains(low, sig) {
			isBot = true
			device = "bot"
			break
		}
	}

	switch {
	case strings.Contains(low, "edg/"):
		browser = "Edge"
	case strings.Contains(low, "opr/"), strings.Contains(low, "opera"):
		browser = "Opera"
	case strings.Contains(low, "chrome/"):
		browser = "Chrome"
	case strings.Contains(low, "firefox/"):
		browser = "Firefox"
	case strings.Contains(low, "safari/"):
		browser = "Safari"
	case isBot:
		browser = "Bot"
	default:
		browser = "Other"
	}

	switch {
	case strings.Contains(low, "android"):
		os = "Android"
	case strings.Contains(low, "iphone"), strings.Contains(low, "ipad"), strings.Contains(low, "ipod"):
		os = "iOS"
	case strings.Contains(low, "mac os x"), strings.Contains(low, "macintosh"):
		os = "macOS"
	case strings.Contains(low, "windows"):
		os = "Windows"
	case strings.Contains(low, "linux"):
		os = "Linux"
	default:
		os = "Other"
	}

	if device == "" {
		switch {
		case strings.Contains(low, "ipad"), strings.Contains(low, "tablet"):
			device = "tablet"
		case strings.Contains(low, "mobile"), strings.Contains(low, "iphone"), strings.Contains(low, "android"):
			device = "mobile"
		default:
			device = "desktop"
		}
	}

	return browser, os, device, isBot
}

var botSignatures = []string{
	"bot", "crawler", "spider", "slurp", "facebookexternalhit",
	"bingpreview", "duckduckgo", "yandex", "baiduspider",
	"applebot", "googlebot", "headlesschrome", "curl/", "wget/",
	"python-requests", "go-http-client",
}
