// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package messaging

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	protomfx "github.com/MainfluxLabs/mainflux/pkg/proto"
	mfjson "github.com/MainfluxLabs/mainflux/pkg/transformers/json"
	"github.com/MainfluxLabs/mainflux/pkg/transformers/senml"
)

const (
	SenMLContentType = "application/senml+json"
	JSONContentType  = "application/json"

	TopicPrefixThings   = "things"
	TopicPrefixGroups   = "groups"
	TopicSuffixCommands = "commands"
	TopicSuffixMessages = "messages"
)

// ParsedTopic holds the components extracted from a structured topic or URL path.
type ParsedTopic struct {
	Prefix   string // "things" or "groups"
	ID       string // thing or group ID
	Suffix   string // "commands" or "messages"
	Subtopic string // normalized subtopic (dot-separated)
}

// ParseTopicPath parses a topic or URL path of the form
// "<prefix>/<id>/<suffix>[/<subtopic>]" into its components.
// The subtopic is normalized via NormalizeSubtopic.
// If the path has fewer than three slash-delimited parts or an empty ID,
// ok is false and the returned ParsedTopic is zero.
// Use this in adapters (MQTT, CoAP) that must determine routing from the raw path.
func ParseTopicPath(path string) (pt ParsedTopic, ok bool, err error) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 3 || parts[1] == "" {
		return ParsedTopic{}, false, nil
	}
	pt.Prefix, pt.ID, pt.Suffix = parts[0], parts[1], parts[2]
	if len(parts) == 4 {
		if pt.Subtopic, err = NormalizeSubtopic(parts[3]); err != nil {
			return ParsedTopic{}, false, err
		}
	}
	return pt, true, nil
}

// ExtractSubtopic finds the first occurrence of "/<suffix>/" in path and
// returns the normalized subtopic that follows it. If the suffix is absent
// or has no trailing content, an empty subtopic is returned.
// Use this in adapters (HTTP, WS) where the URL router has already handled
// routing and only the optional trailing subtopic needs to be extracted.
func ExtractSubtopic(path, suffix string) (string, error) {
	sep := "/" + suffix + "/"
	idx := strings.Index(path, sep)
	if idx < 0 {
		return "", nil
	}
	return NormalizeSubtopic(path[idx+len(sep):])
}

var (
	// ErrPublishMessage indicates that message publishing failed.
	ErrPublishMessage = errors.New("failed to publish message")

	// ErrFailedSubscribe indicates that subscribing to a topic failed.
	ErrFailedSubscribe = errors.New("failed to subscribe")

	// ErrFailedUnsubscribe indicates that unsubscribing from a topic failed.
	ErrFailedUnsubscribe = errors.New("failed to unsubscribe")

	// ErrConnect indicates that connection to MQTT broker failed
	ErrConnect = errors.New("failed to connect to MQTT broker")

	// ErrPublishTimeout indicates that the publishing failed due to timeout.
	ErrPublishTimeout = errors.New("failed to publish due to timeout reached")

	// ErrSubscribeTimeout indicates that the subscription failed due to timeout.
	ErrSubscribeTimeout = errors.New("failed to subscribe due to timeout reached")

	// ErrUnsubscribeTimeout indicates that unsubscribe failed due to timeout.
	ErrUnsubscribeTimeout = errors.New("failed to unsubscribe due to timeout reached")

	// ErrUnsubscribeDeleteTopic indicates that unsubscribe failed because the topic was deleted.
	ErrUnsubscribeDeleteTopic = errors.New("failed to unsubscribe due to deletion of topic")

	// ErrNotSubscribed indicates that the topic is not subscribed to.
	ErrNotSubscribed = errors.New("not subscribed")

	// ErrEmptyTopic indicates the absence of topic.
	ErrEmptyTopic = errors.New("empty topic")

	// ErrMalformedSubtopic indicates that the subtopic is malformed.
	ErrMalformedSubtopic = errors.New("malformed subtopic")

	// ErrEmptyID indicates the absence of ID.
	ErrEmptyID = errors.New("empty ID")

	// ErrInvalidContentType indicates an invalid Content-Type
	ErrInvalidContentType = errors.New("invalid content type")
)

