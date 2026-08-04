package tui

import (
	"bufio"
	"os"
	"strings"
)

type Key int

const (
	KeyUnknown Key = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyPageUp
	KeyPageDown
	KeyHome
	KeyEnd
	KeyEnter
	KeyEsc
	KeyTab
	KeyBackspace
	KeyDelete
	KeyRune
	KeyCtrlC
	KeyCtrlD
	KeyFocusIn
	KeyFocusOut
	KeyResize
)

type InputEvent struct {
	Key  Key
	Rune rune
}

type Input struct {
	reader *bufio.Reader
	file   *os.File
	ch     chan InputEvent
}

func NewInput() *Input {
	return &Input{
		reader: bufio.NewReader(os.Stdin),
		file:   os.Stdin,
		ch:     make(chan InputEvent, 64),
	}
}

func (i *Input) Channel() <-chan InputEvent {
	return i.ch
}

func (i *Input) Start() {
	go func() {
		for {
			ev, ok := i.readEvent()
			if !ok {
				close(i.ch)
				return
			}
			i.ch <- ev
		}
	}()
}

func (i *Input) readEvent() (InputEvent, bool) {
	b, err := i.reader.ReadByte()
	if err != nil {
		return InputEvent{}, false
	}

	if b == 0x1b {
		seq := i.readEscape()
		return parseEscape(seq)
	}

	switch b {
	case '\r', '\n':
		return InputEvent{Key: KeyEnter}, true
	case 0x03:
		return InputEvent{Key: KeyCtrlC}, true
	case 0x04:
		return InputEvent{Key: KeyCtrlD}, true
	case 0x7f, 0x08:
		return InputEvent{Key: KeyBackspace}, true
	case 0x09:
		return InputEvent{Key: KeyTab}, true
	default:
		return InputEvent{Key: KeyRune, Rune: rune(b)}, true
	}
}

func (i *Input) readEscape() []byte {
	buf := []byte{0x1b}
	for len(buf) < 16 {
		b, err := i.reader.ReadByte()
		if err != nil {
			break
		}
		buf = append(buf, b)
		if b == '~' || b == 'R' {
			break
		}
		if len(buf) >= 2 && buf[1] == '[' && b >= 0x40 && b <= 0x7e && b != '~' {
			if b == 'I' || b == 'O' {
				break
			}
			if b >= 'A' && b <= 'D' {
				break
			}
			if b == 'H' || b == 'F' {
				break
			}
			if b == 'R' {
				break
			}
		}
	}
	return buf
}

func parseEscape(seq []byte) (InputEvent, bool) {
	if len(seq) == 1 {
		return InputEvent{Key: KeyEsc}, true
	}

	if len(seq) == 2 && seq[1] == 'O' {
		return InputEvent{Key: KeyFocusOut}, true
	}
	if len(seq) == 2 && seq[1] == 'I' {
		return InputEvent{Key: KeyFocusIn}, true
	}

	if len(seq) >= 3 && seq[1] == '[' {
		switch seq[2] {
		case 'A':
			return InputEvent{Key: KeyUp}, true
		case 'B':
			return InputEvent{Key: KeyDown}, true
		case 'C':
			return InputEvent{Key: KeyRight}, true
		case 'D':
			return InputEvent{Key: KeyLeft}, true
		case 'H':
			return InputEvent{Key: KeyHome}, true
		case 'F':
			return InputEvent{Key: KeyEnd}, true
		case 'I':
			return InputEvent{Key: KeyFocusIn}, true
		case 'O':
			return InputEvent{Key: KeyFocusOut}, true
		}

		if seq[2] == '5' && len(seq) >= 4 && seq[3] == '~' {
			return InputEvent{Key: KeyPageUp}, true
		}
		if seq[2] == '6' && len(seq) >= 4 && seq[3] == '~' {
			return InputEvent{Key: KeyPageDown}, true
		}
		if seq[2] == '1' && len(seq) >= 4 && seq[3] == '~' {
			return InputEvent{Key: KeyHome}, true
		}
		if seq[2] == '4' && len(seq) >= 4 && seq[3] == '~' {
			return InputEvent{Key: KeyEnd}, true
		}
		if seq[2] == '3' && len(seq) >= 4 && seq[3] == '~' {
			return InputEvent{Key: KeyDelete}, true
		}

		if strings.Contains(string(seq), ";") {
			return InputEvent{Key: KeyRune, Rune: 0}, true
		}
	}

	return InputEvent{Key: KeyUnknown}, true
}
