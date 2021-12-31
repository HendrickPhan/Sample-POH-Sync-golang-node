package main

import (
	"time"

	"github.com/syndtr/goleveldb/leveldb"
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

func runServer(accountDB *leveldb.DB) *network.Server {

	handler := network.MessageHandler{
		ValidatorConnections: make(map[string]*network.Connection),
		NodeConnections:      make(map[string]*network.Connection),
		LastConfirmedTick: &pb.POHTick{
			Count: -1000,
		},
		CheckingAccountDataFromLeaderTick: make(map[string]*pb.AccountData),
		AccountDB:                         accountDB,
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
	// db
	accountDB, err := leveldb.OpenFile(config.AppConfig.AccountDBPath, nil)
	if err != nil {
		panic(err)
	}
	defer accountDB.Close()

	// runServer(accountDB)
	server := runServer(accountDB)

	checkedBlock := &pb.CheckedBlock{
		Transactions: []*pb.Transaction{
			{
				FromAddress: "Hash",
				ToAddress:   "To",
				Balance:     9000000,
				Sign:        "Sign",
				PreviousData: &pb.Transaction{
					Hash: "fake hash",
				},
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
