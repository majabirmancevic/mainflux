// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"context"
	"sync"

	"github.com/MainfluxLabs/mainflux/consumers/notifiers"
	"github.com/MainfluxLabs/mainflux/pkg/apiutil"
	"github.com/MainfluxLabs/mainflux/pkg/errors"
	protomfx "github.com/MainfluxLabs/mainflux/pkg/proto"
	"github.com/MainfluxLabs/mainflux/pkg/uuid"
)

var _ notifiers.Sender = (*notifier)(nil)
var _ notifiers.NotifierRepository = (*notifierRepositoryMock)(nil)

const (
	invalidEmail = "invalid@example.com"
	invalidPhone = "0611111111"
)

type notifier struct{}

// NewNotifier returns a new Sender mock.
func NewNotifier() notifiers.Sender {
	return notifier{}
}

type notifierRepositoryMock struct {
	mu        sync.Mutex
	notifiers map[string]notifiers.Notifier
}

func NewNotifierRepository() notifiers.NotifierRepository {
	return &notifierRepositoryMock{notifiers: make(map[string]notifiers.Notifier)}
}

func (n notifier) Send(to []string, _ protomfx.Message) error {
	if len(to) < 1 {
		return notifiers.ErrNotify
	}

	for _, t := range to {
		if t == invalidEmail || t == "" {
			return notifiers.ErrNotify
		}
	}

	return nil
}

func (n notifier) ValidateContacts(contacts []string) error {
	for _, c := range contacts {
		if c == "" || c == invalidEmail || c == invalidPhone {
			return apiutil.ErrInvalidContact
		}
	}

	return nil
}

func (nrm *notifierRepositoryMock) Save(_ context.Context, nfs ...notifiers.Notifier) ([]notifiers.Notifier, error) {
	nrm.mu.Lock()
	defer nrm.mu.Unlock()

	for _, nf := range nfs {
		for _, n := range nrm.notifiers {
			if n.GroupID == nf.GroupID && n.Name == nf.Name {
				return []notifiers.Notifier{}, errors.ErrConflict
			}
		}

		nrm.notifiers[nf.ID] = nf
	}

	return nfs, nil
}

func (nrm *notifierRepositoryMock) RetrieveByGroup(_ context.Context, groupID string, pm apiutil.PageMetadata) (res notifiers.NotifiersPage, err error) {
	nrm.mu.Lock()
	defer nrm.mu.Unlock()
	var items []notifiers.Notifier
	filteredItems := make([]notifiers.Notifier, 0)

	first := uint64(pm.Offset) + 1
	last := first + uint64(pm.Limit)

	for _, nf := range nrm.notifiers {
		if nf.GroupID == groupID {
			id := uuid.ParseID(nf.ID)
			if id >= first && id < last || pm.Limit == 0 {
				items = append(items, nf)
			}
		}
	}

	if pm.Name != "" {
		for _, v := range items {
			if v.Name == pm.Name {
				filteredItems = append(filteredItems, v)
			}
		}
		items = filteredItems
	}

	return notifiers.NotifiersPage{
		Notifiers: items,
		PageMetadata: apiutil.PageMetadata{
			Total:  uint64(len(items)),
			Offset: pm.Offset,
			Limit:  pm.Limit,
		},
	}, nil
}

func (nrm *notifierRepositoryMock) RetrieveByID(_ context.Context, id string) (notifiers.Notifier, error) {
	nrm.mu.Lock()
	defer nrm.mu.Unlock()

	for _, nf := range nrm.notifiers {
		if nf.ID == id {
			return nf, nil
		}
	}

	return notifiers.Notifier{}, errors.ErrNotFound
}

func (nrm *notifierRepositoryMock) Update(_ context.Context, nf notifiers.Notifier) error {
	nrm.mu.Lock()
	defer nrm.mu.Unlock()

	if _, ok := nrm.notifiers[nf.ID]; !ok {
		return errors.ErrNotFound
	}
	nrm.notifiers[nf.ID] = nf

	return nil
}

func (nrm *notifierRepositoryMock) Remove(_ context.Context, ids ...string) error {
	nrm.mu.Lock()
	defer nrm.mu.Unlock()

	for _, id := range ids {
		if _, ok := nrm.notifiers[id]; !ok {
			return errors.ErrNotFound
		}
		delete(nrm.notifiers, id)
	}

	return nil
}
