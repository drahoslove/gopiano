package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	WAIT             = 15 // how many seconds to wait before leaving realtime mode
	FPS              = 50 // packet sent per second while active
	SUSTAIN_DURATION = 5  // how long sustained note holds its color in seconds
)

// real time protocols
const (
	_ = iota
	WARLS
	DRGB // only supporeted now
	DRGBW
	DNRGB
)

const (
	MODE_NONE = iota
	MODE_RAINBOW_1
	MODE_RAINBOW_2
	MODE_RAINBOW_3
	MODE_RAINBOW_4
	MODE_WHITE_WARM
	MODE_WHITE
	MODE_WHITE_COLD
	MODE_RED
	MODE_RED_YELLOW
	MODE_YELLOW
	MODE_YELLOW_GREEN
	MODE_GREEN
	MODE_GREEN_CYAN
	MODE_CYAN
	MODE_CYAN_BLUE
	MODE_BLUE
	MODE_BLUE_MAGENTA
	MODE_MAGENTA
	MODE_MAGENTA_RED
)
const (
	BKG_BLACK = iota
	BKG_DIMMED
	BKG_LIGHT
)

// key functions
const (
	KEY_CTRL_0        = NOTE_A0 + iota
	KEY_TOGGLE_ACTIVE // on / off
	KEY_CTRL_1
	KEY_DEC_SAT
	KEY_DEC_BRI
	KEY_TOGGLE_BACKGROUND
	KEY_INC_BRI
	KEY_INC_SAT
)

// maps notes (keys) to color modes
var noteToColorMode = map[byte]int{
	NOTE_A0 + 9:  MODE_WHITE_COLD,
	NOTE_A0 + 11: MODE_WHITE,
	NOTE_A0 + 13: MODE_WHITE_WARM,
	NOTE_A0 + 8:  MODE_RAINBOW_1,
	NOTE_A0 + 10: MODE_RAINBOW_2,
	NOTE_A0 + 12: MODE_RAINBOW_3,
	NOTE_A0 + 14: MODE_RAINBOW_4,
	NOTE_A0 + 15: MODE_RED,
	NOTE_A0 + 16: MODE_RED_YELLOW,
	NOTE_A0 + 17: MODE_YELLOW,
	NOTE_A0 + 18: MODE_YELLOW_GREEN,
	NOTE_A0 + 19: MODE_GREEN,
	NOTE_A0 + 20: MODE_GREEN_CYAN,
	NOTE_A0 + 21: MODE_CYAN,
	NOTE_A0 + 22: MODE_CYAN_BLUE,
	NOTE_A0 + 23: MODE_BLUE,
	NOTE_A0 + 24: MODE_BLUE_MAGENTA,
	NOTE_A0 + 25: MODE_MAGENTA,
	NOTE_A0 + 26: MODE_MAGENTA_RED,
}

// colors
type RGB [3]byte

var (
	BLACK      = RGB{0, 0, 0}
	WHITE_COLD = RGB{255, 255, 255} // RGB{226, 232, 255}
	WHITE      = RGB{255, 224, 160} // RGB{255, 255, 255}
	WHITE_WARM = RGB{255, 182, 111} // RGB{255, 224, 160}
	RED        = RGB{255, 0, 0}
	GREEN      = RGB{0, 255, 0}
	BLUE       = RGB{0, 0, 255}
)

var ctrl = [2]bool{false, false} // fisrt 2 white keys for controlls
var (
	doDecSat                     = false
	doIncSat                     = false
	doDecBri                     = false
	doIncBri                     = false
	pressingOffTimer *time.Timer = nil
)

// state
var state = struct {
	active         bool
	brightness     byte
	saturation     byte
	colorMode      int
	backgroundMode int
	sustainMode    bool
}{
	active:         true,
	brightness:     127,
	saturation:     255,
	colorMode:      MODE_WHITE_WARM,
	backgroundMode: BKG_BLACK,
	sustainMode:    true,
}

type Note struct {
	on  bool      // true if note is pressed down
	sus bool      // true if note is sustained
	t   time.Time // time of when it was pressed
}
type Notes map[byte]Note