// Publisher specifies message publishing API.
type Publisher interface {
	// Publish publishes message to the message broker.
	Publish(subject string, msg protomfx.Message) error

	// Close gracefully closes message publisher's connection.
	Close() error
}

// MessageHandler represents protomfx.Message handler for Subscriber.
type MessageHandler interface {
	// Handle handles messages passed by underlying implementation.
	Handle(subject string, msg protomfx.Message) error

	// Cancel is used for cleanup during unsubscribing and it's optional.
	Cancel() error
}

// Subscriber specifies message subscription API.
type Subscriber interface {
	// Subscribe subscribes to the message stream and consumes messages.
	Subscribe(id, topic string, handler MessageHandler) error

	// Unsubscribe unsubscribes from the message stream and
	// stops consuming messages.
	Unsubscribe(id, topic string) error

	// Close gracefully closes message subscriber's connection.
	Close() error
}

// PubSub  represents aggregation interface for publisher and subscriber.
type PubSub interface {
	Publisher
	Subscriber
}

func NormalizeSubtopic(topic string) (string, error) {
	if topic == "" {
		return topic, nil
	}

	// URL decode if needed
	decoded, err := url.QueryUnescape(topic)
	if err != nil {
		return "", ErrMalformedSubtopic
	}

	// Replace slashes with dots
	normalized := strings.Replace(decoded, "/", ".", -1)

	// Split and filter empty elements
	elems := strings.Split(normalized, ".")
	filteredElems := []string{}

	for _, elem := range elems {
		if elem == "" {
			continue
		}

		if len(elem) > 1 && (strings.Contains(elem, "*") || strings.Contains(elem, ">")) {
			return "", ErrMalformedSubtopic
		}

		filteredElems = append(filteredElems, elem)
	}

	return strings.Join(filteredElems, "."), nil
}

func FormatMessage(pc *protomfx.PubConfigByKeyRes, msg *protomfx.Message) error {
	msg.Publisher = pc.PublisherID
	msg.Created = time.Now().UnixNano()

	if pc.ProfileConfig != nil {
		msg.ContentType = pc.ProfileConfig.ContentType
		if pc.ProfileConfig.Transformer != nil {
			switch msg.ContentType {
			case JSONContentType:
				if err := mfjson.TransformPayload(*pc.ProfileConfig.Transformer, msg); err != nil {
					return err
				}
			case SenMLContentType:
				if err := senml.TransformPayload(msg); err != nil {
					return err
				}
			default:
				return ErrInvalidContentType
			}
		}
	}

	return nil
}

func ToJSONMessage(message protomfx.Message) mfjson.Message {
	created := message.Created
	var payload map[string]any

	if len(message.Payload) > 0 {
		if err := json.Unmarshal(message.Payload, &payload); err == nil {
			if payloadCreated, ok := payload["Created"].(float64); ok {
				created = int64(payloadCreated)
				delete(payload, "Created")

				message.Payload, _ = json.Marshal(payload)
			}
		}
	}

	return mfjson.Message{
		Created:   created,
		Subtopic:  message.Subtopic,
		Publisher: message.Publisher,
		Protocol:  message.Protocol,
		Payload:   message.Payload,
	}
}

func ToSenMLMessage(message protomfx.Message) (senml.Message, error) {
	var msg senml.Message
	if err := json.Unmarshal(message.Payload, &msg); err != nil {
		return senml.Message{}, err
	}

	msg.Publisher = message.Publisher
	msg.Subtopic = message.Subtopic
	msg.Protocol = message.Protocol

	return msg, nil
}

func SplitMessage(message protomfx.Message) ([]protomfx.Message, error) {
	var payload any
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return nil, err
	}

	if pyds, ok := payload.([]any); ok {
		var messages []protomfx.Message
		for _, pyd := range pyds {
			data, err := json.Marshal(pyd)
			if err != nil {
				return nil, err
			}
			newMsg := message
			newMsg.Payload = data
			messages = append(messages, newMsg)
		}
		return messages, nil
	}

	return []protomfx.Message{message}, nil
}
