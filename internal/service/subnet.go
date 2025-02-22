package service

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/ipfs/go-datastore"
	"github.com/mlayerprotocol/go-mlayer/common/apperror"
	"github.com/mlayerprotocol/go-mlayer/common/constants"
	"github.com/mlayerprotocol/go-mlayer/common/encoder"
	"github.com/mlayerprotocol/go-mlayer/common/utils"
	"github.com/mlayerprotocol/go-mlayer/configs"
	"github.com/mlayerprotocol/go-mlayer/entities"
	"github.com/mlayerprotocol/go-mlayer/internal/crypto"
	dsquery "github.com/mlayerprotocol/go-mlayer/internal/ds/query"
	"github.com/mlayerprotocol/go-mlayer/internal/sql/models"
	query "github.com/mlayerprotocol/go-mlayer/internal/sql/query"
	"gorm.io/gorm"
	// query "github.com/mlayerprotocol/go-mlayer/internal/sql/query"
)

/*
Validate an agent authorization
*/
func ValidateSubnetData(clientPayload *entities.ClientPayload, chainID configs.ChainId) ( *models.SubnetState,error) {
	// check fields of Subnet
	
	var currentSubnetState *models.SubnetState
	subnet := clientPayload.Data.(entities.Subnet)
	agent := entities.AddressFromString(string(subnet.Agent))
	account := entities.AddressFromString(string(subnet.Account))
	
	if len(subnet.Agent) > 0 && subnet.ID != "" {
		
		// TODO Check that this agent is an admin of subnet. Return error if not
		priv := constants.AdminPriviledge
		
		// err := query.GetOne(models.AuthorizationState{Authorization: entities.Authorization{
		// 	Agent: agent.ToDeviceString(),
		// 	Subnet: subnet.ID,
		// 	Priviledge: &priv,
		// 	Account: account.ToString(),
		// }}, &auth)
		authorizations, err := dsquery.GetAccountAuthorizations( entities.Authorization{
			Agent: agent.ToDeviceString(),
			Subnet: subnet.ID,
			Account: account.ToString(),
		}, dsquery.DefaultQueryLimit, nil)
		if err != nil  {
			if  dsquery.IsErrorNotFound(err) {
				return nil,  apperror.Unauthorized("agent not authorized")
			}
			return nil,  apperror.Internal("internal database error")
		}
		authorized := false
		// var auth models.AuthorizationState
		for _, _auth := range authorizations {
			if *(_auth.Priviledge) == priv {
				authorized = true
			}
			// auth = models.AuthorizationState{Authorization: *_auth}
		}
		if !authorized {
			return nil,  apperror.Unauthorized("agent not authorized")
		}
		
	}

	// TODO if agent is specified, ensure agent is allowed to sign on behalf of Owner

	if len(subnet.Ref) > 64 {
		return nil, apperror.BadRequest("Subnet ref cannont be more than 64 characters")
	}
	if len(subnet.Ref) > 0 && !utils.IsAlphaNumericDot(subnet.Ref) {
		return nil, apperror.BadRequest("Ref can only include alpha-numerics, and .")
	}
	var valid bool
	// b, _ := subnet.EncodeBytes()
	msg, err := clientPayload.GetHash()
	if err != nil {
		return nil, err
	}
	action :=  "write_subnet"
	switch subnet.SignatureData.Type {
	case entities.EthereumPubKey:
		authMsg := fmt.Sprintf(constants.SignatureMessageString, action,  subnet.Ref, chainID, encoder.ToBase64Padded(msg))
		msgByte := crypto.EthMessage([]byte(authMsg))
		logger.Infof("AUTHMESSAGE %s", authMsg)

		valid = crypto.VerifySignatureECC(entities.AddressFromString(string(subnet.Account)).Addr, &msgByte, subnet.SignatureData.Signature)

	case entities.TendermintsSecp256k1PubKey:
		
		decodedSig, err := base64.StdEncoding.DecodeString(subnet.SignatureData.Signature)
		if err != nil {
			return nil, err
		}
		// account := entities.AddressFromString(string(subnet.Account))
		publicKeyBytes, err := base64.RawStdEncoding.DecodeString(subnet.SignatureData.PublicKey)

		if err != nil {
			return nil, err
		}
		authMsg := fmt.Sprintf(constants.SignatureMessageString, action, chainID, subnet.Ref, encoder.ToBase64Padded(msg))
		logger.Debug("MSG:: ", authMsg)
		valid, err = crypto.VerifySignatureAmino(encoder.ToBase64Padded([]byte(authMsg)), decodedSig, account.Addr, publicKeyBytes)
		if err != nil {
			return nil, err
		}

	}
	
	if !valid {
		return nil, apperror.Unauthorized("Invalid subnet data signature")
	}

	if subnet.ID != "" {
		// var  curSt  models.SubnetState
		// query.GetOne(models.SubnetState{Subnet: entities.Subnet{ID: subnet.ID}}, &curSt)
		snetS, err := dsquery.GetSubnetStateById(subnet.ID)
		if err != nil {
			if !dsquery.IsErrorNotFound(err) {
				return nil, err
			} else {
				return nil, nil
			}
		}
		currentSubnetState = &models.SubnetState{Subnet: *snetS}
	}
	// logger.Infof("IsValidSigner %v, subId: %s, currentstate: %v, error: %v", valid, subnet.ID, currentSubnetState, err)
	// logger.Infof("IsValidSigner %v, subId: %s, currentstate: %v, error: %v", valid, subnet.ID, currentSubnetState, err)
	return currentSubnetState, nil
}