type Leds struct {
	buffer     []byte // data to be sent (3 bytes per led)
	keys       int    // total keys (notes) to be lit (88)
	ledPerNote int    // number of leds per single key (note) (2)
	firstLed   int    // index of first led on the strip to be lit (1)
	firstNote  int    // midi value of first note to be lit (21)
	sustain    bool   // whehter the sustain is now on or off
	notes      Notes
}

func newLeds(keys int, ledPerNote int, firstLed int, firstNote int) Leds {
	buffer := make([]byte, (firstLed+(keys*ledPerNote))*3)
	return Leds{
		buffer:     buffer,
		keys:       keys,
		ledPerNote: ledPerNote,
		firstLed:   firstLed,
		firstNote:  firstNote,
		sustain:    false,
		notes:      make(Notes, keys),
	}
}

func (leds Leds) set(note byte, rgb RGB) {
	keyIndex := int(note) - leds.firstNote
	ledIndex := leds.firstLed + keyIndex*leds.ledPerNote
	for i := ledIndex * 3; i < (ledIndex+leds.ledPerNote)*3; i++ {
		leds.buffer[i] = rgb[i%3]
	}
}

func (leds Leds) On(note byte, velocity byte) {
	leds.set(note, noteToColor(note, velocity))
	leds.notes[note] = Note{true, false, time.Now()}
}
func (leds Leds) Off(note byte) {
	leds.set(note, noteToBkgColor(note))
	leds.notes[note] = Note{false, leds.sustain, leds.notes[note].t}

}
func (leds *Leds) Sustain(val byte) {
	leds.sustain = val > 0 // track state of the sustain pedal
	if val == 0 {          // silent sustained
		for midi, note := range leds.notes {
			note.sus = false
			leds.notes[midi] = note
		}
	}
}
func (leds *Leds) Render() {
	now := time.Now()
	for midi, note := range leds.notes {
		color := noteToColor(midi, toVal(1))
		bColor := noteToBkgColor(midi)
		duration := now.Sub(note.t)
		t := float64(duration) / float64(time.Second*SUSTAIN_DURATION) // 0..1
		if note.on {
			leds.set(midi, color)
		} else if note.sus && t < 1 {
			leds.set(midi, avgColor(color, bColor, t))
		} else {
			leds.set(midi, bColor)
			delete(leds.notes, midi)
		}
	}
}
func (leds *Leds) Reset() {
	// leds.buffer = make([]byte, len(leds.buffer))
	for i := 0; i < leds.keys; i++ {
		note := byte(i + leds.firstNote)
		leds.set(note, noteToBkgColor(note))
		delete(leds.notes, note)
	}
}

