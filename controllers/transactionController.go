package controllers

import (
	"encoding/hex"

	"github.com/ethereum/go-ethereum/crypto"
	"google.golang.org/protobuf/proto"
	pb "poh-client.com/proto"
)

func GetTransactionHash(transaction *pb.Transaction) string {
	hashData := &pb.HashData{
		FromAddress:  transaction.FromAddress,
		ToAddress:    transaction.ToAddress,
		Pubkey:       transaction.Pubkey,
		Data:         transaction.Data,
		PreviousHash: transaction.PreviousData.Hash,
		PendingUse:   transaction.PendingUse,
		Balance:      transaction.Balance,
	}
	b, _ := proto.Marshal(hashData)
	bHash := crypto.Keccak256(b)
	hash := hex.EncodeToString(bHash)
	return hash
}
