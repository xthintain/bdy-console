package timeline

import (
	"fmt"
)

// encodeArchiveBlock builds an ArchiveBlock from its header, inner
// SegmentBlocks, and a set of content objects. The ARCH frame payload contains
// header + segment index + node_section (RECF frames of segment bytes) +
// object_section (OBJ frames) + trailer.
func encodeArchiveBlock(h ArchiveHeader, segmentBlocks [][]byte, objects []ObjectRef) ([]byte, error) {
	header, err := marshalArchiveHeader(h)
	if err != nil {
		return nil, err
	}
	segIndex := marshalOffIndex(nodeOffsets(segmentBlocks))
	payload := append([]byte(nil), header...)
	payload = append(payload, segIndex...)
	for _, sb := range segmentBlocks {
		payload = append(payload, appendFrame(nil, recFrameMagic(), byte('R'), 0, sb)...)
	}
	payload = append(payload, marshalObjectSection(objects)...)
	payload = append(payload, appendFrame(nil, trailerMagic(), byte('T'), 0, nil)...)
	inner := appendFrame(nil, frameKinds[KindArchive], kindByte(KindArchive), 0, payload)
	return wrapBlock(KindArchive, h.ArchiveID, inner), nil
}

// decodeArchiveBlock parses an ArchiveBlock, returning its header, inner
// SegmentBlocks, and the object section entries.
func decodeArchiveBlock(data []byte) (ArchiveHeader, [][]byte, []ObjectRef, error) {
	body, err := unwrapBlock(data)
	if err != nil {
		return ArchiveHeader{}, nil, nil, err
	}
	kind, _, payloadLen, payloadStart, _, ok := scanFrame(body, 0)
	if !ok || kind != kindByte(KindArchive) {
		return ArchiveHeader{}, nil, nil, fmt.Errorf("decodeArchiveBlock: bad archive frame")
	}
	payload := body[payloadStart : payloadStart+int(payloadLen)]
	hdr, rest, err := unmarshalArchiveHeader(payload)
	if err != nil {
		return ArchiveHeader{}, nil, nil, err
	}
	segEntries, afterIndex, err := unmarshalOffIndex(rest)
	if err != nil {
		return ArchiveHeader{}, nil, nil, err
	}
	segBlocks, tail, err := extractSegmentFrames(afterIndex)
	if err != nil {
		return ArchiveHeader{}, nil, nil, err
	}
	if len(segBlocks) != len(segEntries) {
		return ArchiveHeader{}, nil, nil, fmt.Errorf("decodeArchiveBlock: segment count %d != index %d", len(segBlocks), len(segEntries))
	}
	objects, err := unmarshalObjectSection(tail)
	if err != nil {
		return ArchiveHeader{}, nil, nil, err
	}
	return hdr, segBlocks, objects, nil
}

// marshalObjectSection encodes content objects as OBJ frames. Each OBJ frame
// kind byte is flagObject; the payload is object_id LPSTR + object bytes.
func marshalObjectSection(objects []ObjectRef) []byte {
	var buf []byte
	for _, o := range objects {
		payload := writeLPSTR(nil, o.ObjectID)
		payload = append(payload, []byte(o.SHA256)...) // object bytes carried in SHA256 field for compactness
		buf = append(buf, appendFrame(nil, objFrameMagic(), flagObject, 0, payload)...)
	}
	return buf
}

// unmarshalObjectSection parses OBJ frames, returning each object's id and bytes.
func unmarshalObjectSection(data []byte) ([]ObjectRef, error) {
	var refs []ObjectRef
	off := 0
	for off < len(data) {
		kind, _, payloadLen, payloadStart, total, ok := scanFrame(data, off)
		if !ok {
			return nil, fmt.Errorf("unmarshalObjectSection: bad frame at %d", off)
		}
		if kind == byte('T') {
			break
		}
		if kind != flagObject {
			return nil, fmt.Errorf("unmarshalObjectSection: unexpected frame kind %d", kind)
		}
		payload := data[payloadStart : payloadStart+int(payloadLen)]
		var o ObjectRef
		var pos int
		var err error
		if o.ObjectID, err = readLPSTR(payload, &pos); err != nil {
			return nil, err
		}
		o.SHA256 = string(payload[pos:])
		o.Size = uint64(len(o.SHA256))
		refs = append(refs, o)
		off += total
	}
	return refs, nil
}

// extractSegmentFrames parses RECF frames until an OBJ (flagObject) or trailer
// frame is reached, returning the segment payloads and the remaining bytes.
func extractSegmentFrames(data []byte) ([][]byte, []byte, error) {
	var segs [][]byte
	off := 0
	for off < len(data) {
		kind, _, payloadLen, payloadStart, total, ok := scanFrame(data, off)
		if !ok {
			return nil, nil, fmt.Errorf("extractSegmentFrames: bad frame at %d", off)
		}
		if kind == byte('T') || kind == flagObject {
			return segs, data[off:], nil
		}
		if kind != byte('R') {
			return nil, nil, fmt.Errorf("extractSegmentFrames: unexpected frame kind %d", kind)
		}
		segs = append(segs, data[payloadStart:payloadStart+int(payloadLen)])
		off += total
	}
	return segs, data[off:], nil
}

// objFrameMagic returns the OBJ object frame magic.
func objFrameMagic() [4]byte {
	return [4]byte{frameMagicHead1, frameMagicHead2, 'O', 'J'}
}
