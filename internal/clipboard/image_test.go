package clipboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMacClipboardBridge(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("AppKit requires macOS")
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "image with spaces.png")
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	quotedPath, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, setup string
		wantImage   bool
	}{
		{"empty", "", false},
		{"text", `pb.setStringForType('ordinary text', $.NSPasteboardTypeString);`, false},
		{"bitmap", `pb.setDataForType($.NSData.alloc.initWithBase64EncodedStringOptions('` + base64.StdEncoding.EncodeToString(b.Bytes()) + `', 0), 'public.png');`, true},
		{"file", `pb.writeObjects($.NSArray.arrayWithObject($.NSURL.fileURLWithPath(` + string(quotedPath) + `)));`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Use a private pasteboard; never read or overwrite the user's clipboard.
			script := strings.Replace(macImageScript, "const pb = $.NSPasteboard.generalPasteboard;", "const pb = $.NSPasteboard.pasteboardWithUniqueName;\n"+tt.setup, 1)
			out, err := run(context.Background(), "osascript", "-l", "JavaScript", "-e", script)
			if err != nil {
				t.Fatal(err)
			}
			if !tt.wantImage {
				if len(bytes.TrimSpace(out)) != 0 {
					t.Fatal("text clipboard became an image")
				}
				return
			}
			data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 3 {
				t.Fatalf("image bounds = %v", img.Bounds())
			}
		})
	}
}

func TestLimitedClipboardOutput(t *testing.T) {
	var b limitedBuffer
	if _, err := io.Copy(&b, bytes.NewReader(make([]byte, 8<<20))); err == nil {
		t.Fatal("unbounded clipboard output")
	}
}

func TestLinuxClipboardFormats(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	for _, wayland := range []bool{false, true} {
		t.Run(map[bool]string{false: "x11", true: "wayland"}[wayland], func(t *testing.T) {
			dir := t.TempDir()
			name := "xclip"
			t.Setenv("WAYLAND_DISPLAY", "")
			if wayland {
				name = "wl-paste"
				t.Setenv("WAYLAND_DISPLAY", "test-display")
			}
			// A command fixture verifies both format negotiation and exact read arguments.
			script := "#!/bin/sh\ncase \"$*\" in\n'--list-types'|'-selection clipboard -t TARGETS -o') printf '%s\\n' text/plain image/jpeg ;;\n'--no-newline --type image/jpeg'|'-selection clipboard -t image/jpeg -o') printf 'image bytes' ;;\n*) exit 1 ;;\nesac\n"
			if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			data, err := readLinuxImage(context.Background())
			if err != nil || string(data) != "image bytes" {
				t.Fatalf("clipboard = %q, %v", data, err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nprintf 'text/plain\\n'\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := readLinuxImage(context.Background()); err != ErrNoImage {
				t.Fatalf("text clipboard: %v", err)
			}
		})
	}
}
