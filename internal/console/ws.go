// Minimal RFC 6455 framing for the console WebSocket (ADR 0005).
// No new dependency: the peer already hand-rolls small wire
// formats (ADR 0001 manifest, ADR 0002 snapshot). Only the
// frames this server sends/receives are implemented: text and
// binary, with client-to-server masking.
package console

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// readFrame reads one frame from a client (mask bit required).
func readFrame(br *bufio.ReadWriter) (payload []byte, isText bool, err error) {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, false, err
	}
	opcode := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	length := int64(hdr[1] & 0x7f)
	switch length {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(br, b[:]); err != nil {
			return nil, false, err
		}
		length = int64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(br, b[:]); err != nil {
			return nil, false, err
		}
		length = int64(binary.BigEndian.Uint64(b[:]))
	}
	if length < 0 || length > 1<<20 {
		return nil, false, fmt.Errorf("console: frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(br, mask[:]); err != nil {
			return nil, false, err
		}
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(br, buf); err != nil {
		return nil, false, err
	}
	if masked {
		for i := range buf {
			buf[i] ^= mask[i%4]
		}
	}
	switch opcode {
	case 0x1:
		return buf, true, nil
	case 0x2:
		return buf, false, nil
	case 0x8: // close
		return nil, false, fmt.Errorf("console: client closed")
	case 0x9: // ping -> pong, then keep reading
		_ = writeFrame(br, 0xA, buf)
		return readFrame(br)
	case 0xA:
		return readFrame(br)
	default:
		return readFrame(br)
	}
}

// writeFrame sends one unmasked server frame.
func writeFrame(br *bufio.ReadWriter, opcode byte, payload []byte) error {
	var hdr []byte
	hdr = append(hdr, 0x80|opcode)
	if len(payload) < 126 {
		hdr = append(hdr, byte(len(payload)))
	} else if len(payload) < 65536 {
		hdr = append(hdr, 126, byte(len(payload)>>8), byte(len(payload)))
	} else {
		hdr = append(hdr, 127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(len(payload)))
		hdr = append(hdr, b[:]...)
	}
	if _, err := br.Write(hdr); err != nil {
		return err
	}
	if _, err := br.Write(payload); err != nil {
		return err
	}
	return br.Flush()
}
