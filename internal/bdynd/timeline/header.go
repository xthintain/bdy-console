package timeline

import "fmt"

// marshalNodeHeader serializes a NodeBlock header into LPSTR/uvarint fields.
func marshalNodeHeader(h NodeBlockHeader) ([]byte, error) {
	var buf []byte
	buf = writeLPSTR(buf, h.NodeID)
	buf = writeLPSTR(buf, h.ParentNodeID)
	buf = writeLPSTR(buf, h.ProjectID)
	buf = writeLPSTR(buf, h.Author)
	buf = writeUvarint(buf, h.TimestampMs)
	buf = writeUvarint(buf, h.Seq)
	buf = append(buf, h.Compression)
	return buf, nil
}

// unmarshalNodeHeader parses a NodeBlock header from data.
func unmarshalNodeHeader(data []byte) (NodeBlockHeader, []byte, error) {
	var h NodeBlockHeader
	var off int
	var err error
	if h.NodeID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.ParentNodeID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.ProjectID, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.Author, err = readLPSTR(data, &off); err != nil {
		return h, nil, err
	}
	if h.TimestampMs, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if h.Seq, err = readUvarint(data, &off); err != nil {
		return h, nil, err
	}
	if off >= len(data) {
		return h, nil, fmt.Errorf("unmarshalNodeHeader: missing compression byte")
	}
	h.Compression = data[off]
	off++
	return h, data[off:], nil
}
