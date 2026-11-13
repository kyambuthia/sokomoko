package messaging

import "github.com/kyambuthia/sokomoko/internal/domain"

type (
	ChatInbox         = domain.ChatInbox
	ChatContact       = domain.ChatContact
	ChatTarget        = domain.ChatTarget
	ChatThread        = domain.ChatThread
	ChatReport        = domain.ChatReport
	EncryptedRequest  = domain.EncryptedRequest
	EncryptedResponse = domain.EncryptedResponse
)

var (
	ErrChatInvalid            = domain.ErrChatInvalid
	ErrChatEventOrder         = domain.ErrChatEventOrder
	ErrChatDeviceApproval     = domain.ErrChatDeviceApproval
	ErrChatRosterChanged      = domain.ErrChatRosterChanged
	ErrChatEncryptionRequired = domain.ErrChatEncryptionRequired
	ErrChatNotFound           = domain.ErrChatNotFound
	ErrChatClosed             = domain.ErrChatClosed
	ErrChatBlocked            = domain.ErrChatBlocked
	ErrChatLimited            = domain.ErrChatLimited
	ErrChatConflict           = domain.ErrChatConflict
)
