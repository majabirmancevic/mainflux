// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"

	"github.com/MainfluxLabs/mainflux/pkg/dbutil"
	"github.com/MainfluxLabs/mainflux/pkg/errors"
	"github.com/MainfluxLabs/mainflux/pkg/transformers/senml"
	"github.com/MainfluxLabs/mainflux/readers"
	"github.com/go-kit/kit/endpoint"
)

var header = []string{
	"profile",
	"subtopic",
	"publisher",
	"protocol",
	"name",
	"unit",
	"value",
	"string_value",
	"bool_value",
	"data_value",
	"sum",
	"time",
	"update_time",
}

func listAllMessagesEndpoint(svc readers.MessageRepository) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(listAllMessagesReq)
		if err := req.validate(); err != nil {
			return nil, err
		}

		var page readers.MessagesPage
		switch {
		case req.key != "":
			pc, err := getPubConfByKey(ctx, req.key)
			if err != nil {
				return nil, err
			}
			req.pageMeta.Publisher = pc.PublisherID

			p, err := svc.ListAllMessages(req.pageMeta)
			if err != nil {
				return nil, err
			}

			page = p
		default:
			// Check if user is authorized to read all messages
			if err := isAdmin(ctx, req.token); err != nil {
				return nil, err
			}

			p, err := svc.ListAllMessages(req.pageMeta)
			if err != nil {
				return nil, err
			}

			page = p
		}

		return listMessagesRes{
			PageMetadata: page.PageMetadata,
			Total:        page.Total,
			Messages:     page.Messages,
		}, nil
	}
}

func deleteMessagesEndpoint(svc readers.MessageRepository) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(deleteMessagesReq)
		if err := req.validate(); err != nil {
			return nil, err
		}

		var table string

		switch {
		case req.key != "":
			pc, err := getPubConfByKey(ctx, req.key)
			if err != nil {
				return nil, errors.Wrap(errors.ErrAuthentication, err)
			}

			req.pageMeta.Publisher = pc.PublisherID
			table = dbutil.GetTableName(pc.ProfileConfig.GetContentType())

		case req.token != "":
			if err := isAdmin(ctx, req.token); err != nil {
				return nil, err
			}
		default:
			return nil, errors.ErrAuthentication
		}

		err :=  svc.DeleteMessages(ctx, req.pageMeta, table)
		if err != nil {
			return nil, err
		}

		return nil, nil
	}
}

func backupEndpoint(svc readers.MessageRepository) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(listAllMessagesReq)
		if err := req.validate(); err != nil {
			return nil, err
		}

		if err := isAdmin(ctx, req.token); err != nil {
			return nil, err
		}

		page, err := svc.Backup(req.pageMeta)
		if err != nil {
			return nil, err
		}

		csvData, err := generateCSV(page)
		if err != nil {
			return nil, err
		}

		return backupFileRes{file: csvData}, nil
	}
}

func restoreEndpoint(svc readers.MessageRepository) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(restoreMessagesReq)
		if err := req.validate(); err != nil {
			return nil, err
		}

		// Check if user is authorized to read all messages
		if err := isAdmin(ctx, req.token); err != nil {
			return nil, err
		}

		if err := svc.Restore(ctx, req.Messages...); err != nil {
			return nil, err
		}

		return restoreMessagesRes{}, nil
	}
}

func generateCSV(page readers.MessagesPage) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	if err := writer.Write(header); err != nil {
		return nil, err
	}

	if err := convertSenMLToCSV(page, writer); err != nil {
		return nil, err
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func convertSenMLToCSV(page readers.MessagesPage, writer *csv.Writer) error {
	for _, msg := range page.Messages {
		if m, ok := msg.(senml.Message); ok {
			row := []string{
				m.Subtopic,
				m.Publisher,
				m.Protocol,
				m.Name,
				m.Unit,
				getValue(m.Value, ""),
				getValue(m.StringValue, ""),
				getValue(m.BoolValue, ""),
				getValue(m.DataValue, ""),
				getValue(m.Sum, ""),
				fmt.Sprintf("%v", m.Time),
				fmt.Sprintf("%v", m.UpdateTime),
			}

			if err := writer.Write(row); err != nil {
				return err
			}
		}
	}
	return nil
}

func getValue(ptr interface{}, defaultValue string) string {
	switch v := ptr.(type) {
	case *string:
		if v != nil {
			return *v
		}
	case *float64:
		if v != nil {
			return fmt.Sprintf("%v", *v)
		}
	case *bool:
		if v != nil {
			return fmt.Sprintf("%v", *v)
		}
	}
	return defaultValue
}
