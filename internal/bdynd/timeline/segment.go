package timeline

import (
	"bytes"
	"fmt"
)

// encodeSegmentBlock builds a SegmentBlock from its header and inner NodeBlocks.
// The SEGM frame payload contains header + index + RECF sub-frames + trailer,
// matching the block format contract.
func encodeSegmentBlock(h SegmentHeader, nodeBlocks [][]byte) ([]byte, error) {
	header, err := marshalSegmentHeader(h)
	if err != nil {
		return nil, err
	}
	index := marshalOffIndex(nodeOffsets(nodeBlocks))
	payload := append([]byte(nil), header...)
	payload = append(payload, index...)
	for _, nb := range nodeBlocks {
		payload = append(payload, appendFrame(nil, recFrameMagic(), byte('R'), 0, nb)...)
	}
	payload = append(payload, appendFrame(nil, trailerMagic(), byte('T'), 0, nil)...)
	inner := appendFrame(nil, frameKinds[KindSegment], kindByte(KindSegment), 0, payload)
	return wrapBlock(KindSegment, h.SegmentID, inner), nil
}

// decodeSegmentBlock parses a SegmentBlock, returning its header, the inner
// NodeBlock bytes, and the offsets locating each node.
func decodeSegmentBlock(data []byte) (SegmentHeader, [][]byte, []OffEntry, error) {
	body, err := unwrapBlock(data)
	if err != nil {
		return SegmentHeader{}, nil, nil, err
	}
	kind, _, payloadLen, payloadStart, _, ok := scanFrame(body, 0)
	if !ok || kind != kindByte(KindSegment) {
		return SegmentHeader{}, nil, nil, fmt.Errorf("decodeSegmentBlock: bad segment frame")
	}
	payload := body[payloadStart : payloadStart+int(payloadLen)]
	hdr, rest, err := unmarshalSegmentHeader(payload)
	if err != nil {
		return SegmentHeader{}, nil, nil, err
	}
	entries, afterIndex, err := unmarshalOffIndex(rest)
	if err != nil {
		return SegmentHeader{}, nil, nil, err
	}
	nodeBlocks, _, err := extractRecordFrames(afterIndex)
	if err != nil {
		return SegmentHeader{}, nil, nil, err
	}
	if len(nodeBlocks) != len(entries) {
		return SegmentHeader{}, nil, nil, fmt.Errorf("decodeSegmentBlock: index count %d != node count %d", len(entries), len(nodeBlocks))
	}
	return hdr, nodeBlocks, entries, nil
}

// nodeOffsets builds the offset/length index for a list of inner NodeBlocks.
// Offsets are relative to the RECF frame payload (the inner node bytes).
func nodeOffsets(nodeBlocks [][]byte) []OffEntry {
	entries := make([]OffEntry, 0, len(nodeBlocks))
	var offset uint64
	for _, nb := range nodeBlocks {
		entries = append(entries, OffEntry{
			Seq:    0,
			Offset: offset,
			Length: uint64(len(nb)),
			SHA256: HashBytes(nb),
		})
		offset += uint64(len(nb))
	}
	return entries
}

// marshalOffIndex encodes OffEntry list: count uvarint, then per entry
// (seq uvarint, offset uvarint, length uvarint, sha256 LPSTR).
func marshalOffIndex(entries []OffEntry) []byte {
	var buf []byte
	buf = writeUvarint(buf, uint64(len(entries)))
	for _, e := range entries {
		buf = writeUvarint(buf, e.Seq)
		buf = writeUvarint(buf, e.Offset)
		buf = writeUvarint(buf, e.Length)
		buf = writeLPSTR(buf, e.SHA256)
	}
	return buf
}

// unmarshalOffIndex parses OffEntry list from data.
func unmarshalOffIndex(data []byte) ([]OffEntry, []byte, error) {
	var off int
	count, err := readUvarint(data, &off)
	if err != nil {
		return nil, nil, err
	}
	entries := make([]OffEntry, 0, count)
	for i := uint64(0); i < count; i++ {
		var e OffEntry
		if e.Seq, err = readUvarint(data, &off); err != nil {
			return nil, nil, err
		}
		if e.Offset, err = readUvarint(data, &off); err != nil {
			return nil, nil, err
		}
		if e.Length, err = readUvarint(data, &off); err != nil {
			return nil, nil, err
		}
		if e.SHA256, err = readLPSTR(data, &off); err != nil {
			return nil, nil, err
		}
		entries = append(entries, e)
	}
	return entries, data[off:], nil
}

// extractRecordFrames parses a sequence of RECF frames from data, returning
// each frame's payload and the remaining bytes after the trailer.
func extractRecordFrames(data []byte) ([][]byte, []byte, error) {
	var records [][]byte
	off := 0
	for off < len(data) {
		kind, _, payloadLen, payloadStart, total, ok := scanFrame(data, off)
		if !ok {
			return nil, nil, fmt.Errorf("extractRecordFrames: bad frame at %d", off)
		}
		if kind == byte('T') {
			return records, data[off:], nil
		}
		if kind != byte('R') {
			return nil, nil, fmt.Errorf("extractRecordFrames: unexpected frame kind %d", kind)
		}
		records = append(records, data[payloadStart:payloadStart+int(payloadLen)])
		off += total
	}
	return records, data[off:], nil
}

// recFrameMagic returns the RECF sub-record frame magic.
func recFrameMagic() [4]byte {
	return [4]byte{frameMagicHead1, frameMagicHead2, 'R', 'C'}
}

var _ = bytes.Compare
