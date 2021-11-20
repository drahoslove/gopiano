package main

import (
	"log"
	"net/url"

	"github.com/gorilla/websocket"
)

// Return channel which
func getWebSocket(addr string) chan []byte {
	u, _ := url.Parse(addr)

	messages := make(chan []byte)

	var ws *websocket.Conn
	var err error
	ws, err = newWs(u.String())
	if err != nil {
		log.Fatal("ws dial failed", err)
	}

	go func() {
		for {
			msg := <-messages
			err := ws.WriteMessage(websocket.BinaryMessage, msg)
			if err != nil { // if first write failed, try to re-dial
				ws, err = newWs(u.String())
				if err != nil {
					log.Println("no ws connection, message dropped", err)
				} else {
					err := ws.WriteMessage(websocket.BinaryMessage, msg)
					if err != nil {
						log.Println("second send try failed", err)
					}
				}
			}
		}
	}()

	return messages
}

func newWs(url string) (*websocket.Conn, error) {
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		go readLoop(ws)
	}
	return ws, err
}

func readLoop(c *websocket.Conn) {
	for {
		if _, _, err := c.NextReader(); err != nil {
			c.Close()
			break
		}
	}
}
