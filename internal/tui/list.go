package tui

// cursor tracks a selected index and a scroll offset in a list.
type cursor struct {
	pos, offset int
}

// move shifts the selection by delta within n items.
func (c *cursor) move(delta, n int) {
	c.pos += delta
	c.clamp(n)
}

// clamp keeps the selection inside n items.
func (c *cursor) clamp(n int) {
	if c.pos >= n {
		c.pos = n - 1
	}
	if c.pos < 0 {
		c.pos = 0
	}
}

// window returns the [start, end) slice of n items visible in height rows,
// scrolling so the selection stays visible.
func (c *cursor) window(n, height int) (start, end int) {
	c.clamp(n)
	if height <= 0 || n == 0 {
		return 0, 0
	}
	if c.pos < c.offset {
		c.offset = c.pos
	}
	if c.pos >= c.offset+height {
		c.offset = c.pos - height + 1
	}
	c.offset = max(0, min(c.offset, n-height))
	return c.offset, min(n, c.offset+height)
}

// navKey applies a movement key and reports whether it was one.
func (c *cursor) navKey(k string, n, page int) bool {
	switch k {
	case "j", "down":
		c.move(1, n)
	case "k", "up":
		c.move(-1, n)
	case "g", "home":
		c.pos = 0
	case "G", "end":
		c.pos = n - 1
		c.clamp(n)
	case "pgdown", "ctrl+f":
		c.move(max(1, page), n)
	case "pgup", "ctrl+b":
		c.move(-max(1, page), n)
	default:
		return false
	}
	return true
}
