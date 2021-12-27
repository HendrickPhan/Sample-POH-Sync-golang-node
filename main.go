package main

import (
	"time"

	"poh-client.com/config"
	"poh-client.com/network"
	pb "poh-client.com/proto"
)

func initParentConnections() *network.Connection {
	parentConfig := config.AppConfig.ParentConnection
	parentConnection := &network.Connection{
		IP:      parentConfig.Ip,
		Port:    parentConfig.Port,
		Address: parentConfig.Address,
		Type:    parentConfig.Type,
	}
	return parentConnection
}

func runServer() *network.Server {

	handler := network.MessageHandler{
		ValidatorConnections: make(map[string]*network.Connection),
		NodeConnections:      make(map[string]*network.Connection),
		LastConfirmedTick: &pb.POHTick{
			Count: -1000,
		},
	}
	server := network.Server{
		Address:        config.AppConfig.Address,
		IP:             config.AppConfig.Ip,
		Port:           config.AppConfig.Port,
		MessageHandler: &handler,
	}
	go server.Run()
	go server.ConnectToParent(initParentConnections())
	return &server
}

func main() {
	finish := make(chan bool)

	server := runServer()

	checkedBlock := &pb.CheckedBlock{
		Transactions: []*pb.Transaction{
			{
				Hash:     "Hash",
				LastHash: "LastHash",
				From:     "From",
				To:       "To",
				Balance:  9000000,
				Sign:     "Sign",
			},
		},
	}
	for {
		if len(server.MessageHandler.ValidatorConnections) > 0 {
			for _, v := range server.MessageHandler.ValidatorConnections {
				v.SendCheckedBlock(checkedBlock)
			}
		}
		time.Sleep(1000 * time.Microsecond)
	}

	<-finish

}
