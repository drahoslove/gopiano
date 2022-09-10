package main

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gitlab.com/gomidi/midi/midimessage/channel" // (Channel Messages)
	"gitlab.com/gomidi/midi/smf"
	"gitlab.com/gomidi/midi/smf/smfreader"
)

// Recording represent the metadata of
// a single midi file from the archvie
type Recording struct {
	Time     time.Time
	Duration time.Duration
	Notes    int64 // kes total (sum of keys)
	// Keys     *[88]int // key pressed by notes
	Keys *map[byte]int
}

func (r *Recording) MarshalJSON() ([]byte, error) {
	// keys := []int{}
	keys := &map[byte]int{}
	if r.Keys != nil {
		keys = r.Keys
		// keys = (*r.Keys)[:]
		// for i := byte(0); i < 88; i++ {
		// 	if val, ok := (*r.Keys)[i]; ok {
		// 		keys[i] = val
		// 	}
	}
	return json.Marshal(&struct {
		Time     int64         `json:"t"`
		Duration int64         `json:"d"`
		Notes    int64         `json:"n"`
		Keys     *map[byte]int `json:"k,omitempty"`
	}{
		r.Time.Unix(),
		int64(r.Duration.Seconds()),
		r.Notes,
		keys,
	})
}

func (r *Recording) load88(pathname string) error {
	if r.Keys != nil { // already loaded
		return nil
	}

	// keys88 := [88]int{}
	keys88 := make(map[byte]int)
	sum := 0

	midFile, err := os.Open(pathname)
	if err != nil {
		return fmt.Errorf("failed to open mid file: %s", err)
	}
	defer midFile.Close()
	reader := smfreader.New(midFile)

	for {
		m, err := reader.Read()
		if err != nil {
			break
		}
		switch msg := m.(type) {
		case channel.NoteOn:
			key := int(msg.Key())
			key -= NOTE_A0
			if key >= 0 && key < 88 { // count the note
				if val, ok := keys88[byte(key)]; ok {
					keys88[byte(key)] = val + 1
				} else {
					keys88[byte(key)] = 1
				}
				// keys88[key]++
				sum++
			}
		}
	}
	if err != nil && err != smf.ErrFinished {
		return fmt.Errorf("failed to parse mid file: %s", err.Error())
	}

	r.Keys = &keys88 // attach to self

	if sum != int(r.Notes) {
		fmt.Printf("invalid keys count (%v): %s\n", sum, pathname)
	}

	return nil
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

	location, err := time.LoadLocation("Local")
	if err != nil {
		log.Fatal(err)
	}
	dateTime, err := time.ParseInLocation(
		"2006-01-02 1504",
		strings.Join([]string{datePart, timePart}, " "),
		location,
	)
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

func timeOfDir(dirPath string) *time.Time {
	var parts = strings.Split(filepath.ToSlash(dirPath), "/")
	if len(parts) < 2 {
		return nil
	}
	yyyy := parts[len(parts)-2]
	mm := parts[len(parts)-1]
	if len(mm) != 2 || len(yyyy) != 4 {
		return nil
	}
	location, err := time.LoadLocation("Local")
	if err != nil {
		log.Fatal(err)
	}
	startOfMonth, err := time.ParseInLocation(
		"2006/01",
		yyyy+"/"+mm,
		location,
	)
	if err != nil {
		log.Fatal(err)
	}
	return &startOfMonth
}

func recordingsFromDir(dirPath string) Recordings {
	recordings, err := recordingsFromGob(filepath.Join(dirPath, "/recordings.gob"))
	var newestCachedMonthEnd time.Time // only cache full month
	var lenLoaded = len(recordings)
	if err != nil {
		fmt.Println(err)
	} else {
		newestCachedMonthEnd = endOfMonth(recordings[len(recordings)-1].Time)
		// fmt.Println("loaded", lenLoaded, newestCachedMonthEnd)
	}

	// stats := loadStats(dirPath)
	filepath.WalkDir(dirPath, func(pathname string, info fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
			return nil
		}
		if info.IsDir() { // check if the dir is done
			dirTime := timeOfDir(pathname)
			if dirTime != nil && !endOfMonth(*dirTime).After(newestCachedMonthEnd) { // dir already in cache
				// fmt.Println("skipping", pathname)
				return fs.SkipDir
			} else {
				if dirTime != nil && dirTime.Before(time.Now()) && lenLoaded < len(recordings) { // whole month passed
					err := recordings.saveToGob(filepath.Join(dirPath, "/recordings.gob"))
					// fmt.Println("saving", pathname, len(recordings))
					if err != nil {
						log.Fatal(err)
					}
				} else {
					// fmt.Println("not skipped, not saved", pathname, dirTime, newestCachedMonthEnd, lenLoaded < len(recordings))
				}
			}
			return nil
		}
		if filepath.Ext(pathname) != ".mid" { // skip dirs and non midi files
			return nil
		}
		rec := recordingFromName(pathname)
		if !rec.Time.After(newestCachedMonthEnd) {
			// checked := false
			// for _, r := range recordings {
			// 	if r.Time.Equal(rec.Time) {
			// 		checked = true
			// 	}
			// }
			// fmt.Println("already in the collection", pathname, checked)
			return nil
		}
		err = rec.load88(pathname)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		recordings = append(recordings, rec)
		return nil
	})

	// fmt.Println("returning", len(recordings))

	return recordings
}

func recordingsFromGob(gobPath string) (recordings Recordings, err error) {
	gobFile, err := os.Open(gobPath)
	if err != nil {
		err = fmt.Errorf("failed to open for read gob file, %s", err.Error())
		return
	}
	defer gobFile.Close()
	decoder := gob.NewDecoder(gobFile)
	err = decoder.Decode(&recordings)
	if err != nil {
		err = fmt.Errorf("failed to decode gob file, %s", err.Error())
	}
	return
}

func (recs *Recordings) saveToGob(gobPath string) (err error) {
	gobFile, err := os.Create(gobPath)
	if err != nil {
		err = fmt.Errorf("faield to open for save gob file, %s", err.Error())
		return
	}
	defer gobFile.Close()
	encoder := gob.NewEncoder(gobFile)
	err = encoder.Encode(recs)
	if err != nil {
		err = fmt.Errorf("faield to encode gob file, %s", err.Error())
	}
	return
}

func endOfMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0).Add(-time.Nanosecond)
}
