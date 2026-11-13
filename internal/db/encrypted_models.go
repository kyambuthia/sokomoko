package db

import (
	"encoding/json"

	"github.com/kyambuthia/sokomoko/internal/domain"
)

var (
	ErrChatEncryptionRequired = domain.ErrChatEncryptionRequired
	ErrChatDeviceApproval     = domain.ErrChatDeviceApproval
	ErrChatEventOrder         = domain.ErrChatEventOrder
	ErrChatRosterChanged      = domain.ErrChatRosterChanged
)

type (
	EncryptedRequest  = domain.EncryptedRequest
	ChatDevice        = domain.ChatDevice
	EncryptedRoster   = domain.EncryptedRoster
	ChatPackage       = domain.ChatPackage
	EncryptedSession  = domain.EncryptedSession
	EncryptedEvent    = domain.EncryptedEvent
	EncryptedResponse = domain.EncryptedResponse
)

func encryptedPayload[T any](raw string) (T, error) {
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return v, ErrChatInvalid
	}
	return v, nil
}
