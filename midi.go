package main

// midiInput is a platform MIDI input backend.
type midiInput interface {
	Rescan()
	Devices() (open []string, busy []string)
	Close()
}