func saveSubnetEvent(where entities.Event, createData *entities.Event, updateData *entities.Event, txn *datastore.Txn, tx *gorm.DB) (*entities.Event, error) {
	return SaveEvent(entities.SubnetModel, where, createData, updateData, txn)
 }


func HandleNewPubSubSubnetEvent(event *entities.Event, ctx *context.Context, ) error {

	cfg, ok := (*ctx).Value(constants.ConfigKey).(*configs.MainConfiguration)
	
	if !ok {
		panic("Unable to load config from context")
	}
	
	dataStates := dsquery.NewDataStates(cfg)
	dataStates.AddEvent(*event)
	
	data := event.Payload.Data.(entities.Subnet)
	data.Event = *event.GetPath()
	data.BlockNumber = event.BlockNumber
	data.Cycle = event.Cycle
	data.Epoch = event.Epoch
	data.EventSignature = event.Signature
	hash, err := data.GetHash()
	if err != nil {
		return err
	}
	data.Hash = hex.EncodeToString(hash)
	logger.Debugf("HandlingNewEvent: %s in subnet %s", data.ID, event.Payload.Subnet )
	var id string
	if len(data.ID) == 0 {
		id, _ = entities.GetId(data, data.ID)
	} else {
		id = data.ID
	}

	defer func () {
		stateUpdateError := dataStates.Commit(nil, nil, nil)
		if stateUpdateError != nil {
			
			panic(stateUpdateError)
		} else {
			go  OnFinishProcessingEvent(ctx, event,  &data)
			
			
			// go utils.WriteBytesToFile(filepath.Join(cfg.DataDir, "log.txt"), []byte("newMessage" + "\n"))
		}	
	}()
	
	var localState models.SubnetState
	
	 subnet, err := dsquery.GetSubnetStateById(id)
	 if err != nil && !dsquery.IsErrorNotFound(err){
		logger.Debugf("SubnetStateQueryError: %v", err)
		return err
	 }
	 if (subnet != nil ) {
	 	localState =  models.SubnetState{Subnet: *subnet}
	 }

	// if err != nil {
	// 	logger.Error(err)
	// }
	
	
	var localDataState *LocalDataState
	if localState.ID != "" {
		localDataState = &LocalDataState{
			ID: localState.ID,
			Hash: localState.ID,
			Event: &localState.Event,
			Timestamp: localState.Timestamp,
		}
	}
	// localDataState := utils.IfThenElse(localTopicState != nil, &LocalDataState{
	// 	ID: localTopicState.ID,
	// 	Hash: localTopicState.ID,
	// 	Event: &localTopicState.Event,
	// 	Timestamp: localTopicState.Timestamp,
	// }, nil)
	var stateEvent *entities.Event
	if localState.ID != "" {
		stateEvent, err = dsquery.GetEventFromPath(&localState.Event)
		if err != nil && err != query.ErrorNotFound && !dsquery.IsErrorNotFound(err) {
			logger.Debug(err)
		}
	}
	var localDataStateEvent *LocalDataStateEvent
	if stateEvent != nil {
		localDataStateEvent = &LocalDataStateEvent{
			ID: stateEvent.ID,
			Hash: stateEvent.Hash,
			Timestamp: stateEvent.Timestamp,
		}
	}

	eventData := PayloadData{Subnet: data.ID, localDataState: localDataState, localDataStateEvent:  localDataStateEvent}
	// tx := sql.SqlDb
	// defer func () {
	// 	if tx.Error != nil {
	// 		tx.Rollback()
	// 	} else {
	// 		tx.Commit()
	// 	}
	// }()
	// txn, err := stores.EventStore.NewTransaction(context.Background(), false) // true for read-write, false for read-only
	// if err != nil {
	// 	// either subnet does not exist or you are not uptodate
	// }
	// defer txn.Discard(context.Background())  
	previousEventUptoDate,  _, _, eventIsMoreRecent, err := ProcessEvent(event,  eventData, false, saveSubnetEvent, nil, nil, ctx, dataStates)
	if err != nil {
		logger.Debugf("Processing Error...: %v", err)
		return err
	}
	
		event.Subnet = id
		// err = dsquery.IncrementCounters(event.Cycle, event.Validator, event.Subnet, &txn)
		// if err != nil { 
		// 	return err
		// }
	
	logger.Debugf("Processing 2...: %v", previousEventUptoDate)
	if previousEventUptoDate {

		if event.Validator != entities.PublicKeyString(cfg.PublicKeyEDDHex) {
			_, err = ValidateSubnetData(&event.Payload, cfg.ChainId)
		}
		
		if err != nil {
			// update error and mark as synced
			// notify validator of error
			dataStates.AddEvent(entities.Event{ID: event.ID, Error: err.Error(), IsValid: utils.FalsePtr(), Synced:  utils.TruePtr()})
			
		} else {
			// TODO if event is older than our state, just save it and mark it as synced
			dataStates.AddEvent(entities.Event{ID: event.ID, IsValid:  utils.TruePtr(), Synced:  utils.TruePtr()})
		
			// savedEvent, err := saveSubnetEvent(entities.Event{ID: event.ID}, nil, &entities.Event{IsValid:  utils.TruePtr(), Subnet: event.Subnet, Synced:  utils.TruePtr()}, &txn, nil );
			// data.ID, _ = entities.GetId(data, id)
			
			if eventIsMoreRecent {
				// update state
					dataStates.AddCurrentState(entities.SubnetModel, id, data)
				
				// if err != nil {
				// 	// tx.Rollback()
				// 	logger.Errorf("SaveStateError %v", err)
				// 	return err
				// } else {
				// 	_, err = saveSubnetEvent(entities.Event{ID: event.ID}, nil, &entities.Event{IsValid: utils.TruePtr(), Synced:  utils.TruePtr()}, &txn, nil )
				// }
			} else {
				dataStates.AddHistoricState(entities.SubnetModel, data.ID, data.MsgPack())
			}
			go dsquery.UpdateAccountCounter(event.Payload.Account.ToString())
			// if err == nil {
			// 	if err = txn.Commit(context.Background()); err != nil {
			// 		logger.Errorf("ErorrSavingEvent: %v", err)
			// 		return err
			// 	}
			// 	go func ()  {
			// 		dsquery.UpdateAccountCounter(data.Account.ToString())
			// 		//event.Subnet = savedEvent.ID
			// 		dsquery.IncrementStats(event, nil)

			// 		OnFinishProcessingEvent(ctx, event, &models.SubnetState{
			// 				Subnet: data,
			// 			}, &savedEvent.ID)
			// 	}()
				
			// }
			
			
			
		}

}
return nil
}

