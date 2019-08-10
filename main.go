package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// fanout.go: A simple unidirectional WS message fanout
// Subscribes to a single WS source and broadcasts each
// incoming message to every connected peer.

// TODO Ping health checks

const maxPressure = 10
const inBuffer = 256

var log = logrus.StandardLogger()

func main() {
	viper.SetEnvPrefix("fanout")
	viper.AutomaticEnv()

	if len(os.Args) == 2 {
		if strings.Contains(os.Args[1], "help") {
			usage()
		}
		viper.SetConfigFile(os.Args[1])
		err := viper.ReadInConfig()
		if err != nil {
			log.Fatal(err)
		}
	} else if len(os.Args) != 1 {
		usage()
	}

	if viper.GetString("bind") == "" {
		log.Fatal(`Key "bind" not set`)
	}
	if viper.GetString("source") == "" {
		log.Fatal(`Key "source" not set`)
	}

	source := make(chan []byte, inBuffer)
	newConns := make(chan *websocket.Conn)

	go manage(source, newConns)

	// Connect to source and ingest messages
	go receiver(viper.GetString("source"), source)

	// Collect WS connections
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsHandler(w, r, newConns)
	})

	log.Infof("Listening on %s", viper.GetString("bind"))

	var err error
	if viper.GetBool("tls.enabled") {
		err = http.ListenAndServeTLS(viper.GetString("bind"),
			viper.GetString("tls.cert"), viper.GetString("tls.key"), handler)
	} else {
		err = http.ListenAndServe(viper.GetString("bind"), handler)
	}

	log.Fatal(err)
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

	logrus.Infof("Connected to %s", sourceUrl)

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

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "Usage: ws-fanout [config_file.yml]\n" +
		"  or over environment: $FANOUT_...")
	os.Exit(1)
}
