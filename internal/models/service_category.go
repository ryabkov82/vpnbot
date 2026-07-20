package models

import "strings"

// ServiceCategoryAllowed — авторизационная проверка категории услуги.
// Пустая ожидаемая категория сохраняет legacy-поведение (без ограничения).
// Сравнение строгое (после TrimSpace); проверка по префиксу недопустима.
func ServiceCategoryAllowed(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	return strings.TrimSpace(actual) == expected
}