// func UpdateSubnetFromPeer(subnetId string , cfg *configs.MainConfiguration, validator string) (*entities.Subnet, error) {
// 	_subnet := &entities.Subnet{}
// 	if validator == "" {
// 		validator = chain.NetworkInfo.GetRandomSyncedNode()
// 	}
// 	if len(validator) == 0 {
// 		return nil, apperror.NotFound("subnet not found")
// 	}
// 	subPath := entities.NewEntityPath(entities.PublicKeyString(validator), entities.SubnetModel, subnetId)
// 	pp, err := p2p.GetState(cfg, *subPath, nil, _subnet)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if len(pp.Event) < 2 {
// 		return nil, apperror.NotFound("subnet not found")
// 	}
// 	subnetEvent, err := entities.UnpackEvent(pp.Event, entities.SubnetModel)
// 	if err != nil {
// 		logger.Errorf("UnpackError: %v", err)
// 		return  nil, err
// 	}
// 	err = dsquery.CreateEvent(subnetEvent, nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	for _, snetData := range pp.States {
// 		_subnet, err := entities.UnpackSubnet(snetData)
// 		logger.Infof("FoundSubnet %v", _subnet)
// 		if err != nil {
// 			return  nil, apperror.NotFound("unable to retrieve subnet")
// 		}
// 			s, err := dsquery.CreateSubnetState(&_subnet, nil)
// 			logger.Infof("FoundSubnet 2 %v", _subnet)
// 			if err != nil {
// 				return  nil, apperror.NotFound("subnet not saved")
// 			}
// 			_subnet = *s;
		
// 	}
// 	return _subnet, nil
// }

