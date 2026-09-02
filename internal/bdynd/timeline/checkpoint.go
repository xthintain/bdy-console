package timeline

import (
	"fmt"
)

// encodeCheckpointBlock builds a CheckpointBlock containing a full tree
// snapshot and the objects referenced by it. The tree length is recorded in
// the header so the parser can slice the tree bytes precisely.
func encodeCheckpointBlock(h CheckpointHeader, fullTree []byte, objects []ObjectRef) ([]byte, error) {
	h.TreeLen = uint64(len(fullTree))
	header, err := marshalCheckpointHeader(h)
	if err != nil {
		return nil, err
	}
	payload := append([]byte(nil), header...)
	payload = append(payload, fullTree...)
	payload = append(payload, marshalObjectSection(objects)...)
	payload = append(payload, appendFrame(nil, trailerMagic(), byte('T'), 0, nil)...)
	inner := appendFrame(nil, frameKinds[KindCheckpoint], kindByte(KindCheckpoint), flagFullSnapshot, payload)
	return wrapBlock(KindCheckpoint, h.CheckpointID, inner), nil
}

// decodeCheckpointBlock parses a CheckpointBlock, returning its header, full
// tree bytes, and the object section entries.
func decodeCheckpointBlock(data []byte) (CheckpointHeader, []byte, []ObjectRef, error) {
	body, err := unwrapBlock(data)
	if err != nil {
		return CheckpointHeader{}, nil, nil, err
	}
	kind, _, payloadLen, payloadStart, _, ok := scanFrame(body, 0)
	if !ok || kind != kindByte(KindCheckpoint) {
		return CheckpointHeader{}, nil, nil, fmt.Errorf("decodeCheckpointBlock: bad checkpoint frame")
	}
	payload := body[payloadStart : payloadStart+int(payloadLen)]
	hdr, rest, err := unmarshalCheckpointHeader(payload)
	if err != nil {
		return CheckpointHeader{}, nil, nil, err
	}
	if uint64(len(rest)) < hdr.TreeLen {
		return CheckpointHeader{}, nil, nil, fmt.Errorf("decodeCheckpointBlock: tree truncated")
	}
	tree := rest[:hdr.TreeLen]
	objects, err := unmarshalObjectSection(rest[hdr.TreeLen:])
	if err != nil {
		return CheckpointHeader{}, nil, nil, err
	}
	return hdr, tree, objects, nil
}
