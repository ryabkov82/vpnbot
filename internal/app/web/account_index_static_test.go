package web

import (
	"strings"
	"testing"
)

func TestAccountIndexMagicLinkSuccessCopy(t *testing.T) {
	s := mustRenderAccountLoginHTML(t, orderStartTestCfg(), accountLocaleRU)
	if strings.Contains(s, "Если такой email есть в системе, мы отправили ссылку для входа.") {
		t.Fatal("must not leak old ambiguous copy")
	}
	if !strings.Contains(s, "Мы отправили ссылку для входа на указанный email. Откройте письмо и перейдите по ссылке.") {
		t.Fatal("missing success copy for magic-link")
	}
	if !strings.Contains(s, "Если письма нет, проверьте папку «Спам» или попробуйте еще раз через пару минут.") {
		t.Fatal("missing spam/minutes hint")
	}
	if strings.Contains(s, "с которым вы регистрировались") {
		t.Fatal(`must not imply prior registration (“с которым вы регистрировались”)`)
	}
	if !strings.Contains(s, "Введите email — мы отправим ссылку для входа без пароля.") {
		t.Fatal("missing magic-link intro copy")
	}
	if !strings.Contains(s, "Если вы уже пользуетесь Telegram-ботом, откройте в боте команду «Личный кабинет». Так ваши текущие услуги и баланс будут доступны в web-кабинете.") {
		t.Fatal("missing Telegram bot linking hint")
	}
	if strings.Contains(s, "перенести доступ") {
		t.Fatal("must not use old copy with «перенести доступ»")
	}
	if strings.Contains(s, "балансу") {
		t.Fatal("must not use old copy with «балансу»")
	}
	if strings.Contains(s, "Если вы здесь впервые, личный кабинет будет создан после подтверждения email.") {
		t.Fatal("must not show old new-user-only intro copy")
	}
}