// Returns a channel which consumes midi messages
// and function for turning the wled on/off
func getWled(addr string) (chan []byte, func(bool)) {
	var conn net.Conn
	var err error

	// 88 keys, two leds per key, skip first led, first note is A0
	var leds = newLeds(88, 2, 1, NOTE_A0)
	// use different view to access idnividual leds of the 'on/off controll area'
	var subleds = Leds{leds.buffer, 8, 1, 0, 0, false, leds.notes}

	incommingMidi := make(chan []byte)

	conn, err = net.Dial("udp", addr)
	if err != nil {
		log.Fatal("wled dial failed", err)
	}

	ticker := time.NewTicker(time.Second / FPS)

	sendLeds := func(args ...byte) {
		wait := byte(WAIT)
		if len(args) > 0 {
			wait = args[0]
		}
		if state.sustainMode && len(args) > 1 { // rerender buffer
			leds.Render()
		}

		_, err = conn.Write(append([]byte{DRGB, wait}, leds.buffer...))
		if err != nil {
			// try redial
			conn, err = net.Dial("udp", addr)
			if err != nil {
				log.Println("no udp connection, message dropped", err)
			} else {
				_, err = conn.Write(append([]byte{DRGB, wait}, leds.buffer...))
				if err != nil {
					log.Println("second send try failed", err)
				}
			}
		}
	}

	preview := func(args ...bool) {
		solidColor := false
		if len(args) > 0 {
			solidColor = args[0]
		}
		for i := 0; i < leds.keys; i++ {
			note := byte(i + leds.firstNote)
			clr := BLACK
			if solidColor { // show all as frontend
				clr = dimmedColor(noteToColor(note, 64))
			} else { // show some lit, but most as background
				switch int(note) - (NOTE_A0 + 34) {
				case -2, 0, 2, 4, 6, 8:
					fallthrough
				case 11, 13, 15, 17, 19, 21:
					clr = noteToColor(note, 64)
				default:
					clr = noteToBkgColor(note)
				}
			}
			leds.set(note, clr)
		}
	}

	incSat := func(step byte) {
		if state.saturation <= 255-step {
			state.saturation += step
		}
		preview()
	}

	decSat := func(step byte) {
		if state.saturation >= step+64 {
			state.saturation -= step
		}
		preview()
	}

	incBri := func(step byte) {
		if state.brightness <= 255-step {
			state.brightness += step
		}
		setWledState(addr, "bri", state.brightness)
		preview()
	}

	decBri := func(step byte) {
		if state.brightness >= step+8 {
			state.brightness -= step
		}
		setWledState(addr, "bri", state.brightness)
		preview()
	}

	animateOn := func() {
		done := make(chan bool)
		bkgMode := state.backgroundMode // temporaryly change bkg mode to turn all light off
		state.backgroundMode = BKG_BLACK
		leds.Reset()
		state.backgroundMode = bkgMode
		go func() {
			for bri := 15; bri < int(state.brightness); bri += 16 {
				setWledState(addr, "bri", bri)
				time.Sleep(time.Second / 40)
			}
			done <- true
		}()
		sendLeds() // takes some time for the leds to react on first send after turnin on
		// so we send this unncecessar update and wait a bit
		time.Sleep(time.Second / 4)
		for t := subleds.keys/2 - 1; t >= 0; t-- {
			for i := 0; i < subleds.keys; i++ {
				if i >= t && i < subleds.keys-t { // turn red
					subleds.set(byte(i), GREEN)
				} else { // clear
					if i == 0 {
						subleds.set(byte(i), BLACK)
					} else {
						subleds.set(byte(i), noteToBkgColor(byte(i*2)))
					}
				}
			}
			sendLeds()
			time.Sleep(time.Second / 10)
		}
		state.backgroundMode = BKG_BLACK
		subleds.Reset()
		<-done
		state.backgroundMode = bkgMode
		leds.Reset()
		sendLeds()
	}
	animateOff := func() {
		done := make(chan bool)
		bkgMode := state.backgroundMode // temporaryly change bkg mode to turn all light off
		go func() {
			for bri := int(state.brightness); bri > 15; bri -= 16 {
				setWledState(addr, "bri", bri)
				time.Sleep(time.Second / 40)
			}
			done <- true
		}()
		for t := 0; t < subleds.keys/2; t++ {
			for i := 0; i < subleds.keys; i++ {
				if i >= t && i < subleds.keys-t { // turn red
					subleds.set(byte(i), RED)
				} else { // clear
					if i == 0 {
						subleds.set(byte(i), BLACK)
					} else {
						subleds.set(byte(i), noteToBkgColor(byte(i*2)))
					}
				}
			}
			sendLeds()
			time.Sleep(time.Second / 10)
		}
		state.backgroundMode = BKG_BLACK
		subleds.Reset()
		<-done
		leds.Reset()
		sendLeds(1) // leave realtime mode soon
		state.backgroundMode = bkgMode
	}

	go func() {
		defer conn.Close()
		sendLeds()
		for {
			select {
			case msg := <-incommingMidi:
				cmd := fromCmd(msg[0])
				note := msg[1]
				if cmd == CMD_CONTROL_CHANGE && note == CC_SUTAIN {
					on := msg[2]
					leds.Sustain(on)
					if ctrl[0] && ctrl[1] && on == 0 { // controlls are pressed and pedal release
						// toggle sustainmode
						state.sustainMode = !state.sustainMode
					}
				}
				if cmd == CMD_NOTE_ON {
					velocity := msg[2]
					if state.active {
						leds.On(note, velocity)
					}
					// controll
					ctrl[0] = ctrl[0] || note == KEY_CTRL_0
					ctrl[1] = ctrl[1] || note == KEY_CTRL_1
					if ctrl[0] && ctrl[1] { // controlls are pressed
						if note == KEY_TOGGLE_ACTIVE && pressingOffTimer == nil { // toggle on/off
							on := getWledState(addr, "on").(bool)
							state.active = state.active && on
							state.active = !state.active
							if state.active {
								if !on {
									setWledState(addr, "on", true)
									animateOn()
								}
							} else {
								leds.Reset()
								sendLeds(2) // leave realtime mode early
							}
							if on {
								pressingOffTimer = time.AfterFunc(time.Second, func() {
									state.active = false
									animateOff()
									setWledState(addr, "on", false)
									pressingOffTimer = nil
								})
							}
						}
						if state.active { // handle key shortucs binding
							switch note {
							case KEY_TOGGLE_BACKGROUND:
								state.backgroundMode = (state.backgroundMode + 1) % 3
								leds.Reset()
							case KEY_DEC_SAT:
								doDecSat = true
								preview()
							case KEY_INC_SAT:
								doIncSat = true
								preview()
							case KEY_DEC_BRI:
								doDecBri = true
								preview()
							case KEY_INC_BRI:
								doIncBri = true
								preview()
							}
							for key, mode := range noteToColorMode { // changing color mode
								if key == note {
									state.colorMode = mode
									preview(true)
								}
							}
						} else { // change favourite presets
							on := getWledState(addr, "on").(bool)
							psId := int(note) - (NOTE_A0 + 3) + 1
							if on && psId > 0 {
								setWledState(addr, "ps", psId)
							}
						}
					}
				}
				if cmd == CMD_NOTE_OFF {
					if state.active {
						leds.Off(note)
					}
					// controlls
					ctrl[0] = ctrl[0] && note != KEY_CTRL_0
					ctrl[1] = ctrl[1] && note != KEY_CTRL_1

					if note == KEY_TOGGLE_ACTIVE && pressingOffTimer != nil {
						pressingOffTimer.Stop()
						pressingOffTimer = nil
						sendLeds(0) // leave realtime mode immideately
					}
					if state.active {
						if doDecSat && note == KEY_DEC_SAT || doIncSat && note == KEY_INC_SAT ||
							doDecBri && note == KEY_DEC_BRI || doIncBri && note == KEY_INC_BRI {
							doDecSat = false
							doIncSat = false
							doDecBri = false
							doIncBri = false
							leds.Reset()
							sendLeds()
						}

						for key, mode := range noteToColorMode {
							if mode == state.colorMode && key == note {
								leds.Reset()
								sendLeds()
							}
						}
					}
				}
				if state.active {
					sendLeds()
				}

			case t := <-ticker.C:
				isEmpty := true
				for _, val := range leds.buffer {
					if val != 0 {
						isEmpty = false
					}
				}
				for _, note := range leds.notes {
					if note.on || note.sus {
						isEmpty = false
					}
				}
				frag := (int(t.Nanosecond()/1000000) % 1000) / FPS
				if frag/8 == 0 { // 8 sat steps per second
					switch {
					case doDecSat:
						decSat(1)
					case doIncSat:
						incSat(1)
					}
				}
				if frag/4 == 0 { // 4 steps per second
					switch {
					case doDecBri:
						decBri(4)
					case doIncBri:
						incBri(4)
					}
				}
				if !isEmpty && state.active {
					sendLeds(WAIT, 0)
				}
			}
			// fmt.Println(">", leds.buffer)
		}
	}()

	power := func(doOn bool) {
		on := getWledState(addr, "on").(bool)
		state.active = state.active && on
		if !state.active && doOn { // turn on
			state.active = true
			setWledState(addr, "on", true)
			animateOn()
		}
		if state.active && !doOn { // turn off
			animateOff()
			setWledState(addr, "on", false)
			state.active = false
		}
	}

	return incommingMidi, power
}

