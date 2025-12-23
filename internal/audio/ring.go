package audio

import "sync"

// Ring buffer for PCM16 samples (int16). Stores last N samples.
type Ring struct {
	mu   sync.Mutex
	buf  []int16
	pos  int
	full bool
}

func NewRing(size int) *Ring {
	return &Ring{buf: make([]int16, size)}
}

func (r *Ring) Write(samples []int16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range samples {
		r.buf[r.pos] = s
		r.pos++
		if r.pos >= len(r.buf) {
			r.pos = 0
			r.full = true
		}
	}
}

func (r *Ring) ReadLast(n int) []int16 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if n > len(r.buf) {
		n = len(r.buf)
	}

	// Determine how many samples are actually available
	available := len(r.buf)
	if !r.full {
		available = r.pos
	}
	if n > available {
		n = available
	}

	if n == 0 {
		return []int16{}
	}

	out := make([]int16, n)

	// Calculate start position for reading last n samples
	start := r.pos - n
	if start < 0 {
		start += len(r.buf)
	}

	// Copy samples in correct order
	if start+n <= len(r.buf) {
		// Simple case: no wrap-around
		copy(out, r.buf[start:start+n])
	} else {
		// Wrap-around case: copy from start to end, then from beginning
		firstPart := len(r.buf) - start
		copy(out, r.buf[start:])
		copy(out[firstPart:], r.buf[:n-firstPart])
	}

	return out
}

// Clear resets the ring buffer, discarding all stored audio
func (r *Ring) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pos = 0
	r.full = false
}
