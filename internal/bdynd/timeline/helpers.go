package timeline

import (
	"bytes"
	"fmt"
	"strings"
)

// kindByte returns the 1-byte kind enum used inside frames.
func kindByte(k Kind) byte {
	switch k {
	case KindNode:
		return 'N'
	case KindSegment:
		return 'S'
	case KindArchive:
		return 'A'
	case KindCheckpoint:
		return 'C'
	}
	return 0
}

// kindName returns the uppercase name used in BEGIN/END tags.
func kindName(k Kind) string {
	switch k {
	case KindNode:
		return "NODE"
	case KindSegment:
		return "SEGMENT"
	case KindArchive:
		return "ARCHIVE"
	case KindCheckpoint:
		return "CHECKPOINT"
	}
	return "UNKNOWN"
}

// trailerMagic returns the trailing frame magic.
func trailerMagic() [4]byte {
	return [4]byte{frameMagicHead1, frameMagicHead2, 'E', 'O'}
}

// wrapBlock wraps framed bytes with BEGIN/END text tags and a whole-block SHA-256.
func wrapBlock(kind Kind, id string, inner []byte) []byte {
	begin := fmt.Sprintf("---- BDYDB-%s-BEGIN 1/0 %s ----\n", kindName(kind), id)
	endTag := fmt.Sprintf("---- BDYDB-%s-END %s %s ----\n", kindName(kind), id, HashBytes(inner))
	out := make([]byte, 0, len(begin)+len(inner)+len(endTag))
	out = append(out, begin...)
	out = append(out, inner...)
	out = append(out, endTag...)
	return out
}

// unwrapBlock returns the inner framed bytes after validating the whole-block
// SHA-256 recorded in the END tag. Any corruption is rejected.
func unwrapBlock(data []byte) ([]byte, error) {
	inner, expectedHash, err := splitBlock(data)
	if err != nil {
		return nil, err
	}
	if HashBytes(inner) != expectedHash {
		return nil, fmt.Errorf("unwrapBlock: whole-block sha256 mismatch")
	}
	return inner, nil
}

// splitBlock separates a wrapped block into its inner framed bytes and the
// expected SHA-256 recorded in the END tag.
func splitBlock(data []byte) (inner []byte, endSHA256 string, err error) {
	beginEnd := bytes.IndexByte(data, '\n')
	if beginEnd < 0 {
		return nil, "", fmt.Errorf("splitBlock: missing BEGIN newline")
	}
	// The END tag starts with "---- BDYDB-<NAME>-END " and follows the inner
	// frame directly. Find the LAST occurrence so inner binary data cannot
	// shadow it.
	endMarker := []byte("---- BDYDB-")
	endIdx := bytes.LastIndex(data, endMarker)
	if endIdx <= beginEnd {
		return nil, "", fmt.Errorf("splitBlock: missing END line")
	}
	inner = data[beginEnd+1 : endIdx]
	endLine := string(data[endIdx:])
	// END line: "---- BDYDB-NODE-END <id> <sha256hex> ----\n"
	fields := strings.Fields(endLine)
	if len(fields) < 5 {
		return nil, "", fmt.Errorf("splitBlock: malformed END line %q", endLine)
	}
	endSHA256 = fields[len(fields)-2]
	return inner, endSHA256, nil
}

// marshalDeltaOps encodes delta operations: count uvarint, then per op
// (opcode byte, path LPSTR, object_id LPSTR).
func marshalDeltaOps(ops []DeltaOp) []byte {
	var buf []byte
	buf = writeUvarint(buf, uint64(len(ops)))
	for _, op := range ops {
		buf = append(buf, op.Op)
		buf = writeLPSTR(buf, op.Path)
		buf = writeLPSTR(buf, op.ObjectID)
	}
	return buf
}

// marshalObjectRefs encodes object refs: count uvarint, then per ref
// (object_id LPSTR, size uvarint, sha256 LPSTR).
func marshalObjectRefs(refs []ObjectRef) []byte {
	var buf []byte
	buf = writeUvarint(buf, uint64(len(refs)))
	for _, r := range refs {
		buf = writeLPSTR(buf, r.ObjectID)
		buf = writeUvarint(buf, r.Size)
		buf = writeLPSTR(buf, r.SHA256)
	}
	return buf
}
