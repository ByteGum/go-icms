/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"sync"

	"net"
	"net/http"
	"net/rpc"

	// "net/rpc/jsonrpc"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-datastore"
	"github.com/mlayerprotocol/go-mlayer/configs"
	"github.com/mlayerprotocol/go-mlayer/entities"
	dsquery "github.com/mlayerprotocol/go-mlayer/internal/ds/query"
	dsstores "github.com/mlayerprotocol/go-mlayer/internal/ds/stores"
	"github.com/multiformats/go-multiaddr"
	"github.com/quic-go/quic-go"

	// "github.com/mlayerprotocol/go-mlayer/entities"
	"github.com/mlayerprotocol/go-mlayer/common/apperror"
	"github.com/mlayerprotocol/go-mlayer/common/constants"
	"github.com/mlayerprotocol/go-mlayer/common/utils"
	"github.com/mlayerprotocol/go-mlayer/internal/chain"
	"github.com/mlayerprotocol/go-mlayer/internal/channelpool"
	"github.com/mlayerprotocol/go-mlayer/internal/crypto"
	"github.com/mlayerprotocol/go-mlayer/pkg/core/ds"
	p2p "github.com/mlayerprotocol/go-mlayer/pkg/core/p2p"
	"github.com/mlayerprotocol/go-mlayer/pkg/core/rest"
	rpcServer "github.com/mlayerprotocol/go-mlayer/pkg/core/rpc"
	ws "github.com/mlayerprotocol/go-mlayer/pkg/core/ws"
	"github.com/mlayerprotocol/go-mlayer/pkg/log"
)

// var Nodes = []*websocket.Conn{}

// var upgrader = websocket.Upgrader{
// 	ReadBufferSize:  1024,
// 	WriteBufferSize: 1024,
// 	CheckOrigin:     func(r *http.Request) bool { return true },
// } // use default options

var logger = &log.Logger
var syncedBlockMutex sync.Mutex

// var wsClients =  make(map[string]map[*websocket.Conn]*entities.ClientWsSubscription)
// var subscribedWsClientIndex =  make(map[string]map[*websocket.Conn][]int)
// var wsClients =  make(map[string][]*websocket.Conn)

var wsClients = entities.NewWsClientLog()

