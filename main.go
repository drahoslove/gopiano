package main

// GOOS=linux GOARCH=arm GOARM=7 go build

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

const ADDR = ":1212"

var piancoAddr = flag.String("addr", "wss://pianoecho.draho.cz", "pianco api ws address")
var wledAddr = flag.String("wled", "192.168.1.3:21324", "udp address of warls")
var archiveDir = "/home/pi/.local/share/Modartt/Pianoteq/Archive"

func init() {
	if runtime.GOOS == "windows" { // for local testing
		archiveDir = "./Archive"
	}
}

func main() {
	flag.Parse()

	websocket := getWebSocket(*piancoAddr)
	messages, closeMidi := getMidiMessages("gopiano")
	wled, wledPower := getWled(*wledAddr)

	GID := byte(0)
	UID := byte(0)

	// Return json containing data of recordings obtained from names of mid files created from pianoteq
	http.HandleFunc("/archive.json", func(w http.ResponseWriter, r *http.Request) {
		setupResponse(&w, r)
		recordings := Recordings{}
		filepath.Walk(archiveDir, func(pathname string, info os.FileInfo, err error) error {
			if err != nil {
				log.Fatal(err)
				return nil
			}
			if info.IsDir() || filepath.Ext(pathname) != ".mid" { // skip dirs and non midi files
				return nil
			}
			recordings = append(recordings, recordingFromName(pathname))
			return nil
		})
		fmt.Fprintln(w, recordings.toJSON())
	})

	// this is to test the pianco api
	http.HandleFunc("/emitrandomnote", func(w http.ResponseWriter, r *http.Request) {
		setupResponse(&w, r)
		// ws.WriteMessage(websocket.TextMessage, []byte("playrandomfile 0 0")) // BinaryMessage
		note := byte(NOTE_A0 + rand.Intn(NOTE_C8-NOTE_A0))
		websocket <- []byte{GID, UID, toCmd(CMD_NOTE_ON), note, toVal(0.5)}
		wled <- []byte{toCmd(CMD_NOTE_ON), note, toVal(0.5)}
		<-time.After(time.Second / 2)
		websocket <- []byte{GID, UID, toCmd(CMD_NOTE_OFF), note}
		wled <- []byte{toCmd(CMD_NOTE_OFF), note}
	})

	// wled api
	http.HandleFunc("/wled/on", func(w http.ResponseWriter, r *http.Request) {
		setupResponse(&w, r)
		wledPower(true)
	})

	http.HandleFunc("/wled/off", func(w http.ResponseWriter, r *http.Request) {
		setupResponse(&w, r)
		wledPower(false)
	})
	http.HandleFunc("/wled/set/bri", func(w http.ResponseWriter, r *http.Request) {
		setupResponse(&w, r)
		setWledState(*wledAddr, "bri", 20)
		time.Sleep(time.Second / 2)
		setWledState(*wledAddr, "bri", 60)
		time.Sleep(time.Second / 2)
		setWledState(*wledAddr, "bri", 120)
		time.Sleep(time.Second / 2)
		setWledState(*wledAddr, "bri", 200)
	})

	http.HandleFunc("/wled/get/on", func(w http.ResponseWriter, r *http.Request) {
		setupResponse(&w, r)
		on := getWledState(*wledAddr, "on").(bool)
		fmt.Println("on", on)
	})

	// send midi messages from device to server
	go func() {
		for {
			msg := normalizeMidiMsg(<-messages)
			if isBasicMessage(msg) {
				// prepe pend gid and uid required by pianco api
				wrappedMsg := append([]byte{GID, UID}, msg...)
				websocket <- wrappedMsg
				wled <- msg
			}
		}
	}()

	// setup cleanup procedures
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		closeMidi()
		wledPower(false)
		os.Exit(1)
	}()

	log.Println("Starting", ADDR)

	if err := http.ListenAndServe(ADDR, nil); err != nil {
		log.Fatal(err)
	}
}

func setupResponse(w *http.ResponseWriter, req *http.Request) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}
