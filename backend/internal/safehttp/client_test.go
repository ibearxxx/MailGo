package safehttp

import (
	"context"
	"net/url"
	"testing"
)

func TestValidateURLRejectsPrivateAndNonHTTPDestinations(t *testing.T) {
	tests := []string{
		"http://127.0.0.1/admin",
		"http://[::1]/admin",
		"http://169.254.169.254/latest/meta-data/",
		"file:///etc/passwd",
		"https://user:password@example.com/",
	}
	for _, raw := range tests {
		target, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateURL(context.Background(), target); err == nil {
			t.Errorf("ValidateURL(%q) unexpectedly succeeded", raw)
		}
	}
}
