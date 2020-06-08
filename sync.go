package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/jarcoal/httpmock"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

var ()

type EthereumNode struct {
	URI       string `gorm:"primary_key"`
	Network   string
	Provider  string
	UpdatedAt time.Time
}

func init() {

}

func main() {
	var providerURL = os.Getenv("PROVIDER_URL")
	flag.StringVar(&providerURL, "providerURL", providerURL, "Provider URL")
	var databaseURL = os.Getenv("DATABASE_URL")
	flag.StringVar(&databaseURL, "databaseURL", databaseURL, "Database URL")
	var network = os.Getenv("NETWORK")
	flag.StringVar(&network, "network", network, "Ethereum network name")
	var provider = os.Getenv("PROVIDER")
	flag.StringVar(&provider, "provider", provider, "Ethereum provider")

	enableMocks(providerURL)
	defer httpmock.DeactivateAndReset()

	fmt.Println("Connecting to DB: ", databaseURL)
	db, err := gorm.Open("postgres", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Set to `true` and GORM will print out all DB queries.
	db.LogMode(false)

	// Automatically create the "ethereum_nodes" table based on the EthereumNode
	// model.
	db.AutoMigrate(&EthereumNode{})

	fmt.Println("Connecting to Ethreum Node: ", providerURL)
	client, clientErr := rpc.Dial(providerURL)
	if clientErr != nil {
		fmt.Println("Error connecting: ", clientErr)
		return
	}
	defer client.Close()

	var t = time.Now()
	updateNodes(t, client, db, network, provider)

	ticker := time.NewTicker(time.Second * 60)
	go func() {
		for t := range ticker.C {
			updateNodes(t, client, db, network, provider)
		}
	}()

	// Block until termination signal is received
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-osSignals

	fmt.Println("Done!")
}

func updateNodes(t time.Time, client *rpc.Client, db *gorm.DB, network string, provider string) {
	dumpEnode(t, client, db, network, provider)
	readOtherEnodes(t, client, db, network, provider)
}

func dumpEnode(t time.Time, client *rpc.Client, db *gorm.DB, network string, provider string) {
	ctx := context.Background()
	var raw json.RawMessage
	if err := client.CallContext(ctx, &raw, "parity_enode"); err != nil {
		fmt.Println("Error connecting: ", err)
		return
	}

	var enode string
	if err := json.Unmarshal(raw, &enode); err != nil {
		fmt.Println("Error reading enode: ", err)
		return
	}

	var existingNode = EthereumNode{URI: enode}
	var newNode = EthereumNode{URI: enode, Network: network, Provider: provider, UpdatedAt: t}
	if dbc := db.Where(&existingNode).Assign("updated_at", t).FirstOrCreate(&newNode); dbc.Error != nil {
		fmt.Println("Error saving to DB: ", dbc.Error)
	}
}

func readOtherEnodes(t time.Time, client *rpc.Client, db *gorm.DB, network string, provider string) {

}

func enableMocks(providerURL string) {
	httpmock.Activate()

	httpmock.RegisterResponder("POST", providerURL,
		func(req *http.Request) (*http.Response, error) {
			request := make(map[string]interface{})

			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return httpmock.NewStringResponse(400, ""), nil
			}

			request["result"] = "enode://050929adcfe47dbe0b002cb7ef2bf91ca74f77c4e0f68730e39e717f1ce38908542369ae017148bee4e0d968340885e2ad5adea4acd19c95055080a4b625df6a@172.17.0.1:30303"

			resp, err := httpmock.NewJsonResponse(200, request)
			if err != nil {
				return httpmock.NewStringResponse(500, ""), nil
			}
			return resp, nil
		},
	)
}
