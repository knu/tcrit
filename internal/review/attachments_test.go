package review

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAttachmentsPersistAndClear(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, err := OpenSession("", "0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	first, err := s.SaveAttachment(imageData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveAttachment(imageData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("repeated paste overwrote an image")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSession("", s.Key)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.HasAttachments() {
		t.Fatal("images lost on reopen")
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, first))
	if err != nil || !bytes.Equal(data, imageData.Bytes()) {
		t.Fatalf("saved image: %v", err)
	}
	info, err := os.Stat(filepath.Join(s.Dir, first))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("image permissions = %o", info.Mode().Perm())
	}
	if err := reopened.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, first)); !os.IsNotExist(err) {
		t.Fatalf("image survives clear: %v", err)
	}
}

func TestAttachmentValidation(t *testing.T) {
	s := &Session{Dir: t.TempDir()}
	for _, data := range [][]byte{nil, []byte("not an image"), bytes.Repeat([]byte{0}, MaxAttachmentBytes+1)} {
		if _, err := s.SaveAttachment(data); err == nil {
			t.Fatal("accepted invalid image")
		}
	}
	if s.HasAttachments() {
		t.Fatal("invalid paste left files behind")
	}
}
