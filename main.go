package main

import (
	"encoding/json"
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

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		logrus.Fatal("$PORT not set")
	}

	sourceUrl := os.Getenv("WS_SOURCE")
	if sourceUrl == "" {
		logrus.Fatal("$WS_SOURCE not set")
	}

	source := make(chan []byte)
	newConns := make(chan *websocket.Conn)

	go manage(source, newConns)

	// Connect to source and ingest messages
	go receiver(sourceUrl, source)

	// Collect WS connections
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		wsHandler(w, r, newConns)
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		logrus.Fatal(err)
	}
}

// receiver dumps messages from sourceUrl into incoming.
// Kills process if connection fails.
func receiver(sourceUrl string, incoming chan<- []byte) {
	defer close(incoming)
	for {
		err := connectAndReceive(sourceUrl, incoming)
		logrus.WithError(err).Error("Disconnected from source")
		time.Sleep(10 * time.Second)
	}
}

func connectAndReceive(sourceUrl string, incoming chan<- []byte) error {
	source, _, err := websocket.DefaultDialer.Dial(sourceUrl, nil)
	if err != nil {
		return err
	}

	_ = source.WriteMessage(websocket.TextMessage,
		[]byte(`{"jsonrpc":"2.0","id":42,"method":"subscribe","params":"subscribe"}`))

	for {
		msgType, msg, err := source.ReadMessage()
		if err != nil {
			return err
		}
		if msgType != websocket.TextMessage {
			logrus.Warn("Ignoring incoming non-text message")
			continue
		}
		var m JrpcMessage
		if err := json.Unmarshal(msg, &m); err != nil {
			logrus.Error(err)
			continue
		}
		if m.Method != "subscription" {
			continue
		}
		incoming <- m.Result
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
		logrus.WithError(err).Warn("Failed to upgrade WS")
		return
	}
	logrus.WithField("addr", r.RemoteAddr).Info("New connection")
	conns <- conn
}

func manage(source <-chan []byte, newConns chan *websocket.Conn) {
	m := Manager{
		source:   source,
		newConns: newConns,
	}
	m.run()
}

type JrpcMessage struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}
