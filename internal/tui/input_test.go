package tui

import "testing"

func TestParseEscapeFocusEvents(t *testing.T) {
	tests := []struct {
		name string
		seq  []byte
		key  Key
	}{
		{"CSI focus-in", []byte{0x1b, '[', 'I'}, KeyFocusIn},
		{"CSI focus-out", []byte{0x1b, '[', 'O'}, KeyFocusOut},
		{"SS3 focus-in", []byte{0x1b, 'I'}, KeyFocusIn},
		{"SS3 focus-out", []byte{0x1b, 'O'}, KeyFocusOut},
		{"arrow up", []byte{0x1b, '[', 'A'}, KeyUp},
		{"arrow down", []byte{0x1b, '[', 'B'}, KeyDown},
		{"page up", []byte{0x1b, '[', '5', '~'}, KeyPageUp},
		{"page down", []byte{0x1b, '[', '6', '~'}, KeyPageDown},
		{"esc alone", []byte{0x1b}, KeyEsc},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, ok := parseEscape(tt.seq)
			if !ok {
				t.Fatalf("parseEscape(%v) should succeed", tt.seq)
			}
			if ev.Key != tt.key {
				t.Errorf("parseEscape(%v) = key %d, want %d", tt.seq, ev.Key, tt.key)
			}
		})
	}
}

func TestParseEscapeUnknown(t *testing.T) {
	ev, ok := parseEscape([]byte{0x1b, '[', 'Z'})
	if !ok {
		t.Fatal("unknown CSI should still return a valid event")
	}
	if ev.Key != KeyUnknown {
		t.Errorf("unknown CSI should map to KeyUnknown, got %d", ev.Key)
	}
}
