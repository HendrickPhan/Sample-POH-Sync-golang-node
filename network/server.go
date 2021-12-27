package network

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
)

type Server struct {
	Address string
	IP      string
	Port    int

	MessageHandler *MessageHandler
}

func (server *Server) Run() {
	log.Info(fmt.Sprintf("Starting server at port %d", server.Port))
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", server.IP, server.Port))
	if err != nil {
		log.Error(err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Error(err)
		}

		myConn := Connection{
			TCPConnection: conn,
		}
		server.MessageHandler.OnConnect(&myConn)
		go server.MessageHandler.HandleConnection(&myConn)
	}
}

func (server *Server) ConnectToParent(connection *Connection) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%v:%v", connection.IP, connection.Port))
	if err != nil {
		log.Warn("Error when connect to %v:%v, wallet adress : %v", err)
	} else {
		connection.TCPConnection = conn
		server.MessageHandler.OnConnect(connection)
		go server.MessageHandler.HandleConnection(connection)
	}
}
