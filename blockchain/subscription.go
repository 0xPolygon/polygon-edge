package blockchain

type Event struct {
}

type Subscription struct {
}

func (s *Subscription) Close() {

}

func (s *Subscription) Watch() chan Event {
	return nil
}
