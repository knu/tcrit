package ipc

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/knu/tcrit/internal/review"
)

func TestReviewCycleRoundTrip(t *testing.T) {
	sock := shortSockPath(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Aliveness probes connect and close without a request;
			// keep accepting until a real review-cycle arrives.
			req, err := ReadRequest(bufio.NewReader(conn))
			if err != nil || req.Type != "review-cycle" {
				conn.Close()
				continue
			}
			WriteMessage(conn, FinishPayload{
				Type:     "finish",
				Approved: false,
				Prompt:   "fix things",
				Comments: []review.ListedComment{{Scope: "line", Comment: review.Comment{ID: "c_1"}}},
			})
			conn.Close()
			return
		}
	}()

	if !Alive(sock) {
		t.Fatal("expected socket alive")
	}
	payload, err := ReviewCycle(sock)
	if err != nil {
		t.Fatalf("ReviewCycle: %v", err)
	}
	if payload.Approved || payload.Prompt != "fix things" || len(payload.Comments) != 1 {
		t.Errorf("unexpected payload: %+v", payload)
	}
}

func TestReviewCycleServerCloseWithoutFinish(t *testing.T) {
	sock := shortSockPath(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()

	if _, err := ReviewCycle(sock); err == nil {
		t.Fatal("expected error when the session closes without finishing")
	}
}

func TestListenReplacesStaleSocket(t *testing.T) {
	sock := shortSockPath(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // leaves the socket file behind on some platforms

	ln2, err := Listen(sock)
	if err != nil {
		t.Fatalf("expected stale socket replacement, got %v", err)
	}
	ln2.Close()
}

func TestListenRejectsLiveSocket(t *testing.T) {
	sock := shortSockPath(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	if _, err := Listen(sock); err == nil {
		t.Fatal("expected error for a live socket")
	}
}

func TestWaitAliveTimesOut(t *testing.T) {
	sock := shortSockPath(t)
	start := time.Now()
	if err := WaitAlive(sock, 300*time.Millisecond); err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout took too long")
	}
}

// shortSockPath returns a socket path short enough for the platform's
// sun_path limit (t.TempDir embeds long test names).
func shortSockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "tcrit-ipc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}
