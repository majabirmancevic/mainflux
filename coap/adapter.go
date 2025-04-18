// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

// Package coap contains the domain concept definitions needed to support
// Mainflux CoAP adapter service functionality. All constant values are taken
// from RFC, and could be adjusted based on specific use case.
package coap

import (
	"context"
	"sync"

	"github.com/MainfluxLabs/mainflux/pkg/errors"
	"github.com/MainfluxLabs/mainflux/pkg/messaging"
	protomfx "github.com/MainfluxLabs/mainflux/pkg/proto"
)

// Service specifies CoAP service API.
type Service interface {
	// Publish Messssage
	Publish(ctx context.Context, key string, msg protomfx.Message) error

	// Subscribe subscribes to profile with specified id, subtopic and adds subscription to
	// service map of subscriptions under given ID.
	Subscribe(ctx context.Context, key, subtopic string, c Client) error

	// Unsubscribe method is used to stop observing resource.
	Unsubscribe(ctx context.Context, key, subptopic, token string) error
}

var _ Service = (*adapterService)(nil)

// Observers is a map of maps,
type adapterService struct {
	things  protomfx.ThingsServiceClient
	pubsub  messaging.PubSub
	obsLock sync.Mutex
}

// New instantiates the CoAP adapter implementation.
func New(things protomfx.ThingsServiceClient, pubsub messaging.PubSub) Service {
	as := &adapterService{
		things:  things,
		pubsub:  pubsub,
		obsLock: sync.Mutex{},
	}

	return as
}

func (svc *adapterService) Publish(ctx context.Context, key string, msg protomfx.Message) error {
	cr := &protomfx.PubConfByKeyReq{
		Key: key,
	}
	pc, err := svc.things.GetPubConfByKey(ctx, cr)
	if err != nil {
		return errors.Wrap(errors.ErrAuthorization, err)
	}

	messaging.FormatMessage(pc, &msg)

	return svc.pubsub.Publish(msg)
}

func (svc *adapterService) Subscribe(ctx context.Context, key, subtopic string, c Client) error {
	cr := &protomfx.PubConfByKeyReq{
		Key: key,
	}
	if _, err := svc.things.GetPubConfByKey(ctx, cr); err != nil {
		return errors.Wrap(errors.ErrAuthorization, err)
	}

	return svc.pubsub.Subscribe(c.Token(), subtopic, c)
}

func (svc *adapterService) Unsubscribe(ctx context.Context, key, subtopic, token string) error {
	cr := &protomfx.PubConfByKeyReq{
		Key: key,
	}
	_, err := svc.things.GetPubConfByKey(ctx, cr)
	if err != nil {
		return errors.Wrap(errors.ErrAuthorization, err)
	}

	return svc.pubsub.Unsubscribe(token, subtopic)
}
