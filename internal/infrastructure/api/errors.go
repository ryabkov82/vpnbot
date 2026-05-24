package api

import "errors"

// ErrLogin2NotPersistedSHM — POST/PUT админ-пользователя завершился 200, но login2 по факту не виден через API (похожая на успех «синтетика» недопустима).
var ErrLogin2NotPersistedSHM = errors.New("shm login2 not persisted after admin user update")
