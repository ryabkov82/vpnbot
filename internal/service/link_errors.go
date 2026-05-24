package service

import "errors"

// Ошибки привязки web-email к телеграм-пользователю (личный кабинет).
var (
	ErrWebEmailAlreadyLinked      = errors.New("web_email_already_linked")
	ErrWebEmailUsedByOtherAccount = errors.New("web_email_used_by_other_account")
	ErrWebLogin2NotPersisted      = errors.New("web_login2_not_persisted")
	ErrTelegramChatMismatch       = errors.New("telegram_chat_mismatch")
)
