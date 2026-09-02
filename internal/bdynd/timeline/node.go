package timeline

import (
	"bytes"
	"fmt"
)

// decodeNodeBlock parses a NodeBlock from its wrapped byte form.
func decodeNodeBlock(data []byte) (NodeBlockHeader, []DeltaOp, []ObjectRef, error) {
	body, err := unwrapBlock(data)
	if err != nil {
		return NodeBlockHeader{}, nil, nil, err
	}
	kind, _, payloadLen, payloadStart, total, ok := scanFrame(body, 0)
	if !ok || kind != kindByte(KindNode) {
		return NodeBlockHeader{}, nil, nil, fmt.Errorf("decodeNodeBlock: bad node frame")
	}
	payload := body[payloadStart : payloadStart+int(payloadLen)]
	_ = total
	hdr, rest, err := unmarshalNodeHeader(payload)
	if err != nil {
		return NodeBlockHeader{}, nil, nil, err
	}
	ops, after, err := unmarshalDeltaOps(rest)
	if err != nil {
		return NodeBlockHeader{}, nil, nil, err
	}
	refs, err := unmarshalObjectRefs(after)
	if err != nil {
		return NodeBlockHeader{}, nil, nil, err
	}
	return hdr, ops, refs, nil
}

// unmarshalDeltaOps parses delta operations from data.
func unmarshalDeltaOps(data []byte) ([]DeltaOp, []byte, error) {
	var off int
	count, err := readUvarint(data, &off)
	if err != nil {
		return nil, nil, err
	}
	ops := make([]DeltaOp, 0, count)
	for i := uint64(0); i < count; i++ {
		if off >= len(data) {
			return nil, nil, fmt.Errorf("unmarshalDeltaOps: opcode out of range")
		}
		var op DeltaOp
		op.Op = data[off]
		off++
		if op.Path, err = readLPSTR(data, &off); err != nil {
			return nil, nil, err
		}
		if op.ObjectID, err = readLPSTR(data, &off); err != nil {
			return nil, nil, err
		}
		ops = append(ops, op)
	}
	return ops, data[off:], nil
}

// unmarshalObjectRefs parses object references from data.
func unmarshalObjectRefs(data []byte) ([]ObjectRef, error) {
	var off int
	count, err := readUvarint(data, &off)
	if err != nil {
		return nil, err
	}
	refs := make([]ObjectRef, 0, count)
	for i := uint64(0); i < count; i++ {
		var r ObjectRef
		if r.ObjectID, err = readLPSTR(data, &off); err != nil {
			return nil, err
		}
		if r.Size, err = readUvarint(data, &off); err != nil {
			return nil, err
		}
		if r.SHA256, err = readLPSTR(data, &off); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	return refs, nil
}

// encodeNodeBlockForTest is used by tests to build a wrapped NodeBlock.
func encodeNodeBlockForTest(h NodeBlockHeader, ops []DeltaOp, refs []ObjectRef) ([]byte, error) {
	header, err := marshalNodeHeader(h)
	if err != nil {
		return nil, err
	}
	payload := marshalDeltaOps(ops)
	payload = append(payload, marshalObjectRefs(refs)...)
	inner := appendFrame(nil, frameKinds[KindNode], kindByte(KindNode), 0, append(header, payload...))
	inner = append(inner, appendFrame(nil, trailerMagic(), byte('T'), 0, nil)...)
	return wrapBlock(KindNode, h.NodeID, inner), nil
}

var _ = bytes.Compare
