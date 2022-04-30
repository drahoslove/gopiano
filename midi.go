package main

import (
	"fmt"
	"log"

	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/reader"

	driver "gitlab.com/gomidi/rtmididrv"
)

const CMD_NOTE_OFF = 0
const CMD_NOTE_ON = 1
const CMD_CONTROL_CHANGE = 3

const CC_BANK_0 = 0
const CC_BANK_1 = 32
const CC_SUTAIN = 64

const NOTE_A0 = 21
const NOTE_C8 = 108

const CHANNEL = 3 // this is what roland actually uses as output
// 1=piano,  2,3=main layer, 9=silent, rest=piano

func toCmd(x int) byte {
	ch := byte(CHANNEL)
	return (1<<3|byte(x))<<4 | ch
}
func fromCmd(cmd byte) int {
	return int((cmd >> 4) & 7)
}
func chanFromCmd(cmd byte) byte {
	return cmd & 0x0F
}

// 0..1 to 0..127
func toVal(x float64) byte {
	return byte(x * 127)
}

// 0..127 to 0..1
func fromVal(val byte) float64 {
	return float64(val) / 127
}

// will translate 'note on' with zero velocity as 'note off'
// and sets channel to CHANNEL
func normalizeMidiMsg(msg []byte) []byte {
	cmd := msg[0]
	if fromCmd(cmd) == CMD_NOTE_ON {
		note := msg[1]
		velocity := msg[2]
		if fromVal(velocity) == 0 {
			cmd = toCmd(CMD_NOTE_OFF)
			return []byte{cmd, note}
		}
	}
	msg[0] = toCmd(fromCmd(cmd)) // force CHANNEL
	return msg
}

// return true if the midi message is one of:
// note on
// note off
// control change - sustain pedal
func isBasicMessage(msg []byte) bool {
	switch fromCmd(msg[0]) {
	case CMD_NOTE_ON, CMD_NOTE_OFF:
		return true
	case CMD_CONTROL_CHANGE:
		if msg[1] == CC_SUTAIN {
			return true
		}
	}
	return false
}

// returns a channel witch emmits all
// messages from all connected midi devices
// and a cleanup func
func getMidiMessages(devInName string) (chan []byte, func()) {
	messages := make(chan []byte)

	drv, err := driver.New()
	must(err)

	var in midi.In

	rd := reader.New(
		reader.NoLogger(),
		// pass every incoming message to the channel
		reader.Each(func(pos *reader.Position, msg midi.Message) {
			messages <- msg.Raw()
			// log.Println("msg captured:", msg.String())
		}),
	)

	{
		virtIn, err := drv.OpenVirtualIn(devInName)
		must(err)
		must(virtIn.Open())
		rd.ListenTo(virtIn)
		in = virtIn
		log.Println("Virt midi in device created:", virtIn.String())
	}

	return messages, func() {
		fmt.Println("closing midi")
		in.Close()
		drv.Close()
	}
}

func must(err error) {
	if err != nil {
		panic(err.Error())
	}
}
