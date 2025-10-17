package models

import "errors"

var (
	ErrUserFault        = errors.New("error has occurred")
	ErrValidationFailed = errors.Join(ErrUserFault, errors.New("validation failed"))

	ErrNotFound     = errors.New("entity not found")
	ErrUserNotFound = errors.Join(ErrNotFound, errors.New("user not found"))

	ErrDatabase = errors.New("database error")
)

func UserError(text string) error {
	return errors.Join(ErrUserFault, errors.New(text))
}