func Start(mainCtx *context.Context) {
	time.Sleep(1 * time.Second)
	logger.Println("Starting network...")
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, ok := (*mainCtx).Value(constants.ConfigKey).(*configs.MainConfiguration)
	if !ok {
		panic(apperror.Internal("Unable to load main config"))
	}

	connectedSubscribers := make(map[string]map[string][]interface{})

	// incomingEventsC := make(chan types.Log)

	var wg sync.WaitGroup

	ctx, stores := dsstores.InitStores(mainCtx)

	for _, store := range stores {
		defer store.Close()
	}

	eventCountStore := ds.New(&ctx, string(constants.EventCountStore))
	defer eventCountStore.Close()
	ctx = context.WithValue(ctx, constants.EventCountStore, eventCountStore)

	// ctx = context.WithValue(ctx, constants.NewTopicSubscriptionStore, newTopicSubscriptionStore)

	newClientPayloadStore := ds.New(&ctx, string(constants.NewClientPayloadStore))
	ctx = context.WithValue(ctx, constants.NewClientPayloadStore, newClientPayloadStore)

	ctx = context.WithValue(ctx, constants.ConnectedSubscribersMap, connectedSubscribers)

	ctx = context.WithValue(ctx, constants.WSClientLogId, &wsClients)
	eventProcessor := NewEventProcessor(&ctx)

	// defer func () {
	// 	if chain.NetworkInfo.Synced && SystemStore != nil && !SystemStore.DB.IsClosed() {
	// 		lastBlockKey :=  ds.Key(ds.SyncedBlockKey)
	// 		SystemStore.Set(ctx, lastBlockKey, chain.NetworkInfo.CurrentBlock.Bytes(), true)
	// 	}
	// }()

	if err := loadChainInfo(cfg); err != nil {
		logger.Fatal(err)
	}

	defer wg.Wait()

	//  wg.Add(1)
	// go func() {
	// 	defer wg.Done()
	// 	client.ListenForNewAuthEventFromPubSub(&ctx)
	// }()
	//  wg.Add(1)
	//  go func() {
	// 	defer wg.Done()
	// 	// go client.ListenForNewTopicEventFromPubSub(&ctx)
	//  }()

	// distribute message to event listeners and topic subscribers
	// wg.Add(1)
	// go func() {
	// 	_, cancel := context.WithCancel(context.Background())
	// 	defer cancel()
	// 	defer wg.Done()
	// 	for {
	// 		select {
	// 		case inEvent, ok := <-channelpool.IncomingMessageEvent_P2P_D_C:
	// 			if !ok {
	// 				logger.Errorf("Incoming Message channel closed. Please restart server to try or adjust buffer size in config")
	// 				wg.Done()
	// 				return
	// 			}
	// 			// VALIDATE, STORE AND DISTRIBUTE
	// 			go func() {
	// 				inMessage := inEvent.Payload.Data.(entities.Message)
	// 				logger.Debugf("Received new message %s\n", inMessage.DataHash)
	// 				// validMessagesStore.Set(ctx, db.Key(inMessage.Key()), inMessage.MsgPack(), false)
	// 				_reciever := inMessage.Receiver
	// 				_recievers := strings.Split(string(_reciever), ":")
	// 				_currentTopic := connectedSubscribers[_recievers[1]]
	// 				logger.Debug("connectedSubscribers : ", connectedSubscribers, "---", _reciever)
	// 				logger.Debug("_currentTopic : ", _currentTopic, "/n")
	// 				for _, signerConn := range _currentTopic {
	// 					for i := 0; i < len(signerConn); i++ {
	// 						signerConn[i].(*websocket.Conn).WriteMessage(1, inMessage.MsgPack())
	// 					}
	// 				}
	// 			}()

	// 		}

	// 	}
	// }()
	wg.Add(1)
	go func() {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer wg.Done()
		var countDuration = 1 * time.Second
		ticker := time.NewTicker(countDuration)
		defer ticker.Stop()
		
		for range ticker.C {
				readDone := make(chan bool)
				count := map[string]uint64{}
				cyleEvent := map[string]string{}
				// count[entities.NetworkCounterKey(nil)] = 0
				
				go func() {
					for {
						select {
						case event := <-channelpool.EventCounterChannel:
							// fmt.Println("Received event:", event)
							count[entities.NetworkCounterKey(nil)] ++
							
							count[entities.CycleCounterKey(event.Cycle, &event.Validator, utils.FalsePtr(), nil)] ++
							subnet := event.Subnet
							if len(subnet) == 0 {
								subnet = event.Payload.Subnet
							}
							if len(subnet) > 0 {
								// keys = append(keys, datastore.NewKey(entities.CycleCounterKey(cycle, &validator, utils.FalsePtr(), &subnet)))
								count[entities.NetworkCounterKey(&subnet)] ++
							} else {
								count[entities.CycleSubnetKey(event.Cycle, subnet)] ++ 
							}
							for _, keyString := range entities.GetBlockStatsKeys(event) {
								if keyString == entities.RecentEventKey(event.Cycle) {
									cyleEvent[keyString] = event.ID
									continue
								}
								count[keyString] ++
							}

						case <-time.After(countDuration): // Exit the reader after 1s
							
							if len(count) > 0 || len(cyleEvent) > 0  {
								txn, err := dsquery.InitTx(dsstores.NetworkStatsStore, nil)
								if err != nil {
									logger.Error("NetworkStateStoreError: %v", err)
								}
								for k, v := range count {
									if v > 0 {
										dsquery.IncrementCounterByKey(k, v, &txn)
									}
								}
								for k, v := range cyleEvent {
									txn.Put(context.Background(), datastore.NewKey(k), []byte(v))
								}
								err = txn.Commit(context.Background())
								if err != nil {
									logger.Errorf("CounterCommitError %v", err)
								}
							}
							
							readDone <- true
							return
						}
					}
				}()
				<-readDone
		}
	}()

	wg.Add(1)
	go func() {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer wg.Done()
		
		chain.NetworkInfo.SyncedValidators = map[string]multiaddr.Multiaddr{}
		// ticker := time.NewTicker(500 * time.Millisecond)
		// defer ticker.Stop()
			// for range ticker.C {
				// go func() {
					for eventPtr := range channelpool.EventProcessorChannel {
						event := eventPtr
						//modelType := event.GetDataModelType()
						
						eventProcessor.HandleEvent(event)

						
					// 		logger.Debugf("StartedProcessingEvent \"%s\" in Subnet: %s", event.ID, event.Subnet)
					// 		cfg, ok := (*mainCtx).Value(constants.ConfigKey).(*configs.MainConfiguration)
					// 		if !ok {
					// 			logger.Errorf("unable to get config from context")
					// 			return
					// 		}
					// 		if event.Validator != entities.PublicKeyString(hex.EncodeToString(cfg.PublicKeyEDD)) {
					// 			isValidator, err := chain.NetworkInfo.IsValidator(string(event.Validator))
					// 			if err != nil {
					// 				logger.Error(err)
					// 				return
					// 			}
					// 			if !isValidator {
					// 				logger.Error(fmt.Errorf("not signed by a validator"))
					// 				return
					// 			}
					// 			event.Broadcasted = true
					// 			service.SaveEvent(modelType, entities.Event{}, event, nil, nil)
					// 		}
			
					// 		// service.SaveEvent(modelType, entities.Event{}, event, nil, nil)
					// 		switch modelType {
					// 			case entities.SubnetModel:
					// 				service.HandleNewPubSubSubnetEvent(event, &ctx)
					// 			case entities.AuthModel:
					// 				service.HandleNewPubSubAuthEvent(event, &ctx)
					// 			case entities.TopicModel:
					// 				service.HandleNewPubSubTopicEvent(event, &ctx)
					// 			case entities.SubscriptionModel:
					// 				service.HandleNewPubSubSubscriptionEvent(event, &ctx)
					// 			case entities.MessageModel:
					// 				service.HandleNewPubSubMessageEvent(event, &ctx)
					// 		}
					// 		go func() {
					// 			syncedBlockMutex.Lock()
					// 			defer syncedBlockMutex.Unlock()
					// 			if chain.NetworkInfo.Synced {
					// 				lastSynced, err := ds.GetLastSyncedBlock(mainCtx)
					// 				eventBlock := new(big.Int).SetUint64(event.BlockNumber)
					// 				if err == nil && lastSynced.Cmp(eventBlock) == -1 {
					// 					ds.SetLastSyncedBlock(mainCtx, eventBlock)
					// 				}
					// 			}
					// 		}()
						
			
					// }
				// }()
			 }
		
	}()

	// load network params
	wg.Add(1)
	go func() {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer wg.Done()
		// time.Sleep(1 * time.Minute)
		for {
			if err := loadChainInfo(cfg); err != nil {
				logger.Error("loadChainInfoError:", err)
				// time.Sleep(1 * time.Second)
				panic(err)
				// continue
			}
			time.Sleep(1 * time.Second)
		}
	}()

	// test function
	if cfg.TestMode {
		wg.Add(1)
		go func() {
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer wg.Done()
			time.Sleep(5 * time.Second)
			// p2p.SyncNode(cfg, "154.12.228.25:9533", "57ba26ca619898bd6fa73c859918e7c9272d5cc7f7226d97ed5bec2d2787973b")
			certPayload := p2p.NewP2pPayload(cfg, p2p.P2pActionGetCert, []byte{'0'})
			// err := certPayload.Sign(cfg.PrivateKeyEDD)
			// if err != nil {
			// 	logger.Error(err)
			// }
			addr, err := multiaddr.NewMultiaddr("/ip4/154.12.228.25/udp/5002/quic-v1/p2p/12D3KooWFipGipTgu1XxtqpV1wUXcosTjK351Yip7Nj32npo68in")
			if err != nil {
				logger.Error("NewMultiaddr:", err)
			}
			certResponse, err := certPayload.SendP2pRequestToAddress(cfg.PrivateKeyEDD, addr, p2p.DataRequest)
			if err != nil {
				logger.Error("SendP2pRequestToAddress", err)
			}
			if certResponse != nil {
				logger.Debugf("RESPONSEEEEE: %d", certResponse.Action)
			}
		}()
	}

	wg.Add(1)
	// start the REST server
	go func() {
		for {
			subscription, ok := <-channelpool.ClientWsSubscriptionChannel
			if !ok {
				logger.Errorf("Client WS subscription channel closed")
				continue
			}

			wsClients.RegisterClient(subscription)
		}

	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// defer func() {
		// if err := recover(); err != nil {
		// 	wg.Done()
		// 	errc <- fmt.Errorf("P2P error: %g", err)
		// }
		// }()

		go p2p.ProcessEventsReceivedFromOtherNodes(entities.SubnetModel, &entities.SubnetPubSub, &ctx)
		go p2p.ProcessEventsReceivedFromOtherNodes(entities.AuthModel, &entities.AuthorizationPubSub, &ctx)
		go p2p.ProcessEventsReceivedFromOtherNodes(entities.TopicModel, &entities.TopicPubSub, &ctx)
		go p2p.ProcessEventsReceivedFromOtherNodes(entities.WalletModel, &entities.WalletPubSub, &ctx)
		go p2p.ProcessEventsReceivedFromOtherNodes(entities.SubscriptionModel, &entities.SubscriptionPubSub, &ctx)
		go p2p.ProcessEventsReceivedFromOtherNodes(entities.MessageModel, &entities.MessagePubSub, &ctx)

		p2p.Run(&ctx)
		// if err != nil {
		// 	wg.Done()
		// 	panic(err)
		// }
	}()

	if cfg.Validator {
		wg.Add(1)
		go func() {
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer wg.Done()
			TrackReward(&ctx)
		}()
		wg.Add(1)
		go func() {
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer wg.Done()
			ProcessPendingClaims(&ctx)
		}()

	}

	if cfg.Validator || cfg.BootstrapNode {
		wg.Add(1)
		go func() {
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer wg.Done()
			// for {
			// 	if chain.NetworkInfo.Synced {
			// 		if err := ArchiveBlocks(&ctx); err != nil {
			// 			logger.Error(err)
			// 			time.Sleep(10 * time.Second)
			// 			continue
			// 		}
			// 	}
			// 	time.Sleep(10 * time.Second)
			// }
		}()
	}

	// wg.Add(1)
	// go func() {
	// 	_, cancel := context.withCancel(context.Background(), time.Second)
	// 	defer cancel()
	// 	defer wg.Done()
	// 	_, client, contractAddress, err := evm.StakeContract(cfg.EVMRPCWss, cfg.StakeContract)
	// 	if err != nil {
	// 		logger.Fatal(err, cfg.EVMRPCWss, cfg.StakeContract)
	// 	}
	// 	query := ethereum.FilterQuery{
	// 		// FromBlock: big.NewInt(23506010),
	// 		// ToBlock:   big.NewInt(23506110),

	// 		Addresses: []common.Address{contractAddress},
	// 	}

	// 	// logs, err := client.FilterLogs(context.Background(), query)
	// 	// if err != nil {
	// 	// 	logger.Fatal(err)
	// 	// }
	// 	// parserEvent(logs[0], "StakeEvent")

	// 	// logger.Debugf("Past Events", logs)
	// 	// incomingEventsC

	// 	sub, err := client.SubscribeFilterLogs(context.Background(), query, incomingEventsC)
	// 	if err != nil {
	// 		logger.Fatal(err, "SubscribeFilterLogs")
	// 	}

	// 	for {
	// 		select {
	// 		case err := <-sub.Err():
	// 			logger.Fatal(err)
	// 		case vLog := <-incomingEventsC:
	// 			fmt.Println(vLog) // pointer to event log
	// 			parserEvent(vLog, "StakeEvent")
	// 		}
	// 	}

	// }()

	// start the RPC server
	if cfg.Validator {
		waitToSync()
		wg.Add(1)
		go func() {
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer wg.Done()
			rpc.Register(rpcServer.NewRpcService(&ctx))
			rpc.HandleHTTP()
			host := cfg.RPCHost
			if host == "" {
				host = "127.0.0.1"
			}
			listener, err := net.Listen("tcp", host+":"+cfg.RPCPort)
			if err != nil {
				logger.Fatal("RPC failed to listen on TCP port: ", err)
			}
			defer listener.Close()
			logger.Debugf("RPC server runing on: %+s", host+":"+cfg.RPCPort)
			go http.Serve(listener, nil)
			time.Sleep(time.Second)
			sendHttp := rpcServer.NewHttpService(&ctx)
			err = sendHttp.Start(cfg.RPCPort)
			if err != nil {
				logger.Fatal("Http error: ", err)
			}
		}()

		wg.Add(1)
		// starting quick server
		go func() {
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer wg.Done()
			if !cfg.Validator {
				return
			}
			// get the certificate from store

			cd := crypto.GetOrGenerateCert(&ctx)
			keyByte, _ := hex.DecodeString(cd.Key)
			certByte, _ := hex.DecodeString(cd.Cert)
			tlsConfig, err := crypto.GenerateTLSConfig(keyByte, certByte)
			if err != nil {
				logger.Fatal("QuicTLSError", err)
			}
			listener, err := quic.ListenAddr(cfg.QuicHost, tlsConfig, nil)
			if err != nil {
				logger.Fatal(err)
			}
			defer listener.Close()

			for {
				connection, err := listener.Accept(ctx)
				if err != nil {
					logger.Fatal(err)
				}
				go p2p.HandleQuicConnection(&ctx, cfg, connection)
			}
		}()

		// start the websocket server
		wg.Add(1)
		go func() {
			_, cancel := context.WithCancel(context.Background())
			logger.Debugf("Starting Websocket server on: %s", cfg.WSAddress)
			defer cancel()
			defer wg.Done()
			wss := ws.NewWsService(&ctx)
			logger.Debugf("WsAddress: %s\n", cfg.WSAddress)
			http.HandleFunc("/ws", wss.HandleConnection)

			logger.Fatal(http.ListenAndServe(cfg.WSAddress, nil))
		}()

		wg.Add(1)
		// start the REST server
		go func() {
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer wg.Done()
			rest := rest.NewRestService(&ctx)

			router := rest.Initialize()
			logger.Debugf("Starting REST api on: %s", cfg.RestAddress)
			err := router.Run(cfg.RestAddress)
			logger.Fatal(err)

		}()
	}
}

