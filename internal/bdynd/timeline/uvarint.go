package timeline

import "encoding/binary"

// readUvarint decodes an LEB128 unsigned integer from data at offset *off,
// advancing the offset past the consumed bytes.
func readUvarint(data []byte, off *int) (uint64, error) {
	// binary.Uvarint requires at least one byte; pass a slice from off.
	slice := data[*off:]
	value, n := binary.Uvarint(slice)
	if n <= 0 {
		return 0, errUvarint
	}
	*off += n
	return value, nil
}

var errUvarint = &uvarintError{}

type uvarintError struct{}

func (uvarintError) Error() string { return "timeline: invalid uvarint" }
