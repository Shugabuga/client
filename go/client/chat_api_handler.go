// Copyright 2016 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type ErrInvalidOptions struct {
	method  string
	version int
	err     error
}

func (e ErrInvalidOptions) Error() string {
	return fmt.Sprintf("invalid %s v%d options: %s", e.method, e.version, e.err)
}

type Call struct {
	Jsonrpc string
	ID      int
	Method  string
	Params  Params
}

type Params struct {
	Version int
	Options json.RawMessage
}

type ChatAPIHandler interface {
	ListV1(Call, io.Writer) error
	ReadV1(Call, io.Writer) error
	SendV1(Call, io.Writer) error
}

type ChatAPI struct{}

type ChatChannel struct {
	Name   string
	Public bool
}

func (c ChatChannel) Valid() bool {
	return len(c.Name) > 0
}

type ChatMessage struct {
	Body string
}

func (c ChatMessage) Valid() bool {
	return len(c.Body) > 0
}

type sendOptionsV1 struct {
	Channel ChatChannel
	Message ChatMessage
}

func (s sendOptionsV1) Check() error {
	if !s.Channel.Valid() {
		return ErrInvalidOptions{version: 1, method: "send", err: errors.New("invalid channel")}
	}
	if !s.Message.Valid() {
		return ErrInvalidOptions{version: 1, method: "send", err: errors.New("invalid message")}
	}

	return nil
}

type readOptionsV1 struct {
	Channel        ChatChannel
	ConversationID string `json:"conversation_id"`
	Limit          string
}

func (r readOptionsV1) Check() error {
	fmt.Printf("read opts: %+v\n", r)
	if !r.Channel.Valid() && len(r.ConversationID) == 0 {
		return ErrInvalidOptions{version: 1, method: "read", err: errors.New("need channel or conversation_id")}
	}
	if r.Channel.Valid() && len(r.ConversationID) > 0 {
		return ErrInvalidOptions{version: 1, method: "read", err: errors.New("include channel or conversation_id, not both")}
	}

	return nil
}

func (a *ChatAPI) ListV1(c Call, w io.Writer) error {
	if len(c.Params.Options) != 0 {
		return ErrInvalidOptions{version: 1, method: "list", err: errors.New("unexpected options, should be empty")}
	}
	return nil
}

func (a *ChatAPI) ReadV1(c Call, w io.Writer) error {
	if len(c.Params.Options) == 0 {
		return ErrInvalidOptions{version: 1, method: "read", err: errors.New("empty options")}
	}
	var opts readOptionsV1
	if err := json.Unmarshal(c.Params.Options, &opts); err != nil {
		return err
	}
	if err := opts.Check(); err != nil {
		return err
	}

	// opts are valid for read v1

	return nil
}

func (a *ChatAPI) SendV1(c Call, w io.Writer) error {
	if len(c.Params.Options) == 0 {
		return ErrInvalidOptions{version: 1, method: "send", err: errors.New("empty options")}
	}
	var opts sendOptionsV1
	if err := json.Unmarshal(c.Params.Options, &opts); err != nil {
		return err
	}
	if err := opts.Check(); err != nil {
		return err
	}

	// opts are valid for send v1

	return nil
}
