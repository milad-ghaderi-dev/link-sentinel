package domain

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
	ErrUnauthorized       = errors.New("authorization required")
	ErrInvalidToken       = errors.New("invalid or expired token")

	ErrProjectNotFound      = errors.New("project not found")
	ErrProjectAlreadyExists = errors.New("a project with this name and URL already exists")
	ErrInvalidProjectID     = errors.New("invalid project id")

	ErrJobNotFound = errors.New("crawl job not found or expired")
	ErrInvalidURL  = errors.New("invalid target url")
)
