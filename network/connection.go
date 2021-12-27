package network

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"google.golang.org/protobuf/proto"
	"poh-client.com/config"
	pb "poh-client.com/proto"
)

type Connection struct {
	Address       string
	IP            string
	Port          int
	TCPConnection net.Conn
	Type          string
	mu            sync.Mutex
}

func (conn *Connection) SendMessage(message *pb.Message) error {
	b, err := proto.Marshal(message)
	if err != nil {
		fmt.Printf("Error when marshal %v", err)
		return err
	}
	length := make([]byte, 8)
	binary.LittleEndian.PutUint64(length, uint64(len(b)))
	conn.mu.Lock()
	conn.TCPConnection.Write(length)
	conn.TCPConnection.Write(b)
	conn.mu.Unlock()

	return nil
}

func (conn *Connection) SendInitConnection() {
	protoRs, _ := proto.Marshal(&pb.InitConnection{
		Address: config.AppConfig.Address,
		Type:    "Node",
	})
	message := &pb.Message{
		Header: &pb.Header{
			Type:    "request",
			From:    config.AppConfig.Address,
			Command: "InitConnection",
		},
		Body: protoRs,
	}

	err := conn.SendMessage(message)
	if err != nil {
		fmt.Printf("Error when send started %v", err)
	}
}

func (conn *Connection) SendLeaderTick(tick *pb.POHTick) {
	protoRs, _ := proto.Marshal(tick)
	message := &pb.Message{
		Header: &pb.Header{
			Type:    "request",
			From:    config.AppConfig.Address,
			Command: "LeaderTick",
		},
		Body: protoRs,
	}

	err := conn.SendMessage(message)
	if err != nil {
		fmt.Printf("Error when send started %v", err)
	}
}

func (conn *Connection) SendCheckedBlock(block *pb.CheckedBlock) {
	protoRs, _ := proto.Marshal(block)
	message := &pb.Message{
		Header: &pb.Header{
			Type:    "request",
			From:    config.AppConfig.Address,
			Command: "SendCheckedBlock",
		},
		Body: protoRs,
	}

	err := conn.SendMessage(message)
	if err != nil {
		fmt.Printf("Error when send started %v", err)
	}
}

func (conn *Connection) SendValidateTickResult(validateResult *pb.POHValidateTickResult, valid bool) {
	validateResult.Valid = valid
	validateResult.Sign = "TODO"
	// TODO: sign
	protoRs, _ := proto.Marshal(validateResult)
	message := &pb.Message{
		Header: &pb.Header{
			Type:    "request",
			From:    config.AppConfig.Address,
			Command: "ValidateTickResult",
		},
		Body: protoRs,
	}

	err := conn.SendMessage(message)
	if err != nil {
		fmt.Printf("Error when send started %v", err)
	}
}
