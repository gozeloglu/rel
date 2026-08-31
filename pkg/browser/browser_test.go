package browser

import (
	"errors"
	"testing"
)

func TestCommandPerPlatform(t *testing.T) {
	cases := []struct {
		goos     string
		wantName string
		wantArgs []string
	}{
		{"darwin", "open", []string{"https://example.com"}},
		{"linux", "xdg-open", []string{"https://example.com"}},
		{"openbsd", "xdg-open", []string{"https://example.com"}},
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", "https://example.com"}},
	}

	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			name, args, err := command(tc.goos, "https://example.com")
			if err != nil {
				t.Fatalf("command: %v", err)
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tc.wantArgs)
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}

func TestCommandRejectsUnknownPlatform(t *testing.T) {
	if _, _, err := command("plan9", "https://example.com"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}

func TestOpenAllWithNoURLs(t *testing.T) {
	if err := OpenAll(nil); err != nil {
		t.Errorf("err = %v, want nil when there is nothing to open", err)
	}
}
