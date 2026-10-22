package control

// TurnInFlight reports whether a turn is executing. It ends when the turn does,
// before its TurnDone has finished reaching subscribers, so a client that asks
// the moment it sees that event is told the turn is over. Running is the wider
// answer that admission guards read.
func (c *Controller) TurnInFlight() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gate.running
}
