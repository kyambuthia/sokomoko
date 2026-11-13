package messaging

import "github.com/kyambuthia/sokomoko/internal/db"

type (
	ChatInbox         = db.ChatInbox
	ChatContact       = db.ChatContact
	ChatTarget        = db.ChatTarget
	ChatThread        = db.ChatThread
	ChatReport        = db.ChatReport
	EncryptedRequest  = db.EncryptedRequest
	EncryptedResponse = db.EncryptedResponse
)

var (
	ErrChatInvalid            = db.ErrChatInvalid
	ErrChatEventOrder         = db.ErrChatEventOrder
	ErrChatDeviceApproval     = db.ErrChatDeviceApproval
	ErrChatRosterChanged      = db.ErrChatRosterChanged
	ErrChatEncryptionRequired = db.ErrChatEncryptionRequired
	ErrChatNotFound           = db.ErrChatNotFound
	ErrChatClosed             = db.ErrChatClosed
	ErrChatBlocked            = db.ErrChatBlocked
	ErrChatLimited            = db.ErrChatLimited
	ErrChatConflict           = db.ErrChatConflict
)
