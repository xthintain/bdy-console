package timeline

import "fmt"

// marshalArchiveHeader serializes an ArchiveBlock header.
func marshalArchiveHeader(h ArchiveHeader) ([]byte, error) {
	var buf []byte
	buf = writeLPSTR(buf, h.ArchiveID)
	buf = writeLPSTR(buf, h.ProjectID)
	buf = writeLPSTR(buf, h.PrevArchiveID)
	buf = writeUvarint(buf, h.BeginSeq)
	buf = writeUvarint(buf, h.EndSeq)
	buf = writeUvarint(buf, h.SegmentCount)
	buf = writeUvarint(buf, h.NodeCount)
	buf = append(buf, h.Compression)
	return buf, nil
}

// unmarshalArchiveHeader parses an ArchiveBlock header from data.
func unmarshalArchiveHeader(data []byte) (ArchiveHeader, []byte, error) {
	var h ArchiveHeader
	var off int
	var err error
	if h.ArchiveID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.ProjectID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.PrevArchiveID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.BeginSeq, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.EndSeq, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.SegmentCount, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.NodeCount, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if off >= len(data) {
		return h, nil, fmt.Errorf("unmarshalArchiveHeader: missing compression byte")
	}
	h.Compression = data[off]
	off++
	return h, data[off:], nil
}
