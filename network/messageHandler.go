package network

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
	"google.golang.org/protobuf/proto"
	"poh-client.com/config"
	"poh-client.com/controllers"
	pb "poh-client.com/proto"
)

type MessageHandler struct {
	mu                   sync.Mutex
	ValidatorConnections map[string]*Connection // map address to validator connection
	NodeConnections      map[string]*Connection // map address to validator connection

	LastConfirmedTick        *pb.POHTick
	NextLeaderTicks          []*pb.POHTick
	CurrentLeaderTicks       []*pb.POHTick
	CurrentLeaderFutureTicks []*pb.POHTick

	AccountDB                         *leveldb.DB
	CheckingAccountDataFromLeaderTick map[string]*pb.AccountData
}

func (handler *MessageHandler) OnConnect(conn *Connection) {
	log.Infof("OnConnect with server %s\n", conn.TCPConnection.RemoteAddr())
	conn.SendInitConnection()
}

func (handler *MessageHandler) OnDisconnect(conn *Connection) {
	log.Infof("Disconnected with server  %s, wallet address: %v \n", conn.TCPConnection.RemoteAddr(), conn.Address)
	// TODO remove from connection list

}

func (handler *MessageHandler) HandleConnection(conn *Connection) {
	for {
		bLength := make([]byte, 8)
		_, err := conn.TCPConnection.Read(bLength)
		if err != nil {
			switch err {
			case io.EOF:
				handler.OnDisconnect(conn)
				return
			default:
				log.Error("server error: %v\n", err)
				return
			}
		}
		messageLength := uint64(binary.LittleEndian.Uint64(bLength))

		data := make([]byte, messageLength)
		byteRead, err := conn.TCPConnection.Read(data)

		if err != nil {
			switch err {
			case io.EOF:
				handler.OnDisconnect(conn)
				return
			default:
				log.Error("server error: %v\n", err)
				return
			}
		}
		for uint64(byteRead) != messageLength {
			log.Errorf("Invalid message receive byteRead !=  messageLength %v, %v\n", byteRead, messageLength)
			appendData := make([]byte, messageLength-uint64(byteRead))
			conn.TCPConnection.Read(appendData)

			data = append(data[:byteRead], appendData...)
			byteRead = len(data)
		}

		message := pb.Message{}
		proto.Unmarshal(data[:messageLength], &message)
		go handler.ProcessMessage(conn, &message)
	}
}

func (handler *MessageHandler) ProcessMessage(conn *Connection, message *pb.Message) {
	// need init connection before do anything
	switch message.Header.Command {
	case "InitConnection":
		handler.handleInitConnectionMessage(conn, message)
	case "LeaderTick":
		handler.handleLeaderTick(conn, message)
	case "ConfirmResult":
		handler.handleConfirmResult(conn, message)
	default:
	}
}

func (handler *MessageHandler) handleInitConnectionMessage(conn *Connection, message *pb.Message) {
	log.Info("Receive InitConnection from", conn.TCPConnection.RemoteAddr())
	initConnectionMessage := &pb.InitConnection{}
	proto.Unmarshal([]byte(message.Body), initConnectionMessage)
	conn.Address = initConnectionMessage.Address
	conn.Type = initConnectionMessage.Type

	handler.mu.Lock()
	if conn.Type == "Validator" {
		// TODO: should have node type in init connection to add connect to right list ex: validator, node, miner
		handler.ValidatorConnections[conn.Address] = conn
	} else {
		handler.NodeConnections[conn.Address] = conn
	}
	handler.mu.Unlock()
}

func (handler *MessageHandler) generatePOHHash(transactions []*pb.Transaction, lastHash *pb.POHHash) *pb.POHHash {
	h := sha256.New()
	transactionsHashString := ""
	for _, transaction := range transactions {
		transactionsHashString += "|" + transaction.Hash
	}

	h.Write([]byte(strconv.FormatInt(lastHash.Count, 10) + lastHash.Hash + transactionsHashString))

	result := &pb.POHHash{
		Count:        lastHash.Count + 1,
		LastHash:     lastHash.Hash,
		Hash:         hex.EncodeToString(h.Sum(nil)),
		Transactions: transactions,
	}
	return result
}