func noteToColor(note byte, velocity byte) RGB {
	if velocity == 0 {
		return noteToBkgColor(note)
	}
	// white mode
	switch state.colorMode {
	case MODE_WHITE:
		return WHITE
	case MODE_WHITE_WARM:
		return WHITE_WARM
	case MODE_WHITE_COLD:
		return WHITE_COLD
	}
	hue := 0
	// rainbow modes
	if state.colorMode >= MODE_RAINBOW_1 && state.colorMode <= MODE_RAINBOW_4 {
		period := 12 * (state.colorMode - MODE_RAINBOW_1 + 1)  // define how much octaves the rainbow stretched
		frac := float64(int(note+24)%period) / float64(period) // shift 24 to match with pian.co
		hue = int(frac * 360)
	} else
	// solid color modes
	if state.colorMode >= MODE_RED && state.colorMode <= MODE_MAGENTA_RED {
		hue = (360 / 12) * (state.colorMode - MODE_RED)
	} else {
		return BLACK
	}
	return colorHStoRGB(hue, state.saturation)
}

func noteToBkgColor(note byte) RGB {
	switch state.backgroundMode {
	case BKG_DIMMED:
		return dimmedColor(noteToColor(note, 64))
	case BKG_LIGHT:
		return dimmedColor(WHITE)
	default:
		return BLACK
	}
}

