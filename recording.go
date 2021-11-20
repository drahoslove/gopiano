package main

import (
	"encoding/json"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Recording represent the metadata of
// a single midi file from the archvie
type Recording struct {
	Time     time.Time
	Duration time.Duration
	Notes    int64
}

func (r *Recording) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Time     int64 `json:"t"`
		Duration int64 `json:"d"`
		Notes    int64 `json:"n"`
	}{
		r.Time.Unix(),
		int64(r.Duration.Seconds()),
		r.Notes,
	})
}

// Recordings is a list of Recording items
// with toJSON method for debug purposes
type Recordings []Recording

func (rs *Recordings) toJSON() string {
	json, err := json.Marshal(rs)
	if err != nil {
		log.Fatal(err)
	}
	return string(json)
}

func recordingFromName(pathName string) Recording {
	var fileName = filepath.Base(pathName) // "2020-08-21 2128 (Friday) 180 notes, 99 seconds.mid"
	var parts = strings.Split(fileName, " ")
	var datePart, timePart, weekdayPart, notesPart, secondsPart string = parts[0], parts[1], parts[2], parts[3], parts[5]
	_ = weekdayPart

	dateTime, err := time.Parse("2006-01-02 1504", strings.Join([]string{datePart, timePart}, " "))
	if err != nil {
		log.Fatal(err)
	}
	duration, err := time.ParseDuration(secondsPart + "s")
	if err != nil {
		log.Fatal(err)
	}
	notes, err := strconv.ParseInt(notesPart, 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	return Recording{
		Time:     dateTime,
		Duration: duration,
		Notes:    notes,
	}
}