func (handler *MessageHandler) validateTickPOH(tick *pb.POHTick) error {
	log.Infof("HASH LEN %v\n", len(tick.Hashes))
	validatePOHChan := make(chan bool)
	exitChan := make(chan bool)
	for i := 0; i < config.AppConfig.NumberOfValidatePohRoutine; i++ {
		hashPerTick := config.AppConfig.HashPerSecond / config.AppConfig.TickPerSecond
		hashNeedValidatePerRoutine := int(math.Ceil(float64(hashPerTick) / float64(config.AppConfig.NumberOfValidatePohRoutine))) // ceil to not miss any transaction
		go func(i int) {
			validateFromIdx := i*hashNeedValidatePerRoutine - 1
			if validateFromIdx < 0 {
				validateFromIdx = 0
			}
			validateToIdx := (i + 1) * hashNeedValidatePerRoutine
			hashNeedValidate := tick.Hashes[validateFromIdx:validateToIdx]
			for i := 1; i < len(hashNeedValidate); i++ {
				select {
				case <-exitChan:
					return
				default:
					rightHash := handler.generatePOHHash(hashNeedValidate[i].Transactions, hashNeedValidate[i-1])
					if rightHash.Hash != hashNeedValidate[i].Hash {
						validatePOHChan <- false
						return
					}
				}
			}
			validatePOHChan <- true
		}(i)
	}

	totalValidRoutine := 0
	for {
		valid := <-validatePOHChan
		if !valid {
			<-exitChan
			return errors.New("invalid poh\n")
		} else {
			totalValidRoutine++
		}
		if totalValidRoutine == config.AppConfig.NumberOfValidatePohRoutine {
			return nil
		}

	}
}

func (handler *MessageHandler) validateTickTransactions(tick *pb.POHTick) bool {
	// TODO
	for _, hash := range tick.Hashes {
		for _, transaction := range hash.Transactions {
			if !handler.validateTransaction(transaction) {
				log.Info("Transaction Invalid")
				return false
			}
		}
	}
	return true
}
func (handler *MessageHandler) validateTransaction(transaction *pb.Transaction) bool {
	// validate hash
	hash := controllers.GetTransactionHash(transaction)
	if hash != transaction.Hash {
		return false
	}
	// validate lastHash
	lastHash := controllers.GetTransactionHash(transaction.PreviousData)
	if transaction.PreviousData.Hash != lastHash {
		return false
	}
	if _, ok := handler.CheckingAccountDataFromLeaderTick[transaction.FromAddress]; !ok {
		bAccountData, err := handler.AccountDB.Get([]byte(transaction.FromAddress), nil)
		if err != nil {
			log.Warn("Account have no data but create send transaction. Address: %v\n", transaction.FromAddress)
			return false
		}
		accountData := &pb.AccountData{}
		proto.Unmarshal(bAccountData, accountData)
		handler.CheckingAccountDataFromLeaderTick[transaction.FromAddress] = accountData
	}
	if transaction.PreviousData.Hash != handler.CheckingAccountDataFromLeaderTick[transaction.FromAddress].LastHash {
		return false
	}

	// validate balance
	if transaction.PendingUse > handler.CheckingAccountDataFromLeaderTick[transaction.FromAddress].PendingBalance {
		return false
	}

	if transaction.Balance < 0 || // balance cannot be < 0
		transaction.Balance >= transaction.PreviousData.Balance+transaction.PendingUse { // balance after send not be larger or equal transaction.PreviousData.Balance+transaction.PendingUse
		return false
	}
	// update account data so we can continue validate next transaction of this account
	handler.CheckingAccountDataFromLeaderTick[transaction.FromAddress].Balance = transaction.Balance
	handler.CheckingAccountDataFromLeaderTick[transaction.FromAddress].LastHash = transaction.Hash
	handler.CheckingAccountDataFromLeaderTick[transaction.FromAddress].PendingBalance -= transaction.PendingUse
	return true
}

func (handler *MessageHandler) handleLeaderTick(conn *Connection, message *pb.Message) {
	tick := &pb.POHTick{}
	proto.Unmarshal([]byte(message.Body), tick)
	log.Infof("Receive leader tick %v\n", tick.Count)

	validateTickResult := &pb.POHValidateTickResult{
		Tick:  tick,
		From:  config.AppConfig.Address,
		Valid: true,
		Sign:  "TODO",
	}
	// check tick count valid
	if tick.Count < handler.LastConfirmedTick.Count {
		log.Infof("handleLeaderTick Receive old tick. last confirmed tick count: %v, tick count: %v\n", handler.LastConfirmedTick.Count, tick.Count)
		// Send result back to validator
		go conn.SendValidateTickResult(validateTickResult, false)
		return
	}
	// validate POH
	err := handler.validateTickPOH(tick)
	if err != nil {
		log.Infof("handleLeaderTick wrong POH: %v", err)
		// Send result back to validator
		go conn.SendValidateTickResult(validateTickResult, false)
		return
	}
	// add to next leader or current or future
	handler.mu.Lock()
	if tick.Count > handler.LastConfirmedTick.Count+int64(config.AppConfig.TickPerSlot) {
		// rerecieve tick from next leader
		handler.NextLeaderTicks = append(handler.NextLeaderTicks, tick)
		handler.mu.Unlock()
		return
	} else {
		lastTick, lastHash := handler.getLastTickAndLastHashFromCurrentLeaderTicks()
		if tick.Count == lastTick.Count+1 {
			// validate hash
			rightHash := handler.generatePOHHash(tick.Hashes[0].Transactions, lastHash)
			if rightHash.Hash != tick.Hashes[0].Hash {
				log.Infof("handleLeaderTick wrong last hash\n")
				// Send result back to validator
				handler.mu.Unlock()
				go conn.SendValidateTickResult(validateTickResult, false)
				return
			}
			handler.CurrentLeaderTicks = append(handler.CurrentLeaderTicks, tick)
			// Send result back to validator
			validTransactions := handler.validateTickTransactions(tick)
			if validTransactions {
				go conn.SendValidateTickResult(validateTickResult, true)
			} else {
				log.Infof("handleLeaderTick invalid transaction \n")
				go conn.SendValidateTickResult(validateTickResult, false)
			}
			// check future ticks
			handler.checkFutureTick(conn)
			handler.mu.Unlock()
			return
		} else {
			handler.addFutureTick(tick)
			handler.mu.Unlock()
			return
		}
	}

}

