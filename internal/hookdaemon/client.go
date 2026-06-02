package hookdaemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

// dialTimeout bounds the connect attempt; the daemon answers in ~1ms locally.
const dialTimeout = 5 * time.Second

// SendRequest dials the daemon socket, sends one Request, and returns the
// Response. It is the core of the `engram hook <EventName>` shim. No socat
// dependency — pure Go.
func SendRequest(sockPath string, req Request) (Response, error) {
	conn, err := net.DialTimeout("unix", sockPath, dialTimeout)
	if err != nil {
		return Response{}, fmt.Errorf("dial %s: %w", sockPath, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(perConnTimeout))

	b, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return Response{}, fmt.Errorf("write request: %w", err)
	}

	var resp Response
	dec := json.NewDecoder(bufio.NewReader(io.LimitReader(conn, 8<<20)))
	if err := dec.Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	return resp, nil
}

// SendWithRetry tries SendRequest, and on a dial failure invokes start (to lazy-
// start the daemon), waits, and retries once. start may be nil (no auto-start).
// This implements Option C's lazy-start behavior in the shim client.
func SendWithRetry(sockPath string, req Request, start func() error, wait time.Duration) (Response, error) {
	resp, err := SendRequest(sockPath, req)
	if err == nil {
		return resp, nil
	}
	if start == nil {
		return Response{}, err
	}
	if startErr := start(); startErr != nil {
		return Response{}, fmt.Errorf("start daemon: %w (after dial error: %v)", startErr, err)
	}
	time.Sleep(wait)
	return SendRequest(sockPath, req)
}
