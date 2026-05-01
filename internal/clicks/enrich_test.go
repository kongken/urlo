package clicks

import "testing"

func TestParseUA(t *testing.T) {
	cases := []struct {
		name             string
		ua               string
		wantBrowser      string
		wantOS           string
		wantDevice       string
		wantBot          bool
	}{
		{
			name:        "chrome on macos",
			ua:          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			wantBrowser: "Chrome",
			wantOS:      "macOS",
			wantDevice:  "desktop",
		},
		{
			name:        "safari on iphone",
			ua:          "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantBrowser: "Safari",
			wantOS:      "iOS",
			wantDevice:  "mobile",
		},
		{
			name:        "googlebot",
			ua:          "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			wantBrowser: "Bot",
			wantOS:      "Other",
			wantDevice:  "bot",
			wantBot:     true,
		},
		{
			name:        "firefox windows",
			ua:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:124.0) Gecko/20100101 Firefox/124.0",
			wantBrowser: "Firefox",
			wantOS:      "Windows",
			wantDevice:  "desktop",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, o, d, bot := ParseUA(tc.ua)
			if b != tc.wantBrowser || o != tc.wantOS || d != tc.wantDevice || bot != tc.wantBot {
				t.Fatalf("ParseUA = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
					b, o, d, bot, tc.wantBrowser, tc.wantOS, tc.wantDevice, tc.wantBot)
			}
		})
	}
}

func TestReferrerHost(t *testing.T) {
	cases := map[string]string{
		"":                                 "",
		"https://www.google.com/search?q=x": "www.google.com",
		"http://Foo.Bar":                    "foo.bar",
		"not a url":                         "",
	}
	for in, want := range cases {
		if got := ReferrerHost(in); got != want {
			t.Errorf("ReferrerHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstLang(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"en-US,en;q=0.9":    "en-US",
		"  zh-CN ; q=1.0":   "zh-CN",
		"fr":                "fr",
	}
	for in, want := range cases {
		if got := FirstLang(in); got != want {
			t.Errorf("FirstLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHashIP(t *testing.T) {
	if HashIP("", "salt") != "" {
		t.Fatal("empty ip must hash to empty")
	}
	a := HashIP("1.2.3.4", "salt")
	b := HashIP("1.2.3.4", "salt")
	c := HashIP("1.2.3.5", "salt")
	if a == "" || a != b || a == c {
		t.Fatalf("hash determinism broken: a=%q b=%q c=%q", a, b, c)
	}
	if len(a) != 16 {
		t.Fatalf("expected 16-char hash, got %d", len(a))
	}
}
