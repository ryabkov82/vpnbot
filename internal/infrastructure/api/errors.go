package api

import "errors"

// ErrLogin2NotPersistedSHM — POST/PUT админ-пользователя завершился 200, но login2 по факту не виден через API (похожая на успех «синтетика» недопустима).
var ErrLogin2NotPersistedSHM = errors.New("shm login2 not persisted after admin user update")

// ErrServiceNotFound — услуга отсутствует или недоступна текущему экземпляру приложения
// (в т.ч. услуга другой категории: её существование не раскрывается).
// Текст обёртки сохраняет подстроку "not found" для совместимости с существующими проверками.
var ErrServiceNotFound = errors.New("service not found")
