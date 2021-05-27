package multiplexer

type respContainer struct {
	message []byte
	err     error
}

// oneshotChannel make sender/receiver, inspired from Rust
func oneshotChannel() (chan<- *respContainer, <-chan *respContainer) {
	ch := make(chan *respContainer)
	return ch, ch
}

type reqContainer struct {
	sender  chan<- *respContainer
	message []byte
}

// mpscChannel make sender/receiver
func mpscChannel(size int) (chan<- *reqContainer, <-chan *reqContainer, chan *reqContainer) {
	ch := make(chan *reqContainer, size)
	return ch, ch, ch
}
