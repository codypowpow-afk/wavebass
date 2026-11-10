package synth

import (
	"bufio"
	"encoding/binary"
	"os"
)

// recorder streams the synth's output to a 16-bit stereo WAV file. The audio thread only
// appends to a buffer; a goroutine does the disk writes.
type recorder struct {
	path   string
	ch     chan []int16
	done   chan struct{}
	cur    []int16
	frames int64
}

func wavHeader(dataBytes uint32) []byte {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+dataBytes)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], SampleRate)
	binary.LittleEndian.PutUint32(h[28:], SampleRate*4)
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], dataBytes)
	return h
}

func (r *recorder) add(l, rr []float64) {
	for i := range l {
		r.cur = append(r.cur, int16(l[i]*32767), int16(rr[i]*32767))
	}
	r.frames += int64(len(l))
	if len(r.cur) >= 16384 {
		select {
		case r.ch <- r.cur:
			r.cur = make([]int16, 0, 16384+64)
		default: // disk can't keep up: keep buffering in memory
		}
	}
}

// StartRecording begins writing the output to a WAV file.
func (e *Engine) StartRecording(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := f.Write(wavHeader(0)); err != nil {
		f.Close()
		return err
	}
	r := &recorder{path: path, ch: make(chan []int16, 64), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		w := bufio.NewWriterSize(f, 1<<16)
		var n uint32
		b := make([]byte, 2)
		for chunk := range r.ch {
			for _, s := range chunk {
				binary.LittleEndian.PutUint16(b, uint16(s))
				w.Write(b)
			}
			n += uint32(len(chunk)) * 2
		}
		w.Flush()
		f.Seek(0, 0)
		f.Write(wavHeader(n))
		f.Close()
	}()
	e.mu.Lock()
	old := e.rec
	e.rec = r
	e.mu.Unlock()
	if old != nil {
		old.finish()
	}
	return nil
}

func (r *recorder) finish() {
	if len(r.cur) > 0 {
		r.ch <- r.cur
		r.cur = nil
	}
	close(r.ch)
	<-r.done
}

// StopRecording finishes the WAV file and returns its path and length in seconds.
func (e *Engine) StopRecording() (string, float64) {
	e.mu.Lock()
	r := e.rec
	e.rec = nil
	e.mu.Unlock()
	if r == nil {
		return "", 0
	}
	r.finish()
	return r.path, float64(r.frames) / SampleRate
}
