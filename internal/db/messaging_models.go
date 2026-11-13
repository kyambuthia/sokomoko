package db

import "github.com/kyambuthia/sokomoko/internal/domain"

var (
	ErrChatNotFound = domain.ErrChatNotFound
	ErrChatInvalid  = domain.ErrChatInvalid
	ErrChatClosed   = domain.ErrChatClosed
	ErrChatBlocked  = domain.ErrChatBlocked
	ErrChatLimited  = domain.ErrChatLimited
	ErrChatConflict = domain.ErrChatConflict
)

type (
	ChatTarget       = domain.ChatTarget
	ChatContact      = domain.ChatContact
	ChatConversation = domain.ChatConversation
	ChatMessage      = domain.ChatMessage
	ChatThread       = domain.ChatThread
	ChatInbox        = domain.ChatInbox
	ChatReport       = domain.ChatReport
)
