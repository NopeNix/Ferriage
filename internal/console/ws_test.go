package console

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"strings"
	"testing"
)

func TestWsAccept(t *testing.T) {
	// RFC 6455 §1.3 example.
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := wsAccept(key); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	h := sha1.New()
	_, _ = h.Write([]byte(key + wsGUID))
	if base64.StdEncoding.EncodeToString(h.Sum(nil)) != want {
		t.Fatal("harness: RFC example mismatch")
	}
}

func maskedFrame(opcode byte, payload []byte) []byte {
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	out := []byte{0x80 | opcode}
	if len(payload) < 126 {
		out = append(out, 0x80|byte(len(payload)))
	} else {
		out = append(out, 0x80|126, byte(len(payload)>>8), byte(len(payload)))
	}
	out = append(out, mask...)
	for i, b := range payload {
		out = append(out, b^mask[i%4])
	}
	return out
}

func TestReadFrameMasked(t *testing.T) {
	raw := maskedFrame(0x2, []byte("echo hi"))
	br := bufio.NewReadWriter(bufio.NewReader(bytes.NewReader(raw)), bufio.NewWriter(&bytes.Buffer{}))
	payload, isText, err := readFrame(br)
	if err != nil {
		t.Fatal(err)
	}
	if isText || string(payload) != "echo hi" {
		t.Fatalf("payload=%q isText=%v", payload, isText)
	}
}

func TestReadFrameText(t *testing.T) {
	raw := maskedFrame(0x1, []byte(`{"type":"resize","width":80,"height":24}`))
	br := bufio.NewReadWriter(bufio.NewReader(bytes.NewReader(raw)), bufio.NewWriter(&bytes.Buffer{}))
	payload, isText, err := readFrame(br)
	if err != nil {
		t.Fatal(err)
	}
	if !isText || !strings.Contains(string(payload), "resize") {
		t.Fatalf("payload=%q isText=%v", payload, isText)
	}
}

func TestWriteFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	brw := bufio.NewReadWriter(bufio.NewReader(bytes.NewReader(nil)), bufio.NewWriter(&buf))
	if err := writeFrame(brw, 0x2, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	// Server frames are unmasked; read them back raw.
	got := buf.Bytes()
	if got[0] != 0x82 || got[1] != 5 || string(got[2:]) != "hello" {
		t.Fatalf("frame bytes: %v", got)
	}
}

func TestErrorFrame(t *testing.T) {
	if errorFrame("x") != `{"message":"x","type":"error"}` {
		t.Fatalf("got %s", errorFrame("x"))
	}
}
