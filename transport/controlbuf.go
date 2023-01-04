package transport

type dataFrame struct {
	streamID  uint32
	endStream bool
	h         []byte
	d         []byte
	onEachWrite func()
}