func (handler *MessageHandler) addFutureTick(tick *pb.POHTick) {
	log.Infof("Add future tick %v\n", tick.Count)
	handler.CurrentLeaderFutureTicks = append(handler.CurrentLeaderFutureTicks, tick)
	sort.Slice(handler.CurrentLeaderFutureTicks, func(i, j int) bool {
		return handler.CurrentLeaderFutureTicks[i].Count < handler.CurrentLeaderFutureTicks[j].Count
	})
}

func (handler *MessageHandler) getLastTickAndLastHashFromCurrentLeaderTicks() (*pb.POHTick, *pb.POHHash) {
	var lastTickFromCurrentLeaderTicks *pb.POHTick
	if len(handler.CurrentLeaderTicks) == 0 {
		lastTickFromCurrentLeaderTicks = handler.LastConfirmedTick
	} else {
		lastTickFromCurrentLeaderTicks = handler.CurrentLeaderTicks[len(handler.CurrentLeaderTicks)-1]
	}
	lastHashFromCurrentLeaderTicks := lastTickFromCurrentLeaderTicks.Hashes[len(lastTickFromCurrentLeaderTicks.Hashes)-1]
	return lastTickFromCurrentLeaderTicks, lastHashFromCurrentLeaderTicks
}

func (handler *MessageHandler) checkFutureTick(conn *Connection) {
	if len(handler.CurrentLeaderFutureTicks) <= 0 {
		return
	}
	lastTick, lastHash := handler.getLastTickAndLastHashFromCurrentLeaderTicks()
	for len(handler.CurrentLeaderFutureTicks) > 0 && handler.CurrentLeaderFutureTicks[0].Count == lastTick.Count+1 {
		// validate hash
		rightHash := handler.generatePOHHash(handler.CurrentLeaderFutureTicks[0].Hashes[0].Transactions, lastHash)
		if rightHash.Hash != handler.CurrentLeaderFutureTicks[0].Hashes[0].Hash {
			log.Infof("handleLeaderTick future tick wrong last hash\n")
			continue
		}

		handler.CurrentLeaderTicks = append(handler.CurrentLeaderTicks, handler.CurrentLeaderFutureTicks[0])
		validateTickResult := &pb.POHValidateTickResult{
			Tick: handler.CurrentLeaderFutureTicks[0],
			From: config.AppConfig.Address,
		}
		go conn.SendValidateTickResult(validateTickResult, true)
		// Send result back to validator
		handler.CurrentLeaderFutureTicks = handler.CurrentLeaderFutureTicks[1:]
		lastTick, lastHash = handler.getLastTickAndLastHashFromCurrentLeaderTicks()
	}
}

func (handler *MessageHandler) UpdateAccountDB(newAccountDatas map[string]*pb.AccountData) {
	batch := new(leveldb.Batch)
	for _, v := range newAccountDatas {
		b, _ := proto.Marshal(v)
		batch.Put([]byte(v.Address), b)
	}
	err := handler.AccountDB.Write(batch, nil)
	if err != nil {
		log.Errorf("Error when update account db %v\n", err)
	}
}

func (handler *MessageHandler) handleConfirmResult(conn *Connection, message *pb.Message) {
	handler.mu.Lock()
	confirmRs := &pb.POHConfirmResult{}
	proto.Unmarshal([]byte(message.Body), confirmRs)

	// reset data to move on next leader
	handler.LastConfirmedTick = confirmRs.LastTick
	handler.CurrentLeaderFutureTicks = handler.NextLeaderTicks
	handler.CurrentLeaderTicks = []*pb.POHTick{}
	handler.NextLeaderTicks = []*pb.POHTick{}

	handler.checkFutureTick(conn)

	// clear checkingAccountDataFromLeaderTick cuz already confirm to move on  data from next leader
	handler.CheckingAccountDataFromLeaderTick = make(map[string]*pb.AccountData)

	// update db
	handler.UpdateAccountDB(confirmRs.AccountDatas)

	handler.mu.Unlock()
}
