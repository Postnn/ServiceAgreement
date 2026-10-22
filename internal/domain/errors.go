package domain

import "errors"

var ErrConflict = errors.New("запись изменена или удалена другим пользователем; обновите список")
