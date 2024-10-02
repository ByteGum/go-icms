package rest

import (
	// "errors"
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mlayerprotocol/go-mlayer/common/constants"
	"github.com/mlayerprotocol/go-mlayer/common/utils"
	"github.com/mlayerprotocol/go-mlayer/configs"
	"github.com/mlayerprotocol/go-mlayer/entities"
	"github.com/mlayerprotocol/go-mlayer/internal/sql/models"
	"github.com/mlayerprotocol/go-mlayer/internal/sql/query"
	"github.com/mlayerprotocol/go-mlayer/pkg/client"
	"github.com/mlayerprotocol/go-mlayer/pkg/log"
	"github.com/sirupsen/logrus"
)

var logger = &log.Logger


type Flag string

type RestService struct {
	Ctx                    *context.Context
	Cfg                    *configs.MainConfiguration
	// ClientHandshakeChannel *chan *entities.ClientHandshake
}

type RestResponse struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
}


func NewRestService(mainCtx *context.Context) *RestService {
	cfg, _ := (*mainCtx).Value(constants.ConfigKey).(*configs.MainConfiguration)
	// clientVerificationc, _ := (*mainCtx).Value(constants.ClientHandShackChId).(*chan *entities.ClientHandshake)
	return &RestService{
		Ctx:                    mainCtx,
		Cfg:                    cfg,
		//ClientHandshakeChannel: clientVerificationc,
	}
}
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST,HEAD,PATCH, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
func (p *RestService) Initialize() *gin.Engine {
	router := gin.New()
	if p.Cfg.LogLevel == "info" || p.Cfg.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	requestProcessor := client.NewClientRequestProcess(p.Ctx)
	// router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
    //     logger.Infof("[GIN]: %s - [%s] \"%s %s %s %d %s \"%s\" \"%s\"\n",
    //         param.ClientIP,
    //         param.TimeStamp.Format("02/Jan/2006:15:04:05 -0700"),
    //         param.Method,
    //         param.Path,
    //         param.Request.Proto,
    //         param.StatusCode,
    //         param.Latency,
    //         param.Request.UserAgent(),
    //         param.ErrorMessage,
    //     )
    //     return ""
    // }))
	

	router.Use(CORSMiddleware())
	

	// ping the api
	router.GET("/api/ping", func(c *gin.Context) {

		// Send a response back
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{}))
	})

	// get info about the node
	router.GET("/api/info", func(c *gin.Context) {
		info, err := requestProcessor.Process(client.GetNodeInfoRequest, nil, nil)
		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: info}))
	})


	router.GET("/api/authorizations", func(c *gin.Context) {

		b, parseError := utils.ParseQueryString(c)
		if parseError != nil {
			logger.Error(parseError)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
			return
		}

		var authEntity entities.Authorization

		json.Unmarshal(*b, &authEntity)
		logger.Debugf("authEntity %v", authEntity)
		auths, err := client.GetAccountAuthorizations(&authEntity)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: auths}))
	})

	router.PUT("/api/authorization", func(c *gin.Context) {
		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		authorization := entities.Authorization{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &authorization)
		if e != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: e.Error()}))
		}
		// Subnet.ID = id
		payload.Data = authorization

		logger.WithFields(logrus.Fields{"payload": string(payload.ToJSON())}).Debug("New auth payload from REST api")
		authEvent, err := client.CreateEvent(payload, p.Ctx)
		if err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		// Send a response back
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: map[string]any{
			"event": authEvent,
		}}))
	})

	router.POST("/api/topics", func(c *gin.Context) {
		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		logger.Debugf("Payload %v", payload)
		topic := entities.Topic{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &topic)
		if e != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: e.Error()}))
		}
		// topic.ID = id

		payload.Data = topic
		event, err := client.CreateEvent(payload, p.Ctx)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: map[string]any{
			"event": event,
		}}))
	})

	router.GET("/api/topics", func(c *gin.Context) {

		b, parseError := utils.ParseQueryString(c)
		if parseError != nil {
			logger.Error(parseError)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
			return
		}

		var topicPayload entities.Topic
		json.Unmarshal(*b, &topicPayload)

		logger.Debugf("Payload %v", topicPayload.Agent)

		topics, err := query.GetTopics(topicPayload)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: topics}))
	})

	router.GET("/api/topics/subscribers/:id", func(c *gin.Context) {
		id := c.Param("id")
		topic, err := client.GetSubscription(id)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: topic}))
	})

	router.GET("/api/topics/subscribers", func(c *gin.Context) {

		b, parseError := utils.ParseQueryString(c)
		if parseError != nil {
			logger.Error(parseError)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
			return
		}

		//
		var subPayload entities.Subscription
		json.Unmarshal(*b, &subPayload)

		status := c.Query("st")
		if status != "" {
			iStatus, parseError := strconv.Atoi(status)
			if parseError != nil {
				logger.Error(parseError)
				c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
				return
			}
			statusConst := constants.SubscriptionStatus(iStatus)
			subPayload.Status = &statusConst
		}

		// logger.Debugf("Payload %v", subPayload.Topic)

		subs, err := client.GetSubscriptions(subPayload)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		// logger.Debugf("subs %v", subs)

		// var payload entities.ClientPayload
		// if err := c.BindJSON(&payload); err != nil {
		// 	logger.Error(err)
		// 	c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
		// 	return
		// }
		// logger.Debugf("subs %v", subs)
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: subs}))
	})

	router.GET("/api/topics/:id/messages", func(c *gin.Context) {
		id := c.Param("id")
		messages, err := client.GetMessages(id)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: messages}))
	})

	router.GET("/api/topics/:id", func(c *gin.Context) {
		id := c.Param("id")
		topic, err := query.GetTopicById(id)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: topic}))
	})

	router.POST("/api/topics/subscribe", func(c *gin.Context) {
		// id := c.Param("id")

		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		logger.Debugf("Payload %v", payload.Data)
		subscription := entities.Subscription{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &subscription)
		if e != nil {
			logger.Errorf("UnmarshalError %v", e)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: e.Error()}))
			return
		}
		// subscription.ID = id
		payload.Data = subscription
		event, err := client.CreateEvent(payload, p.Ctx)

		if err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: event}))

	})

	router.PATCH("/api/topics/subscribers/approve", func(c *gin.Context) {
		id := c.Param("id")

		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		logger.Debugf("Payload %v", payload.Data)
		subscription := entities.Subscription{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &subscription)
		if e != nil {
			logger.Errorf("UnmarshalError %v", e)
		}
		subscription.ID = id
		payload.Data = subscription
		event, err := client.CreateEvent(payload, p.Ctx)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: event}))

	})

	router.PATCH("/api/topics/unsubscribe", func(c *gin.Context) {
		id := c.Param("id")

		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		logger.Debugf("Payload %v", payload.Data)
		subscription := entities.Subscription{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &subscription)
		if e != nil {
			logger.Errorf("UnmarshalError %v", e)
		}
		subscription.ID = id
		payload.Data = subscription
		event, err := client.CreateEvent(payload, p.Ctx)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: event}))

	})

	router.PATCH("/api/topics/ban", func(c *gin.Context) {
		id := c.Param("id")

		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		logger.Debugf("Payload %v", payload.Data)
		subscription := entities.Subscription{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &subscription)
		if e != nil {
			logger.Errorf("UnmarshalError %v", e)
		}
		subscription.ID = id
		payload.Data = subscription
		event, err := client.CreateEvent(payload, p.Ctx)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: event}))

	})

	router.PUT("/api/topics", func(c *gin.Context) {

		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		payload.EventType = uint16(constants.UpdateTopicEvent)
		logger.Debugf("Payload %v", payload.Data)
		topic := entities.Topic{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &topic)
		if e != nil {
			logger.Errorf("UnmarshalError %v", e)
		}
		// topic.Hash = id
		payload.Data = topic
		event, err := client.CreateEvent(payload, p.Ctx)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: map[string]any{
			"event": event,
		}}))
	})

	router.POST("/api/topics/messages", func(c *gin.Context) {
		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		logger.Debugf("Payload:::::: %v", payload.Data)
		message := entities.Message{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &message)
		if e != nil {
			logger.Errorf("Unmarshal Error %v", e)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: e.Error()}))
			return
		}
		// subscription.ID = id
		payload.Data = message
		event, err := client.CreateEvent(payload, p.Ctx)

		if err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: event}))

	})

	router.POST("/api/subscription/account", func(c *gin.Context) {

		_, parseError := utils.ParseQueryString(c)
		if parseError != nil {
			logger.Error(parseError)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
			return
		}

		subs := entities.Subscription{}
		// json.Unmarshal(*b, &subs)
		// rawQuery := c.Request.URL.Query()
		// err := c.ShouldBind(&subs)
		// if err != nil {
		// 	logger.Errorf("SUBSCR: %v",  err)
		// }
		logger.Debugf("SUBSCR: %s", c.Query("status"))
		// //
		// var payload entities.ClientPayload
		// json.Unmarshal(*b, &payload)

		// logger.Debugf("Payload %v", payload.Account)
		// subscriptions, err := client.GetAccountSubscriptions(payload)

		// if err != nil {
		// 	logger.Error(err)
		// 	c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
		// 	return
		// }
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: subs}))
		// c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: subscriptions}))
	})

	router.GET("/api/subscription/account", func(c *gin.Context) {

		b, parseError := utils.ParseQueryString(c)
		if parseError != nil {
			logger.Error(parseError)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
			return
		}

		//
		var payload entities.Subscription
		json.Unmarshal(*b, &payload)
		status := c.Query("status")
		if status != "" {
			iStatus, parseError := strconv.Atoi(status)
			if parseError != nil {
				logger.Error(parseError)
				c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
				return
			}
			statusConst := constants.SubscriptionStatus(iStatus)
			payload.Status = &statusConst
		}

		//

		subscriptions, err := client.GetAccountSubscriptionsV2(payload)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: subscriptions}))
	})

	router.GET("/api/sync", func(c *gin.Context) {
		b, parseError := utils.ParseQueryString(c)
		if parseError != nil {
			logger.Error(parseError)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
			return
		}
		var authEntity entities.Authorization
		var payload entities.ClientPayload
		json.Unmarshal(*b, &authEntity)
		json.Unmarshal(*b, &payload)

		syncResponse := entities.SyncResponse{}
		client.SyncAgent(&entities.SyncRequest{}, &entities.ClientPayload{})

		// if err != nil {
		// 	logger.Error(err)
		// 	c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
		// 	return
		// }
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: syncResponse}))
	})

	router.GET("/api/block-stats", func(c *gin.Context) {
		blockStats, err := client.GetBlockStats()
		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: blockStats}))
	})

	router.GET("/api/main-stats", func(c *gin.Context) {
		mainStats, err := client.GetMainStats(p.Cfg)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: mainStats}))
	})

	router.GET("/api/event-path/:hash/:type/:id", func(c *gin.Context) {
		hash := c.Param("hash")
		logger.Debug("hash", hash)
		typeParam := c.Param("type")
		typeParamInt := client.GetEventTypeFromModel(entities.EntityModel(typeParam))

		topic, err := client.GetEventByHash(hash, int(typeParamInt))

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: topic}))
	})

	router.GET("/api/event/:type/:id", func(c *gin.Context) {
		id := c.Param("id")
		typeParam := c.Param("type")
		typeParamInt, err := strconv.Atoi(typeParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		event, err := client.GetEvent(id, typeParamInt)
		if err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: event}))
	})

	router.POST("/api/subnets", func(c *gin.Context) {
		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		event, err := requestProcessor.Process(client.WriteSubnetRequest, nil, payload)
		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: map[string]any{
			"event": event,
		}}))
	})

	router.GET("/api/subnets", func(c *gin.Context) {
		b, parseError := utils.ParseQueryString(c)
		if parseError != nil {
			logger.Error(parseError)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
			return
		}

		var subnetState models.SubnetState

		json.Unmarshal(*b, &subnetState)

		subnets, err := client.GetSubscribedSubnets(subnetState)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: subnets}))
	})

	router.GET("/api/subnets/:id/by-account", func(c *gin.Context) {
		id := c.Param("id")
		messages, err := client.GetMessages(id)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: messages}))
	})

	router.POST("/api/wallets", func(c *gin.Context) {
		var payload entities.ClientPayload
		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}
		logger.Debugf("Payload %v", payload)
		Wallet := entities.Wallet{}
		d, _ := json.Marshal(payload.Data)
		e := json.Unmarshal(d, &Wallet)
		if e != nil {
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: e.Error()}))
		}
		// Wallet.ID = id
		payload.Data = Wallet
		event, err := client.CreateEvent(payload, p.Ctx)

		if err != nil {
			logger.Error(err)
			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
			return
		}

		c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: map[string]any{
			"event": event,
		}}))
	})

	// router.GET("/api/block-stats", func(c *gin.Context) {
	// 	b, parseError := utils.ParseQueryString(c)
	// 	if parseError != nil {
	// 		logger.Error(parseError)
	// 		c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: parseError.Error()}))
	// 		return
	// 	}

	// 	//
	// 	var params BlockParams
	// 	json.Unmarshal(*b, &params)
	// 	fromBlock, fromBlockErr := strconv.Atoi(params.FromBlock)
	// 	toBlock, toBlockErr := strconv.Atoi(params.ToBlock)

	// 	if fromBlockErr != nil || toBlockErr != nil {
	// 		logger.Error(parseError)
	// 		c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: utils.IfThenElse(fromBlockErr != nil, fromBlockErr.Error(), toBlockErr.Error())}))
	// 		return
	// 	}
	// 	stats := []BlockStat{}
	// 	for i := fromBlock; i <= toBlock; i++ {

	// 		topicEvents, err := client.GetTopicEvents()
	// 		if err != nil {
	// 			logger.Error(err)
	// 			c.JSON(http.StatusBadRequest, entities.NewClientResponse(entities.ClientResponse{Error: err.Error()}))
	// 			return
	// 		}
	// 		stats = append(stats, BlockStat{
	// 			Events:   i,
	// 			Topics:   i,
	// 			Messages: i,
	// 		})
	// 	}

	// 	logger.Debugf("Payload %v", params)

	// 	c.JSON(http.StatusOK, entities.NewClientResponse(entities.ClientResponse{Data: stats}))
	// })
	return router
}

type BlockParams struct {
	FromBlock string `json:"from_block"`
	ToBlock   string `json:"to_block"`
}
type BlockStat struct {
	Events   int `json:"events"`
	Topics   int `json:"topics"`
	Messages int `json:"messages"`
}
