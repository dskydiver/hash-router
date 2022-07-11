package eventbus

import (
	"context"
)

type EventBusController struct {
	eventBus *eventBus
}

const (
	ConnectionEventName = "connection"
	ContractEventName   = "contract"
	TestEventName       = "test"
)

type ConnectionEventData struct {
	Addr string
	Name string
}

type ContractEventData struct {
	ID       string
	DestAddr string
}

func NewEventBusController(e *eventBus) *EventBusController {
	return &EventBusController{
		eventBus: e,
	}
}

func (c *EventBusController) PublishConnection(data ConnectionEventData) {
	c.eventBus.Publish(ConnectionEventName, data)
}

func (c *EventBusController) PublishContract(data ContractEventData) {
	c.eventBus.Publish(ContractEventName, data)
}

func (c *EventBusController) SubscribeConnection(ctx context.Context, ch chan<- ConnectionEventData) {
	c.eventBus.Subscribe(ctx, ConnectionEventName, func(val interface{}) {
		ch <- val.(ConnectionEventData)
	})
}

func (c *EventBusController) SubscribeConnectionCb(ctx context.Context, cb func(data ConnectionEventData)) {
	c.eventBus.Subscribe(ctx, ConnectionEventName, func(val interface{}) {
		cb(val.(ConnectionEventData))
	})
}

func (c *EventBusController) SubscribeContract(ctx context.Context, ch chan<- ContractEventData) {
	c.eventBus.Subscribe(ctx, ContractEventName, func(val interface{}) {
		ch <- val.(ContractEventData)
	})
}

func (c *EventBusController) SubscribeContractCb(ctx context.Context, cb func(data ConnectionEventData)) {
	c.eventBus.Subscribe(ctx, ConnectionEventName, func(val interface{}) {
		cb(val.(ConnectionEventData))
	})
}
