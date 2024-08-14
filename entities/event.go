package entities

import (
	// "errors"

	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/copier"
	"github.com/mlayerprotocol/go-mlayer/common/encoder"
	"github.com/mlayerprotocol/go-mlayer/common/utils"
	"github.com/mlayerprotocol/go-mlayer/configs"
	"github.com/mlayerprotocol/go-mlayer/internal/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var synced = "sync"

type EntityModel string

const (
	AuthModel         EntityModel = "auth"
	TopicModel        EntityModel = "top"
	SubscriptionModel EntityModel = "sub"
	MessageModel      EntityModel = "msg"
	SubnetModel       EntityModel = "snet"
	WalletModel       EntityModel = "wal"
)

/*
*
Event paths define the unique path to an event and its relation to the entitie

*
*/
type EntityPath struct {
	Model     EntityModel      `json:"mod"`
	Hash      string          `json:"h"`
	Validator PublicKeyString `json:"val"`
}

type EventPath struct {
	EntityPath
}

func (e *EntityPath) ToString() string {
	if e == nil || e.Hash == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", e.Validator, e.Model, e.Hash)
}

func NewEntityPath(validator PublicKeyString, model EntityModel, hash string) *EntityPath {
	return &EntityPath{Model: model, Hash: hash, Validator: validator}
}
func NewEventPath(validator PublicKeyString, model EntityModel, hash string) *EventPath {
	return &EventPath{EntityPath{Model: model, Hash: hash, Validator: validator}}
}

func (e *EntityPath) MsgPack() ([]byte) {
	b, _ := encoder.MsgPackStruct(e)
	return b
}

func UnpackEntityPath(b []byte) (*EntityPath, error) {
	var p EntityPath
	err := encoder.MsgPackUnpackStruct(b, &p)
	return &p, err
}
func UnpackEventPath(b []byte) (*EventPath, error) {
	var p EventPath
	err := encoder.MsgPackUnpackStruct(b, &p.EntityPath)
	return &p, err
}

func EntityPathFromString(path string) *EntityPath {
	parts := strings.Split(path, "/")
	// assoc, err := strconv.Atoi(parts[0])
	// if err != nil {
	// 	return nil, err
	// }
	switch len(parts) {
	case 0:
		return &EntityPath{}
	case 1:
		return &EntityPath{
			//Relationship: EventAssoc(assoc),
			Model: EntityModel(""),
			Hash:  parts[0],
		}
	case 2:
		return &EntityPath{
			//Relationship: EventAssoc(assoc),
			Model:     EntityModel(""),
			Hash:      parts[1],
			Validator: PublicKeyString(parts[0]),
		}
	default:
		return &EntityPath{
			Validator: PublicKeyString(parts[0]),
			Model:     EntityModel(parts[1]),
			Hash:      parts[2],
		}
	}
}
func EventPathFromString(path string) *EventPath {
	b := EntityPathFromString(path)
	return &EventPath{EntityPath: *b}
}
func (eP EntityPath) GormDataType() string {
	return "varchar"
}
func (eP EntityPath) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {

	asString := eP.ToString()
	return clause.Expr{
		SQL:                "?",
		Vars:               []interface{}{asString},
		WithoutParentheses: false,
	}
}

func (sD *EntityPath) Scan(value interface{}) error {
	data, ok := value.(string)
	if !ok {
		return errors.New(fmt.Sprint("Value not instance of string:", value))
	}

	*sD = *EntityPathFromString(data)
	return nil
}

func (sD *EntityPath) Value() (driver.Value, error) {
	return sD.ToString(), nil
}

type EventInterface interface {
	EncodeBytes() ([]byte, error)
	GetValidator() PublicKeyString
	GetSignature() string
	ValidateData(config *configs.MainConfiguration)  (authState any, err error)
}

type Event struct {
	// Primary
	ID string `gorm:"primaryKey;type:uuid;not null" json:"id,omitempty"`

	Payload           ClientPayload `json:"pld" gorm:"serializer:json" msgpack:",noinline"`
	Nonce             string        `json:"nonce" gorm:"type:varchar(80);default:null" msgpack:",noinline"`
	Timestamp         uint64        `json:"ts"`
	EventType         uint16        `json:"t"`
	Associations      []string      `json:"assoc" gorm:"type:text[]"`
	PreviousEventHash EventPath     `json:"preE" gorm:"type:varchar;default:null"`
	AuthEventHash     EventPath     `json:"authE" gorm:"type:varchar;default:null"`
	PayloadHash       string        `json:"pH" gorm:"type:char(64);unique"`
	// StateHash string `json:"sh"`
	// Secondary
	Error       string          `json:"err"`
	Hash        string          `json:"h" gorm:"type:char(64)"`
	Signature   string          `json:"sig"`
	Broadcasted bool            `json:"br"`
	BlockNumber uint64          `json:"blk"`
	Cycle   	uint64			`json:"cy"`
	Epoch		uint64			`json:"ep"`
	IsValid     bool            `json:"isVal" gorm:"default:false"`
	Synced      bool            `json:"sync" gorm:"default:false"`
	Validator   PublicKeyString `json:"val"`
	Subnet   	string			`json:"snet"`

	Total int `json:"total"`
}

func (d *Event) BeforeCreate(tx *gorm.DB) (err error) {
	if d.ID == "" {
		hash, _ := d.GetHash()
		u, err := uuid.FromBytes(hash[:16])
		if err != nil {
			return err
		}

		d.ID = u.String()
	}
	if d.Payload.Nonce > 0 {
		d.Nonce = fmt.Sprintf("%s:%d", string(d.Payload.Account), d.Payload.Nonce)
	}
	return nil
}

func (e *Event) Key() string {
	return fmt.Sprintf("/%s", e.Hash)
}

func (e *Event) ToJSON() []byte {
	m, err := json.Marshal(e)
	if err != nil {
		logger.Errorf("Unable to parse event to []byte")
	}
	return m
}

func (e *Event) MsgPack() []byte {
	b, _ := encoder.MsgPackStruct(e)
	return b
}
func (e *Event) GetDataModelType() EntityModel {
	return GetModel(e.Payload.Data)
}

func GetModel(ent any) EntityModel {
	var model EntityModel
	switch val := ent.(type) {
		case Subnet:
			logger.Debug(val)
			model = SubnetModel
		case Authorization:
			model = AuthModel
		case Topic:
			model = TopicModel
		case Subscription:
			model = SubscriptionModel
		case Message:
			model = MessageModel
	}
	return model
}

func (e *Event) GetPath() *EventPath {
	return NewEventPath(e.Validator, e.GetDataModelType(), e.Hash)
}

func UnpackEvent(b []byte, model EntityModel) (*Event, error) {
	// e.Payload = payload
	e := Event{}
	if err := encoder.MsgPackUnpackStruct(b, &e); err != nil {
		return nil, err
	}
	c, err := json.Marshal(e.Payload)
	if err != nil {
		return nil, err
	}

	pl := ClientPayload{}
	
	err = json.Unmarshal(c, &pl)
	
	if err != nil {
		logger.Errorf("UnmarshalError:: %o", err)
	}
	
	dBytes, err := json.Marshal(pl.Data)
	
	switch model {
	case AuthModel:
		r := Authorization{}
		json.Unmarshal(dBytes, &r)
		pl.Data = r
	case SubnetModel:
		r := Subnet{}
		json.Unmarshal(dBytes, &r)
		pl.Data = r
	case TopicModel:
		r := Topic{}
		json.Unmarshal(dBytes, &r)
		logger.Infof("PAYLOADDDDD %v", r)
		pl.Data = r
	case SubscriptionModel:
		r := Subscription{}
		json.Unmarshal(dBytes, &r)
		pl.Data = r
	case MessageModel:
		r := Message{}
		json.Unmarshal(dBytes, &r)
		pl.Data = r
	}
	
	// json.Unmarshal(dBytes, &pl.Data)
	// pl.Data, err = UnpackToEntity(dBytes, model)
	_, err2 := (&pl).EncodeBytes()
	if err2 != nil {
		logger.Errorf("EncodeBytesError:: %o", err)
	}
	copier.Copy(e.Payload, &pl)
	newEvent := Event{
		Payload: pl,
	}
	copier.Copy(&e.Payload, &newEvent.Payload)

	return &e, err
}



func EventFromJSON(b []byte) (Event, error) {
	var e Event
	// if err := json.Unmarshal(b, &message); err != nil {
	// 	panic(err)
	// }
	err := json.Unmarshal(b, &e)
	return e, err
}

func (e Event) GetHash() ([]byte, error) {
	b, err := e.EncodeBytes()
	if err != nil {
		return []byte(""), err
	}
	return crypto.Sha256(b), nil
}

func (e Event) ToString() string {
	values := []string{}
	d, _ := json.Marshal(e.Payload)
	values = append(values, fmt.Sprintf("%d", d))
	values = append(values, e.ID)
	values = append(values, fmt.Sprintf("%d", e.BlockNumber))
	values = append(values, fmt.Sprintf("%d", e.EventType))
	values = append(values, strings.Join(e.Associations, ","))
	values = append(values, e.PreviousEventHash.ToString())
	values = append(values, e.AuthEventHash.ToString())
	values = append(values, fmt.Sprintf("%d", e.Timestamp))
	return strings.Join(values, "")
}

func (e Event) EncodeBytes() ([]byte, error) {

	d, err := e.Payload.EncodeBytes()
	if err != nil {
		return []byte(""), err
	}
	return encoder.EncodeBytes(
		encoder.EncoderParam{Type: encoder.ByteEncoderDataType, Value: d},
		encoder.EncoderParam{Type: encoder.HexEncoderDataType, Value: strings.Join(e.Associations, "")},
		encoder.EncoderParam{Type: encoder.HexEncoderDataType, Value: e.AuthEventHash.Hash},
		encoder.EncoderParam{Type: encoder.IntEncoderDataType, Value: e.BlockNumber},
		encoder.EncoderParam{Type: encoder.IntEncoderDataType, Value: e.Cycle},
		encoder.EncoderParam{Type: encoder.IntEncoderDataType, Value: e.Epoch},
		encoder.EncoderParam{Type: encoder.IntEncoderDataType, Value: e.EventType},
		encoder.EncoderParam{Type: encoder.HexEncoderDataType, Value: e.PreviousEventHash.Hash},
		encoder.EncoderParam{Type: encoder.ByteEncoderDataType, Value: utils.UuidToBytes(e.Subnet)},
		encoder.EncoderParam{Type: encoder.IntEncoderDataType, Value: e.Timestamp},
	)
}

func (e Event) GetValidator() PublicKeyString {
	return e.Validator
}
func (e Event) GetSignature() string {
	return e.Signature
}


func GetEventEntityFromModel(eventType EntityModel) *Event {
	// cfg, _ := (*ctx).Value(constants.ConfigKey).(*configs.MainConfiguration)

	// check if client payload is valid
	// if err := payload.Validate(PublicKeyString(cfg.NetworkPublicKey)); err != nil {
	// 	return  err
	// }

	//Perfom checks base on event types
	event := &Event{Payload: ClientPayload{}}
	switch eventType {
	case AuthModel:
		event.Payload.Data = Authorization{}

	case TopicModel:
		event.Payload.Data = Topic{}

	case SubscriptionModel:
		event.Payload.Data = Subscription{}

	case MessageModel:
		event.Payload.Data = Message{}

	case SubnetModel:
		event.Payload.Data = Subnet{}

	case WalletModel:
		event.Payload.Data = Wallet{}
	}

	return event

}