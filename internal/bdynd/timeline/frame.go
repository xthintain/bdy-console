package timeline

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

// frameKinds maps block kinds to their frame magic bytes. The leading 0xBD 0x0B
// prefix keeps the magic distinct from ordinary text in user files.
var frameKinds = map[Kind][4]byte{
	KindNode:       {0xBD, 0x0B, 'N', 'B'},
	KindSegment:    {0xBD, 0x0B, 'S', 'P'},
	KindArchive:    {0xBD, 0x0B, 'A', 'K'},
	KindCheckpoint: {0xBD, 0x0B, 'C', 'K'},
}

const (
	frameMagicHead1 = 0xBD
	frameMagicHead2 = 0x0B

	flagCompressed   byte = 0x01
	flagDelta        byte = 0x02
	flagFullSnapshot byte = 0x04
	flagObject       byte = 0x08
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// writeUvarint appends an LEB128 unsigned integer to buf.
func writeUvarint(buf []byte, v uint64) []byte {
	var tmp [10]byte
	n := binary.PutUvarint(tmp[:], v)
	return append(buf, tmp[:n]...)
}

// writeLPSTR appends a length-prefixed UTF-8 string to buf.
func writeLPSTR(buf []byte, s string) []byte {
	buf = writeUvarint(buf, uint64(len(s)))
	return append(buf, s...)
}

// readLPSTR reads a length-prefixed string from data at offset *off.
func readLPSTR(data []byte, off *int) (string, error) {
	n, read := binary.Uvarint(data[*off:])
	if read <= 0 {
		return "", fmt.Errorf("readLPSTR: bad length prefix at offset %d", *off)
	}
	*off += read
	if uint64(*off)+n > uint64(len(data)) {
		return "", fmt.Errorf("readLPSTR: string out of range")
	}
	s := string(data[*off : *off+int(n)])
	*off += int(n)
	return s, nil
}

// frame layout:
//
//	magic(4) | kind(1) | flags(1) | payloadLen uvarint | payload | crc32(4)
//
// appendFrame builds one frame with this exact layout.
func appendFrame(out []byte, magic [4]byte, kind, flags byte, payload []byte) []byte {
	start := len(out)
	out = append(out, magic[:]...)
	out = append(out, kind, flags)
	out = writeUvarint(out, uint64(len(payload)))
	out = append(out, payload...)
	checksum := crc32.Checksum(out[start:], castagnoli)
	var c [4]byte
	binary.LittleEndian.PutUint32(c[:], checksum)
	return append(out, c[:]...)
}

// scanFrame parses a frame at off. It returns kind, flags, payload length, the
// payload start offset, and the total frame size.
func scanFrame(data []byte, off int) (kind, flags byte, payloadLen uint64, payloadStart, total int, ok bool) {
	if off+8 > len(data) {
		return 0, 0, 0, 0, 0, false
	}
	if data[off] != frameMagicHead1 || data[off+1] != frameMagicHead2 {
		return 0, 0, 0, 0, 0, false
	}
	kind = data[off+4]
	flags = data[off+5]
	payloadLen, read := binary.Uvarint(data[off+6:])
	if read <= 0 {
		return 0, 0, 0, 0, 0, false
	}
	header := 6 + read
	payloadStart = off + header
	total = header + int(payloadLen) + 4 // + crc32
	if off+total > len(data) {
		return 0, 0, 0, 0, 0, false
	}
	want := binary.LittleEndian.Uint32(data[off+total-4 : off+total])
	got := crc32.Checksum(data[off:off+header+int(payloadLen)], castagnoli)
	if want != got {
		return 0, 0, 0, 0, 0, false
	}
	return kind, flags, payloadLen, payloadStart, total, true
}
