package timeline

// EncodeNodeBlockForTest is the exported wrapper around the NodeBlock encoder,
// used by integration tools and tests outside the timeline package.
func EncodeNodeBlockForTest(h NodeBlockHeader, ops []DeltaOp, refs []ObjectRef) ([]byte, error) {
	return encodeNodeBlockForTest(h, ops, refs)
}
