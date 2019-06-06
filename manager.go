package main

import (
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type Manager struct {
	conns map[*websocket.Conn]chan []byte
	source <-chan []byte
	newConns chan *websocket.Conn
}

func (m *Manager) run() {
	m.conns = make(map[*websocket.Conn]chan []byte)
	deadConns := make(chan *websocket.Conn)
	defer m.cleanup()
	for {
		select {
		case message, ok := <-m.source:
			if !ok {
				return
			}
			for conn, msgChan := range m.conns {
				select {
				case msgChan <- message:
					break
				default:
					// Client is too slow, kill it
					logrus.WithField("addr", conn.RemoteAddr().String()).
						Warn("Killing connection to slow peer")
					m.killConn(conn)
				}
			}
		case conn, ok := <-m.newConns:
			if !ok {
				return
			}
			buf := make(chan []byte, maxPressure)
			go outBuffer(conn, buf, deadConns)
			m.conns[conn] = buf
		case conn := <-deadConns:
			m.killConn(conn)
		}
	}
}

func (m *Manager) cleanup() {
	for conn := range m.conns {
		conn.UnderlyingConn().Close()
	}
	logrus.Fatal("Manager died")
}

func (m *Manager) killConn(conn *websocket.Conn) {
	conn.Close()
	delete(m.conns, conn)
}

func outBuffer(conn *websocket.Conn, out <-chan []byte, deadConns chan<- *websocket.Conn) {
	for msg := range out {
		err := conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message to client")
			deadConns <- conn
			return
		}
	}
}
