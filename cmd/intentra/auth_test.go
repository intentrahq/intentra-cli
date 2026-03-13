package main

import (
	"testing"
)

func init() {
	browserLauncher = func(url string) error { return nil }
}

func TestOpenBrowserRejectsHTTP(t *testing.T) {
	err := openBrowser("http://evil.com")
	if err == nil {
		t.Error("openBrowser should reject non-HTTPS URLs")
	}
}

func TestOpenBrowserRejectsJavascript(t *testing.T) {
	err := openBrowser("javascript:alert(1)")
	if err == nil {
		t.Error("openBrowser should reject javascript: URLs")
	}
}

func TestOpenBrowserAcceptsHTTPS(t *testing.T) {
	err := openBrowser("https://example.com")
	if err != nil {
		t.Errorf("openBrowser should accept HTTPS URLs, got: %v", err)
	}
}

func TestCapitalizeFirst(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"free", "Free"},
		{"pro", "Pro"},
		{"enterprise", "Enterprise"},
		{"", ""},
		{"A", "A"},
		{"already Capitalized", "Already Capitalized"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := capitalizeFirst(tt.input)
			if got != tt.want {
				t.Errorf("capitalizeFirst(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