func dimmedColor(rgb RGB) RGB {
	rgb[0] = rgb[0] / 8
	rgb[1] = rgb[1] / 8
	rgb[2] = rgb[2] / 8
	return rgb
}

// Merge clr1 with clr2 in ratio given by t
// t=0 return clr1
// t=1 return clr2
func avgColor(clr1 RGB, clr2 RGB, t float64) RGB {
	return RGB{
		byte(float64(clr1[0])*(1-t) + float64(clr2[0])*t),
		byte(float64(clr1[1])*(1-t) + float64(clr2[1])*t),
		byte(float64(clr1[2])*(1-t) + float64(clr2[2])*t),
	}
}

// Converts hue (0-360) and saturations (0-255) to RGB bytes
func colorHStoRGB(hue int, sat byte) RGB {
	h := float64(hue%360) / 360
	s := float64(sat) / 255
	i := math.Floor(h * 6)
	var (
		f float64 = h*6 - i
		p float64 = 255 * (1 - s)
		q float64 = 255 * (1 - f*s)
		t float64 = 255 * (1 - (1-f)*s)
	)
	switch int(i) % 6 {
	case 0:
		return RGB{255, byte(t), byte(p)}
	case 1:
		return RGB{byte(q), 255, byte(p)}
	case 2:
		return RGB{byte(p), 255, byte(t)}
	case 3:
		return RGB{byte(p), byte(q), 255}
	case 4:
		return RGB{byte(t), byte(p), 255}
	case 5:
		return RGB{255, byte(p), byte(q)}
	}
	return BLACK
}

func setWledState(addr string, field string, value interface{}) {
	url := "http://" + strings.Split(addr, ":")[0] + ":80" // change the url to http
	url += "/json/state"
	json := fmt.Sprintf(
		`{"v": false, "tt": 1, "%s": %v}`,
		field, value,
	)
	// fmt.Println(url, json)
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(json))
	if err != nil {
		log.Println("Cant send json", err)
	} else {
		defer resp.Body.Close()
		ioutil.ReadAll(resp.Body)
	}
}

func getWledState(addr string, field string) interface{} {
	url := "http://" + strings.Split(addr, ":")[0] + ":80" // change the url to http
	url += "/json/state"

	resp, err := http.Get(url)
	if err != nil {
		log.Println("Can't get state", err)
		return nil
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Can't read state", err)
		return nil
	}

	state := make(map[string]interface{})
	err = json.Unmarshal(data, &state)
	if err != nil {
		log.Println("Can't parse state", err)
		return nil
	}

	return state[field]
}
