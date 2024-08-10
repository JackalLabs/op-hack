package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	wallet2 "optijack/jackal/wallet"
	"optijack/server"
	"os"
	"time"

	"github.com/desmos-labs/cosmos-go-wallet/types"
	"github.com/desmos-labs/cosmos-go-wallet/wallet"
	_ "modernc.org/sqlite"

	storageTypes "github.com/jackalLabs/canine-chain/v3/x/storage/types"
)

func buyStorage(w *wallet.Wallet) {
	buyStorageMsg := storageTypes.NewMsgBuyStorage(w.AccAddress(), w.AccAddress(), 60, 3_000_000_000, "ujkl")
	data := types.NewTransactionData(
		buyStorageMsg,
	).WithGasAuto().WithFeeAuto()

	res, err := w.BroadcastTxCommit(data)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(res.RawLog)
}

func checkAndBuyStorage(w *wallet.Wallet) {
	cl := storageTypes.NewQueryClient(w.Client.GRPCConn)
	res, err := cl.StoragePaymentInfo(context.Background(), &storageTypes.QueryStoragePaymentInfo{Address: w.AccAddress()})
	if err != nil {
		buyStorage(w)
		return
	}
	if res.StoragePaymentInfo.End.Before(time.Now().AddDate(0, 0, 5)) {
		buyStorage(w)
		return
	}
}

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Failed to load the env vars: %v\n", err)
	}

	seed := os.Getenv("JACKAL_SEED")
	w, err := wallet2.CreateWallet(seed, "m/44'/118'/0'/0/0", types.ChainConfig{
		Bech32Prefix:  "jkl",
		RPCAddr:       "https://jackal-testnet-v4-rpc.brocha.in:443",
		GRPCAddr:      "jkl.grpc.t2.stavr.tech:5913",
		GasPrice:      "0.02ujkl",
		GasAdjustment: 1.5,
	})
	if err != nil {
		fmt.Println("failed to create wallet")
		panic(err)
	}

	checkAndBuyStorage(w)

	js := server.NewJackalStore(w)

	da := server.NewDAServer("0.0.0.0", 3100, js, true)

	err = da.Start()
	if err != nil {
		fmt.Println("failed to start server")
		panic(err)
	}
	fmt.Println("closing server...")
}
