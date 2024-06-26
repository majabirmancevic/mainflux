// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package notifiers

import (
	"github.com/MainfluxLabs/mainflux"
	"github.com/MainfluxLabs/mainflux/consumers"
	"github.com/MainfluxLabs/mainflux/pkg/errors"
	"github.com/MainfluxLabs/mainflux/pkg/messaging"
)

// Service reprents a notification service.
type Service interface {
	consumers.Consumer
}

var _ Service = (*notifierService)(nil)

type notifierService struct {
	auth     mainflux.AuthServiceClient
	idp      mainflux.IDProvider
	notifier Notifier
	from     string
}

// New instantiates the subscriptions service implementation.
func New(auth mainflux.AuthServiceClient, idp mainflux.IDProvider, notifier Notifier, from string) Service {
	return &notifierService{
		auth:     auth,
		idp:      idp,
		notifier: notifier,
		from:     from,
	}
}

func (ns *notifierService) Consume(message interface{}) error {
	msg, ok := message.(messaging.Message)
	if !ok {
		return errors.ErrMessage
	}

	err := ns.notifier.Notify(ns.from, msg.Profile.Notifier.Contacts, msg)
	if err != nil {
		return errors.Wrap(ErrNotify, err)
	}

	return nil
}
