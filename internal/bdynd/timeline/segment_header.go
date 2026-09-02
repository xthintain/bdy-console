package timeline

import "fmt"

// marshalSegmentHeader serializes a SegmentBlock header.
func marshalSegmentHeader(h SegmentHeader) ([]byte, error) {
	var buf []byte
	buf = writeLPSTR(buf, h.SegmentID)
	buf = writeLPSTR(buf, h.ArchiveID)
	buf = writeLPSTR(buf, h.ProjectID)
	buf = writeUvarint(buf, h.BeginSeq)
	buf = writeUvarint(buf, h.EndSeq)
	buf = writeUvarint(buf, h.NodeCount)
	buf = append(buf, h.Compression)
	return buf, nil
}

// unmarshalSegmentHeader parses a SegmentBlock header from data.
func unmarshalSegmentHeader(data []byte) (SegmentHeader, []byte, error) {
	var h SegmentHeader
	var off int
	var err error
	if h.SegmentID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.ArchiveID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.ProjectID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.BeginSeq, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.EndSeq, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.NodeCount, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if off >= len(data) {
		return h, nil, fmt.Errorf("unmarshalSegmentHeader: missing compression byte")
	}
	h.Compression = data[off]
	off++
	return h, data[off:], nil
}
