package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"time"
)

// fanout.go: A simple unidirectional WS message fanout
// Subscribes to a single WS source and broadcasts each
// incoming message to every connected peer.

// TODO Ping health checks

const maxPressure = 10
const inBuffer = 256

var log = logrus.StandardLogger()

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT not set")
	}

	sourceUrl := os.Getenv("WS_SOURCE")
	if sourceUrl == "" {
		log.Fatal("$WS_SOURCE not set")
	}

	source := make(chan []byte, inBuffer)
	newConns := make(chan *websocket.Conn)

	go manage(source, newConns)

	// Connect to source and ingest messages
	go receiver(sourceUrl, source)

	// Collect WS connections
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		wsHandler(w, r, newConns)
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		log.Fatal(err)
	}
}

// receiver dumps messages from sourceUrl into incoming.
// Kills process if connection fails.
func receiver(sourceUrl string, incoming chan<- []byte) {
	defer close(incoming)
	for {
		err := connectAndReceive(sourceUrl, incoming)
		log.WithError(err).Error("Disconnected from source")
		time.Sleep(10 * time.Second)
	}
}

func connectAndReceive(sourceUrl string, incoming chan<- []byte) error {
	source, _, err := websocket.DefaultDialer.Dial(sourceUrl, nil)
	if err != nil {
		return err
	}

	for {
		msgType, msg, err := source.ReadMessage()
		if err != nil {
			return err
		}
		if msgType != websocket.TextMessage {
			log.Warn("Ignoring incoming non-text message")
			continue
		}

		select {
		case incoming <- msg:
			if log.Level == logrus.DebugLevel {
				log.Debug("Received message")
			}
		default:
			log.Error("Buffer full, dropping incoming message")
		}
	}
}

// acceptor dumps an upgraded connection into conns
func wsHandler(w http.ResponseWriter, r *http.Request, conns chan<- *websocket.Conn) {
	// Upgrade connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithError(err).Warn("Failed to upgrade WS")
		return
	}
	log.WithField("addr", r.RemoteAddr).Info("New connection")
	conns <- conn
}

func manage(source <-chan []byte, newConns chan *websocket.Conn) {
	m := Manager{
		source:   source,
		newConns: newConns,
	}
	m.run()
}