func loadChainInfo(cfg *configs.MainConfiguration) error {

	info, err := chain.Provider(cfg.ChainId).GetChainInfo()
	if err != nil {
		return fmt.Errorf("pkg/node/NodeInfo/GetChainInfo: %v", err)
	}
	if chain.NetworkInfo.ActiveValidatorLicenseCount != info.ValidatorActiveLicenseCount.Uint64() {
		// if chain.NetworkInfo.Validators == nil {
		chain.NetworkInfo.Validators = map[string]string{}
		// }
		page := big.NewInt(1)
		perPage := big.NewInt(100)
		for {

			validators, err := chain.Provider(cfg.ChainId).GetValidatorNodeOperators(page, perPage)
			if err != nil {
				logger.Errorf("pkg/node/NodeInfo/GetValidatorNodeOperators: %v", err)
				time.Sleep(10 * time.Second)
				continue
			}

			for _, val := range validators {
				pubKey := hex.EncodeToString(val.PublicKey)
				// chain.NetworkInfo.Validators[pubKey] = val.LicenseOwner
				chain.NetworkInfo.Validators[val.LicenseOwner] = "true"
				chain.NetworkInfo.Validators[fmt.Sprintf("secp/%s/edd", pubKey)] = hex.EncodeToString(val.EddKey[:])
				chain.NetworkInfo.Validators[fmt.Sprintf("secp/%s/addr", pubKey)] = val.LicenseOwner
				chain.NetworkInfo.Validators[fmt.Sprintf("edd/%s/secp", hex.EncodeToString(val.EddKey[:]))] = pubKey
				chain.NetworkInfo.Validators[fmt.Sprintf("edd/%s/addr", hex.EncodeToString(val.EddKey[:]))] = val.LicenseOwner
			}
			if len(validators) == 0 || big.NewInt(int64(len(validators))).Cmp(perPage) == -1 {
				break
			}
			page = new(big.Int).Add(page, big.NewInt(1))
		}
	}
	ownerAddress := chain.NetworkInfo.Validators[fmt.Sprintf("secp/%s/addr", cfg.PublicKeySECPHex)]
	if len(ownerAddress) > 0 {
		cfg.Validator = true
		cfg.OwnerAddress = common.HexToAddress(ownerAddress)
	} else {
		address, err := chain.Provider(cfg.ChainId).GetSentryLicenseOwnerAddress(cfg.PublicKeySECP)
		if err != nil {
			logger.Fatal(err)
		}
		cfg.OwnerAddress = common.BytesToAddress(address)
	}
	// if cfg.NoSync {
	chain.NetworkInfo.Synced = true
	// }
	chain.NetworkInfo.StartBlock = info.StartBlock
	chain.NetworkInfo.StartTime = info.StartTime
	chain.NetworkInfo.CurrentCycle = info.CurrentCycle
	chain.NetworkInfo.CurrentBlock = info.CurrentBlock
 	chain.NetworkInfo.CurrentEpoch = info.CurrentEpoch
	chain.NetworkInfo.ActiveValidatorLicenseCount = info.ValidatorActiveLicenseCount.Uint64()
	chain.NetworkInfo.ActiveSentryLicenseCount = info.SentryActiveLicenseCount.Uint64()

	return err
}

func waitToSync() {
	for {
		if !chain.NetworkInfo.Synced {
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}
}
