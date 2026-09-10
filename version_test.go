package cronitor

import (
	"regexp"
	"testing"
)

func TestVersionIsSemver(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Fatalf("Version %q is not MAJOR.MINOR.PATCH", Version)
	}
}

func TestUserAgentCarriesVersion(t *testing.T) {
	if want := "cronitor-go/" + Version; userAgent != want {
		t.Fatalf("userAgent = %q, want %q", userAgent, want)
	}
}
