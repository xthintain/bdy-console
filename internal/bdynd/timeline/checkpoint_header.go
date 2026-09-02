package timeline

import "fmt"

// marshalCheckpointHeader serializes a CheckpointBlock header. The tree length
// is stored explicitly so the parser can locate the tree bytes without
// scanning for the first OBJ frame.
func marshalCheckpointHeader(h CheckpointHeader) ([]byte, error) {
	var buf []byte
	buf = writeLPSTR(buf, h.CheckpointID)
	buf = writeLPSTR(buf, h.ProjectID)
	buf = writeUvarint(buf, h.BaseSeq)
	buf = writeLPSTR(buf, h.TreeRoot)
	buf = writeLPSTR(buf, h.ObjectIndex)
	buf = writeUvarint(buf, h.FileCount)
	buf = writeUvarint(buf, h.TotalBytes)
	buf = writeUvarint(buf, h.TreeLen)
	buf = append(buf, h.Compression)
	return buf, nil
}

// unmarshalCheckpointHeader parses a CheckpointBlock header from data.
func unmarshalCheckpointHeader(data []byte) (CheckpointHeader, []byte, error) {
	var h CheckpointHeader
	var off int
	var err error
	if h.CheckpointID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.ProjectID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.BaseSeq, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.TreeRoot, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.ObjectIndex, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.FileCount, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.TotalBytes, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.TreeLen, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if off >= len(data) {
		return h, nil, fmt.Errorf("unmarshalCheckpointHeader: missing compression byte")
	}
	h.Compression = data[off]
	off++
	return h, data[off:], nil
}
